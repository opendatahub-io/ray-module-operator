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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
)

var _ = Describe("Ray Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = componentsv1alpha1.RayInstanceName

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Ray")
			ray := &componentsv1alpha1.Ray{}
			err := k8sClient.Get(ctx, typeNamespacedName, ray)
			if errors.IsNotFound(err) {
				resource := &componentsv1alpha1.Ray{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			resource := &componentsv1alpha1.Ray{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Ray")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			// Clean up the ClusterRole if it exists.
			cr := &rbacv1.ClusterRole{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: notebookClusterRoleName}, cr)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			}
		})

		It("should create the notebook ClusterRole when managementState is Managed", func() {
			By("Reconciling the created resource")
			controllerReconciler := &RayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ClusterRole was created")
			clusterRole := &rbacv1.ClusterRole{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: notebookClusterRoleName}, clusterRole)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ClusterRole has correct labels")
			Expect(clusterRole.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "ray-module-operator"))
			Expect(clusterRole.Labels).To(HaveKeyWithValue("app.kubernetes.io/part-of", "ray"))

			By("Verifying the ClusterRole has the correct rules")
			Expect(clusterRole.Rules).To(HaveLen(3))

			// Rule 1: RayJobs access
			Expect(clusterRole.Rules[0].APIGroups).To(Equal([]string{"ray.io"}))
			Expect(clusterRole.Rules[0].Resources).To(Equal([]string{"rayjobs", "rayjobs/status"}))
			Expect(clusterRole.Rules[0].Verbs).To(ConsistOf("get", "list", "create", "patch", "delete"))

			// Rule 2: RayClusters read access
			Expect(clusterRole.Rules[1].APIGroups).To(Equal([]string{"ray.io"}))
			Expect(clusterRole.Rules[1].Resources).To(Equal([]string{"rayclusters", "rayclusters/status"}))
			Expect(clusterRole.Rules[1].Verbs).To(ConsistOf("get", "list"))

			// Rule 3: Secrets create access
			Expect(clusterRole.Rules[2].APIGroups).To(Equal([]string{""}))
			Expect(clusterRole.Rules[2].Resources).To(Equal([]string{"secrets"}))
			Expect(clusterRole.Rules[2].Verbs).To(ConsistOf("create"))
		})

		It("should delete the notebook ClusterRole when managementState is Removed", func() {
			controllerReconciler := &RayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("First reconciling with Managed to create the ClusterRole")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify ClusterRole exists before we remove it.
			clusterRole := &rbacv1.ClusterRole{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: notebookClusterRoleName}, clusterRole)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the Ray resource to Removed")
			ray := &componentsv1alpha1.Ray{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, ray)).To(Succeed())
			ray.Spec.ManagementState = common.Removed
			Expect(k8sClient.Update(ctx, ray)).To(Succeed())

			By("Reconciling again with Removed state")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ClusterRole was deleted")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: notebookClusterRoleName}, clusterRole)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should not error when deleting a ClusterRole that does not exist", func() {
			controllerReconciler := &RayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("Setting the Ray resource to Removed without creating the ClusterRole first")
			ray := &componentsv1alpha1.Ray{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, ray)).To(Succeed())
			ray.Spec.ManagementState = common.Removed
			Expect(k8sClient.Update(ctx, ray)).To(Succeed())

			By("Reconciling — should succeed even though ClusterRole doesn't exist")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("CEL singleton validation", func() {
		ctx := context.Background()

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
