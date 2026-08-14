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
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
)

// RayReconciler reconciles a Ray object
type RayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// RBAC: cluster-scoped permissions only. Namespace-scoped operand
// permissions are in config/rbac/namespace_role.yaml (hand-authored).
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=rays,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=rays/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=rays/finalizers,verbs=update
// TODO(RHOAIENG-64546): create;update;patch added when CRD bootstrap reconciliation lands
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete

func (r *RayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Fetch the Ray custom resource from the cluster.
	//    This is the object that tells us whether Ray is "Managed" or "Removed".
	ray := &componentsv1alpha1.Ray{}
	if err := r.Get(ctx, req.NamespacedName, ray); err != nil {
		if errors.IsNotFound(err) {
			// The Ray resource was deleted — nothing to reconcile.
			log.Info("Ray resource not found, skipping reconciliation")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to fetch Ray resource: %w", err)
	}

	// 2. Check the managementState and act accordingly.
	switch ray.Spec.ManagementState {
	case common.Managed:
		// Ray is active — ensure the notebook ClusterRole exists.
		if err := r.ensureNotebookClusterRole(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure notebook ClusterRole: %w", err)
		}
		log.Info("Notebook ClusterRole reconciled successfully")

	case common.Removed:
		// Ray is being torn down — delete the notebook ClusterRole.
		if err := r.deleteNotebookClusterRole(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete notebook ClusterRole: %w", err)
		}
		log.Info("Notebook ClusterRole deleted successfully")
	}

	return ctrl.Result{}, nil
}

// notebookClusterRoleName is the stable name that the Workbenches team keys
// off to bind permissions to Notebook Service Accounts. Do not rename.
const notebookClusterRoleName = "ray"

// ensureNotebookClusterRole creates or updates the ClusterRole that grants
// Workbench Notebook Service Accounts permission to interact with Ray resources.
func (r *RayReconciler) ensureNotebookClusterRole(ctx context.Context) error {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: notebookClusterRoleName,
		},
	}

	// CreateOrUpdate will:
	//   - Create the ClusterRole if it doesn't exist
	//   - Update it if it exists but the rules have changed
	// The function we pass (the "mutate" function) sets the desired state.
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, clusterRole, func() error {
		clusterRole.Labels = map[string]string{
			"app.kubernetes.io/managed-by": "ray-module-operator",
			"app.kubernetes.io/part-of":    "ray",
		}

		clusterRole.Rules = []rbacv1.PolicyRule{
			{
				// Allow notebooks to create, read, patch, and delete RayJobs.
				// This is the primary use case: submitting distributed workloads.
				APIGroups: []string{"ray.io"},
				Resources: []string{"rayjobs", "rayjobs/status"},
				Verbs:     []string{"get", "list", "create", "patch", "delete"},
			},
			{
				// Allow notebooks to read RayClusters for status checking.
				// They don't need to create or modify clusters directly.
				APIGroups: []string{"ray.io"},
				Resources: []string{"rayclusters", "rayclusters/status"},
				Verbs:     []string{"get", "list"},
			},
			{
				// Allow notebooks to create secrets (used by the CodeFlare SDK
				// for passing credentials to Ray workers).
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"create"},
			},
		}

		return nil
	})

	return err
}

// deleteNotebookClusterRole removes the notebook ClusterRole when Ray is no
// longer managed.
func (r *RayReconciler) deleteNotebookClusterRole(ctx context.Context) error {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: notebookClusterRoleName,
		},
	}

	err := r.Delete(ctx, clusterRole)
	if errors.IsNotFound(err) {
		// Already gone — nothing to do.
		return nil
	}

	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *RayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&componentsv1alpha1.Ray{}).
		Named("ray").
		Complete(r)
}
