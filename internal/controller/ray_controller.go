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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

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

func (r *RayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&componentsv1alpha1.Ray{}).
		Named("ray").
		Complete(r)
}
