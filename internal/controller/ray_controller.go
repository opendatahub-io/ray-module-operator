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

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	frameworkapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/deployments"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

// RBAC for the Ray module CR itself.
//
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=rays,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=rays/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=rays/finalizers,verbs=update

// RBAC for operand resources deployed by the reconciler.
//
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers;certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// SetupWithManager wires the Ray reconciler pipeline into the controller
// manager. The pipeline is: management state → manifest init → kustomize
// render → deploy (SSA) → deployment status → garbage collect.
func SetupWithManager(ctx context.Context, mgr ctrl.Manager, manifestsBasePath string) error {
	nsFn := namespaceFn

	_, err := reconciler.ReconcilerFor(mgr, &componentsv1alpha1.Ray{}).
		WithInstanceName(constants.InstanceName).
		WithConditions(constants.ConditionDeploymentsAvailable).
		WithDynamicOwnership().
		WithReconcilerOpts(
			reconciler.WithRelease(frameworkapi.Release{Name: "opendatahub"}),
			reconciler.WithPreApplyFn(waitForNamespace),
			reconciler.WithPreApplyFailedReason("ApplicationsNamespaceNotProjected"),
		).
		WithAction(managementStateAction()).
		WithAction(manifestInitAction()).
		WithAction(applyImageParamsAction(manifestsBasePath)).
		WithAction(RenderKustomize(manifestsBasePath, nsFn)).
		WithAction(deploy.NewAction(
			deploy.WithFieldOwner(constants.FieldOwner),
			deploy.WithPartOfLabelDefault(constants.ComponentName),
			deploy.WithApplyOrder(),
		)).
		WithAction(deploymentStatusAction(nsFn)).
		WithAction(gc.NewAction(
			nsFn,
			gc.WithDeletePropagationPolicy(metav1.DeletePropagationBackground),
		)).
		Build(ctx)

	return err
}

func namespaceFn(_ context.Context, rr *types.ReconciliationRequest) (string, error) {
	ray := rr.Instance.(*componentsv1alpha1.Ray)

	return ray.Spec.ApplicationsNamespace, nil
}

func waitForNamespace(_ context.Context, rr *types.ReconciliationRequest) bool {
	ray := rr.Instance.(*componentsv1alpha1.Ray)

	return ray.Spec.ApplicationsNamespace == ""
}

func managementStateAction() actions.Fn {
	return func(_ context.Context, rr *types.ReconciliationRequest) error {
		if rr.Extensions == nil {
			rr.Extensions = make(map[string]any)
		}

		ray := rr.Instance.(*componentsv1alpha1.Ray)

		switch ray.Spec.ManagementState {
		case common.Removed:
			rr.Extensions[constants.ExtKeyRemoved] = true
			rr.Generated = true
		default:
			rr.Extensions[constants.ExtKeyRemoved] = false
		}

		return nil
	}
}

func manifestInitAction() actions.Fn {
	return func(_ context.Context, rr *types.ReconciliationRequest) error {
		removed, _ := rr.Extensions[constants.ExtKeyRemoved].(bool)

		if removed {
			rr.Manifests = nil
			rr.Resources = nil

			return nil
		}

		rr.Manifests = []types.ManifestInfo{
			{Path: constants.ManifestPath, ContextDir: constants.ManifestOverlay},
		}

		return nil
	}
}

func deploymentStatusAction(nsFn actions.Getter[string]) actions.Fn {
	fwAction := deployments.NewAction(
		deployments.InNamespaceFn(nsFn),
	)

	return func(ctx context.Context, rr *types.ReconciliationRequest) error {
		removed, _ := rr.Extensions[constants.ExtKeyRemoved].(bool)

		if removed {
			rr.Conditions.MarkTrue(
				constants.ConditionDeploymentsAvailable,
				conditions.WithReason("RemovedComponent"),
				conditions.WithMessage("Component has been removed"),
			)

			return nil
		}

		return fwAction(ctx, rr)
	}
}
