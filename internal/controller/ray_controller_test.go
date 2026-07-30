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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

const (
	testNamespace = "test-ray-ns"
	timeout       = 30 * time.Second
	interval      = 250 * time.Millisecond
)

var _ = Describe("Ray Controller", Ordered, func() {
	rayCR := types.NamespacedName{Name: constants.InstanceName}

	BeforeAll(func() {
		By("creating the test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up the Ray CR")
		ray := &componentsv1alpha1.Ray{}
		err := k8sClient.Get(ctx, rayCR, ray)
		if err == nil {
			Expect(k8sClient.Delete(ctx, ray)).To(Succeed())
		}
	})

	Context("when applicationsNamespace is not projected", func() {
		It("should not deploy any resources", func() {
			By("creating Ray CR without applicationsNamespace")
			ray := &componentsv1alpha1.Ray{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.InstanceName,
				},
				Spec: componentsv1alpha1.RaySpec{
					ManagementSpec: common.ManagementSpec{
						ManagementState: common.Managed,
					},
				},
			}
			Expect(k8sClient.Create(ctx, ray)).To(Succeed())

			By("verifying no deployment is created in the test namespace")
			Consistently(func() bool {
				dep := &appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "kuberay-operator",
					Namespace: testNamespace,
				}, dep)

				return errors.IsNotFound(err)
			}, 5*time.Second, interval).Should(BeTrue())
		})
	})

	Context("when managementState is Managed", func() {
		It("should deploy operand resources", func() {
			By("updating Ray CR with applicationsNamespace")
			ray := &componentsv1alpha1.Ray{}
			Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
			ray.Spec.ApplicationsNamespace = testNamespace
			Expect(k8sClient.Update(ctx, ray)).To(Succeed())

			By("verifying the kuberay-operator Deployment is created")
			Eventually(func() error {
				dep := &appsv1.Deployment{}

				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "kuberay-operator",
					Namespace: testNamespace,
				}, dep)
			}, timeout, interval).Should(Succeed())

			By("verifying the ConfigMap is created")
			Eventually(func() error {
				cm := &corev1.ConfigMap{}

				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "ray-operator-config",
					Namespace: testNamespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

		})
	})

	Context("when managementState is Removed", func() {
		It("should clean up operand resources", func() {
			By("updating managementState to Removed")
			ray := &componentsv1alpha1.Ray{}
			Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())
			ray.Spec.ManagementState = common.Removed
			Expect(k8sClient.Update(ctx, ray)).To(Succeed())

			By("verifying the Deployment is deleted")
			Eventually(func() bool {
				dep := &appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "kuberay-operator",
					Namespace: testNamespace,
				}, dep)

				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("verifying the ConfigMap is deleted")
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "ray-operator-config",
					Namespace: testNamespace,
				}, cm)

				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("verifying DeploymentsAvailable condition is True with RemovedComponent reason")
			Eventually(func(g Gomega) {
				ray := &componentsv1alpha1.Ray{}
				g.Expect(k8sClient.Get(ctx, rayCR, ray)).To(Succeed())

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
