/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"
	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

const (
	testNamespace      = "test-ray-ns"
	timeout            = 60 * time.Second
	interval           = 250 * time.Millisecond
	webhookName        = "kuberay-mutating-webhook-configuration"
	lifecycleCRD       = "fakes.lifecycle.ray.test.io"
	strayCMName        = "stray-part-of-ray"
	inTreeImage        = "quay.io/opendatahub/kuberay-operator:in-tree"
	managedImage       = "quay.io/opendatahub/kuberay-operator:test"
	deploymentName     = "kuberay-operator"
	configMapName      = "ray-operator-config"
	testKickAnnotation = "test.opendatahub.io/kick"
)

var _ = Describe("Ray Controller", Ordered, func() {
	rayCR := types.NamespacedName{Name: constants.InstanceName}

	BeforeAll(func() {
		By("creating the test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())

		By("creating the default platform ConfigMap (standalone defaults)")
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.PlatformConfigMapName,
				Namespace: testNamespace,
			},
			Data: map[string]string{
				constants.PlatformNameKey:    constants.StandaloneDistributionName,
				constants.PlatformVersionKey: "0.0.0",
			},
		}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, cm))).To(Succeed())

		By("creating a CRD labeled as a Ray operand (must survive GC)")
		Expect(k8sClient.Create(ctx, labeledCRD())).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up the Ray CR")
		ray := &componentsv1alpha1.Ray{}
		err := k8sClient.Get(ctx, rayCR, ray)
		if err == nil {
			Expect(k8sClient.Delete(ctx, ray)).To(Succeed())
		}

		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: lifecycleCRD}, crd); err == nil {
			Expect(k8sClient.Delete(ctx, crd)).To(Succeed())
		}
	})

	Context("when applicationsNamespace is not projected", func() {
		It("should not deploy and should delete without sticking", func() {
			By("creating Ray CR without applicationsNamespace")
			createRayCR("")

			By("verifying no deployment is created in the test namespace")
			Consistently(func() bool {
				return isNotFound(&appsv1.Deployment{}, deploymentName, testNamespace)
			}, 5*time.Second, interval).Should(BeTrue())

			By("verifying the deletion finalizer is not kept while nothing is deployed")
			Eventually(func(g Gomega) {
				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(ray, constants.FinalizerName)).To(BeFalse())
			}, timeout, interval).Should(Succeed())

			By("deleting the CR while namespace is still unprojected")
			ray := &componentsv1alpha1.Ray{}
			Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ray)).To(Succeed())

			Eventually(func() bool {
				return isNotFound(&componentsv1alpha1.Ray{}, constants.InstanceName, "")
			}, timeout, interval).Should(BeTrue())

			// Recreating default-ray in the next spec must not race a
			// tombstone still sitting in the informer.
			Consistently(func() bool {
				return isNotFound(&componentsv1alpha1.Ray{}, constants.InstanceName, "")
			}, time.Second, interval).Should(BeTrue())
		})
	})

	Context("when managementState is Managed", func() {
		It("should adopt existing KubeRay resources and deploy the rest", func() {
			By("creating an in-tree kuberay-operator Deployment")
			existing := inTreeDeployment()
			Expect(k8sClient.Create(ctx, existing)).To(Succeed())
			originalUID := existing.UID
			Expect(originalUID).NotTo(BeEmpty())

			By("creating Ray CR with applicationsNamespace")
			createRayCR(testNamespace)

			By("verifying the Deployment is adopted (same UID, spec and part-of label taken over)")
			Eventually(func(g Gomega) {
				dep := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: testNamespace,
				}, dep)).To(Succeed())
				g.Expect(dep.UID).To(Equal(originalUID))
				g.Expect(dep.Spec.Template.Spec.Containers).NotTo(BeEmpty())
				g.Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(managedImage))
				g.Expect(dep.Labels[gc.DefaultPartOfLabelKey]).To(Equal(constants.ComponentName))

				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
				owned := false
				for _, ref := range dep.OwnerReferences {
					if ref.UID == ray.UID && ref.Kind == "Ray" {
						owned = true

						break
					}
				}
				g.Expect(owned).To(BeTrue(), "expected Ray ownerRef on adopted Deployment")
			}, timeout, interval).Should(Succeed())

			By("verifying the ConfigMap is created")
			Eventually(func() error {
				cm := &corev1.ConfigMap{}

				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: testNamespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			By("verifying the cluster-scoped webhook is created with cert-manager TLS")
			Eventually(func(g Gomega) {
				wh := &admissionregistrationv1.MutatingWebhookConfiguration{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: webhookName}, wh)).To(Succeed())

				g.Expect(wh.Annotations).To(HaveKey("cert-manager.io/inject-ca-from"))
				g.Expect(wh.Annotations["cert-manager.io/inject-ca-from"]).To(ContainSubstring("/serving-cert"))

				g.Expect(wh.Webhooks).To(HaveLen(1))
				hook := wh.Webhooks[0]
				g.Expect(hook.ClientConfig.Service).NotTo(BeNil())
				g.Expect(hook.ClientConfig.Service.Name).To(Equal("kuberay-webhook-service"))
				g.Expect(*hook.ClientConfig.Service.Path).To(Equal("/mutate-ray-io-v1-raycluster"))

				g.Expect(hook.Rules).To(HaveLen(1))
				g.Expect(hook.Rules[0].APIGroups).To(ContainElement("ray.io"))
				g.Expect(hook.Rules[0].Resources).To(ContainElement("rayclusters"))
				g.Expect(hook.Rules[0].Operations).To(ContainElements(
					admissionregistrationv1.Create,
					admissionregistrationv1.Update,
				))
			}, timeout, interval).Should(Succeed())

			By("verifying the notebook ClusterRole is created")
			Eventually(func(g Gomega) {
				cr := &rbacv1.ClusterRole{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: notebookClusterRoleName}, cr)).To(Succeed())
				g.Expect(cr.Labels[gc.DefaultPartOfLabelKey]).To(Equal(constants.ComponentName))
				g.Expect(cr.OwnerReferences).To(BeEmpty())
				g.Expect(cr.Rules).To(HaveLen(5))
			}, timeout, interval).Should(Succeed())

			By("verifying KubeRay is reported in status.releases")
			Eventually(func(g Gomega) {
				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
				g.Expect(ray.Status.ObservedGeneration).To(Equal(ray.Generation))

				g.Expect(ray.Status.Releases).To(HaveLen(1))
				g.Expect(ray.Status.Releases[0].Name).To(Equal(constants.KubeRayReleaseName))
				g.Expect(ray.Status.Releases[0].Version).To(Equal("v1.6.2"))
				g.Expect(ray.Status.Releases[0].RepoURL).To(Equal(constants.KubeRayRepoURL))
			}, timeout, interval).Should(Succeed())

			By("verifying Degraded is False")
			Eventually(func(g Gomega) {
				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())

				found := false
				for _, c := range ray.Status.Conditions {
					if c.Type == constants.ConditionDegraded {
						g.Expect(string(c.Status)).To(Equal("False"))
						g.Expect(c.Reason).To(Equal("AsExpected"))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Degraded condition not found")
			}, timeout, interval).Should(Succeed())

			By("marking the KubeRay Deployment available")
			Eventually(func(g Gomega) {
				dep := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: testNamespace,
				}, dep)).To(Succeed())
				dep.Status.Replicas = 1
				dep.Status.ReadyReplicas = 1
				dep.Status.AvailableReplicas = 1
				dep.Status.UpdatedReplicas = 1
				g.Expect(k8sClient.Status().Update(ctx, dep)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("verifying standalone status.distribution after rollout")
			Eventually(func(g Gomega) {
				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
				g.Expect(ray.Status.Distribution.Name).To(Equal(constants.StandaloneDistributionName))
				g.Expect(ray.Status.Distribution.Version).To(Equal("0.0.0"))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("when the platform config ConfigMap is present", func() {
		It("should stamp status.distribution from the ConfigMap after rollout", func() {
			By("updating the platform config ConfigMap with platform values")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      constants.PlatformConfigMapName,
				Namespace: testNamespace,
			}, cm)).To(Succeed())
			cm.Data[constants.PlatformNameKey] = "OpenDataHub"
			cm.Data[constants.PlatformVersionKey] = "2.20.0"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			By("verifying status.distribution matches the ConfigMap")
			Eventually(func(g Gomega) {
				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
				g.Expect(ray.Status.ObservedGeneration).To(Equal(ray.Generation))
				g.Expect(ray.Status.Distribution.Name).To(Equal("OpenDataHub"))
				g.Expect(ray.Status.Distribution.Version).To(Equal("2.20.0"))
				g.Expect(ray.Status.Releases).To(HaveLen(1))
				g.Expect(ray.Status.Releases[0].Name).To(Equal(constants.KubeRayReleaseName))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("when the platform ConfigMap version is invalid", func() {
		It("should keep deploying and retain the previous distribution", func() {
			By("updating the platform config ConfigMap with an invalid version")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      constants.PlatformConfigMapName,
				Namespace: testNamespace,
			}, cm)).To(Succeed())
			cm.Data[constants.PlatformVersionKey] = "not-a-semver"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			By("verifying the Deployment is still present")
			Consistently(func() error {
				dep := &appsv1.Deployment{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: testNamespace,
				}, dep)
			}, 3*time.Second, interval).Should(Succeed())

			By("verifying status.distribution is unchanged")
			Consistently(func(g Gomega) {
				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
				g.Expect(ray.Status.Distribution.Name).To(Equal("OpenDataHub"))
				g.Expect(ray.Status.Distribution.Version).To(Equal("2.20.0"))
			}, 3*time.Second, interval).Should(Succeed())
		})
	})

	Context("when Managed GC runs", func() {
		It("should not delete labeled objects it does not own", func() {
			stray := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strayCMName,
					Namespace: testNamespace,
					Labels: map[string]string{
						gc.DefaultPartOfLabelKey: constants.ComponentName,
					},
					Annotations: map[string]string{"unmanaged": "true"},
				},
			}
			Expect(k8sClient.Create(ctx, stray)).To(Succeed())
			Consistently(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: strayCMName, Namespace: testNamespace}, &corev1.ConfigMap{})
			}, 5*time.Second, interval).Should(Succeed())
		})
	})

	Context("when managementState is Removed", func() {
		It("should clean up labeled operands but keep CRDs", func() {
			By("waiting for any pending reconciles to settle")
			Eventually(func(g Gomega) {
				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
				g.Expect(ray.Status.ObservedGeneration).To(Equal(ray.Generation))
			}, timeout, interval).Should(Succeed())

			By("updating managementState to Removed")
			ray := &componentsv1alpha1.Ray{}
			Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
			ray.Spec.ManagementState = common.Removed
			Expect(k8sClient.Update(ctx, ray)).To(Succeed())

			By("verifying namespaced and cluster-scoped operands are deleted")
			Eventually(func() bool {
				return isNotFound(&appsv1.Deployment{}, deploymentName, testNamespace)
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isNotFound(&corev1.ConfigMap{}, configMapName, testNamespace)
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isNotFound(&admissionregistrationv1.MutatingWebhookConfiguration{}, webhookName, "")
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isNotFound(&rbacv1.ClusterRole{}, notebookClusterRoleName, "")
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isNotFound(&corev1.ConfigMap{}, strayCMName, testNamespace)
			}, timeout, interval).Should(BeTrue())

			By("verifying the labeled CRD is kept")
			crd := &apiextensionsv1.CustomResourceDefinition{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: lifecycleCRD}, crd)).To(Succeed())

			By("verifying DeploymentsAvailable is True with RemovedComponent and the CR remains")
			Eventually(func(g Gomega) {
				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(ray, constants.FinalizerName)).To(BeTrue())

				found := false
				for _, c := range ray.Status.Conditions {
					if c.Type == constants.ConditionDeploymentsAvailable {
						g.Expect(string(c.Status)).To(Equal("True"))
						g.Expect(c.Reason).To(Equal("RemovedComponent"))
						found = true

						break
					}
				}
				g.Expect(found).To(BeTrue(), "DeploymentsAvailable condition not found")
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("when the Ray CR is deleted while already Removed", func() {
		It("should remove the CR without sticking the finalizer", func() {
			ray := &componentsv1alpha1.Ray{}
			Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ray)).To(Succeed())

			Eventually(func() bool {
				return isNotFound(&componentsv1alpha1.Ray{}, constants.InstanceName, "")
			}, timeout, interval).Should(BeTrue())

			By("verifying the labeled CRD is still kept")
			crd := &apiextensionsv1.CustomResourceDefinition{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: lifecycleCRD}, crd)).To(Succeed())
		})
	})

	Context("when the Ray CR is deleted while Managed", func() {
		It("should clean up operands including the webhook, then remove the CR", func() {
			By("creating a Managed Ray CR with operands")
			createRayCR(testNamespace)

			Eventually(func() error {
				dep := &appsv1.Deployment{}

				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: testNamespace,
				}, dep)
			}, timeout, interval).Should(Succeed())
			Eventually(func() error {
				wh := &admissionregistrationv1.MutatingWebhookConfiguration{}

				return k8sClient.Get(ctx, types.NamespacedName{Name: webhookName}, wh)
			}, timeout, interval).Should(Succeed())

			By("deleting the Ray CR")
			ray := &componentsv1alpha1.Ray{}
			Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ray)).To(Succeed())

			Eventually(func() bool {
				return isNotFound(&appsv1.Deployment{}, deploymentName, testNamespace)
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isNotFound(&corev1.ConfigMap{}, configMapName, testNamespace)
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isNotFound(&admissionregistrationv1.MutatingWebhookConfiguration{}, webhookName, "")
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isNotFound(&rbacv1.ClusterRole{}, notebookClusterRoleName, "")
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isNotFound(&componentsv1alpha1.Ray{}, constants.InstanceName, "")
			}, timeout, interval).Should(BeTrue())

			By("verifying the labeled CRD is still kept")
			crd := &apiextensionsv1.CustomResourceDefinition{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: lifecycleCRD}, crd)).To(Succeed())
		})
	})

	Context("CEL singleton validation", func() {
		It("should reject a Ray CR with a name other than default-ray", func() {
			invalid := &componentsv1alpha1.Ray{
				ObjectMeta: metav1.ObjectMeta{
					Name: "not-default-ray",
				},
			}
			err := k8sClient.Create(ctx, invalid)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Ray name must be default-ray"))
		})
	})
})

func newRayCR(applicationsNamespace string) *componentsv1alpha1.Ray {
	return &componentsv1alpha1.Ray{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.InstanceName,
		},
		Spec: componentsv1alpha1.RaySpec{
			ManagementSpec: common.ManagementSpec{
				ManagementState: common.Managed,
			},
			ApplicationsNamespace: applicationsNamespace,
		},
	}
}

// createRayCR creates default-ray. When applicationsNamespace is set, it
// also patches the CR: a same-name recreate after the previous Ordered spec
// can consume the informer Add while Get still returns NotFound, so apply
// never runs. Empty-namespace creates are left alone so that patch does not
// conflict with dropping the unused finalizer.
func createRayCR(applicationsNamespace string) {
	GinkgoHelper()
	Expect(k8sClient.Create(ctx, newRayCR(applicationsNamespace))).To(Succeed())
	if applicationsNamespace == "" {
		return
	}

	Eventually(func(g Gomega) {
		ray := &componentsv1alpha1.Ray{}
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: constants.InstanceName}, ray)).To(Succeed())
		if ray.Annotations[testKickAnnotation] == "1" {
			return
		}
		base := ray.DeepCopy()
		if ray.Annotations == nil {
			ray.Annotations = map[string]string{}
		}
		ray.Annotations[testKickAnnotation] = "1"
		g.Expect(k8sClient.Patch(ctx, ray, client.MergeFrom(base))).To(Succeed())
	}, timeout, interval).Should(Succeed())

	Eventually(func(g Gomega) {
		ray := &componentsv1alpha1.Ray{}
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: constants.InstanceName}, ray)).To(Succeed())
		g.Expect(controllerutil.ContainsFinalizer(ray, constants.FinalizerName)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func inTreeDeployment() *appsv1.Deployment {
	replicas := int32(1)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: testNamespace,
			Labels:    map[string]string{"app": deploymentName},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": deploymentName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": deploymentName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "manager",
						Image: inTreeImage,
					}},
				},
			},
		},
	}
}

func labeledCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: lifecycleCRD,
			Labels: map[string]string{
				gc.DefaultPartOfLabelKey: constants.ComponentName,
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "lifecycle.ray.test.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "fakes",
				Singular: "fake",
				Kind:     "Fake",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    "v1",
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object"},
				},
			}},
		},
	}
}

func isNotFound(obj client.Object, name, namespace string) bool {
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)

	return errors.IsNotFound(err)
}
