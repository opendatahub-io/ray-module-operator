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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

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
// Cluster-scoped Ray CRs record events in namespace "default".
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// Operand ClusterRoles (kuberay-edit-role) grant these; the module SA must hold them to apply those roles.
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters;rayjobs;rayservices;raycronjobs,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters/finalizers;rayjobs/finalizers;rayservices/finalizers;raycronjobs/finalizers,verbs=get;update
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters/status;rayjobs/status;rayservices/status;raycronjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// SetupWithManager wires the Ray reconciler pipeline into the controller
// manager. The pipeline is: management state → releases → manifest init →
// kustomize render → deploy (SSA, ForceOwnership) → deployment status →
// distribution → Degraded → observedGeneration → garbage collect.
// WithFinalizer cleans owned operands on Ray CR deletion (webhooks, SCC); CRDs stay.
func SetupWithManager(ctx context.Context, mgr ctrl.Manager, manifestsBasePath string) error {
	nsFn := namespaceFn

	_, err := reconciler.ReconcilerFor(mgr, &componentsv1alpha1.Ray{}).
		WithInstanceName(constants.InstanceName).
		WithConditions(
			string(common.ConditionTypeProvisioningSucceeded),
			constants.ConditionDeploymentsAvailable,
			constants.ConditionDegraded,
		).
		WithDynamicOwnership().
		Watches(
			&corev1.ConfigMap{},
			reconciler.WithEventMapper(platformConfigMapper(mgr.GetClient())),
			reconciler.WithPredicates(predicate.NewPredicateFuncs(platformConfigPredicate)),
		).
		WithReconcilerOpts(
			reconciler.WithRelease(frameworkapi.Release{Name: "opendatahub"}),
			reconciler.WithFinalizerName(constants.FinalizerName),
			reconciler.WithPreApplyFn(waitForNamespace),
			reconciler.WithPreApplyFailedReason("ApplicationsNamespaceNotProjected"),
		).
		WithAction(managementStateAction()).
		WithAction(releasesAction(manifestsBasePath)).
		WithAction(manifestInitAction()).
		WithAction(applyImageParamsAction(manifestsBasePath)).
		WithAction(RenderKustomize(manifestsBasePath, nsFn)).
		WithAction(deploy.NewAction(
			deploy.WithFieldOwner(constants.FieldOwner),
			deploy.WithPartOfLabelDefault(constants.ComponentName),
			deploy.WithApplyOrder(),
		)).
		WithAction(deploymentStatusAction(nsFn)).
		WithAction(distributionAction(manifestsBasePath)).
		WithAction(degradedAction()).
		WithAction(observedGenerationAction()).
		WithAction(gc.NewAction(
			nsFn,
			gc.WithDeletePropagationPolicy(metav1.DeletePropagationBackground),
			// Removed (generation bump) must collect labeled webhooks/SCC even
			// when they have no ownerRef. Managed is still gated by the default
			// generation predicate. OwnerRefs are possible (cluster-scoped Ray +
			// dynamic ownership) but cascade waits on the CR finalizer, and
			// in-tree leftovers may only carry the part-of label.
			gc.WithOnlyCollectOwned(false),
		)).
		WithFinalizer(deletionCleanupAction(nsFn)).
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

// deletionCleanupAction runs on Ray CR deletion. The framework delete path
// does not run the reconcile pipeline, so Generated is false and the default
// GC generation predicate would skip current operands. Force Generated and
// delete labeled operands (webhooks, SCC) even without ownerRef; CRDs stay.
// Empty applicationsNamespace means nothing was deployed — skip GC so the
// finalizer cannot stick.
func deletionCleanupAction(nsFn actions.Getter[string]) actions.Fn {
	gcFn := gc.NewAction(
		nsFn,
		gc.WithDeletePropagationPolicy(metav1.DeletePropagationBackground),
		gc.WithOnlyCollectOwned(false),
		gc.WithObjectPredicate(func(_ *types.ReconciliationRequest, _ unstructured.Unstructured) (bool, error) {
			return true, nil
		}),
	)

	return func(ctx context.Context, rr *types.ReconciliationRequest) error {
		ns, err := nsFn(ctx, rr)
		if err != nil {
			return err
		}
		if ns == "" {
			return nil
		}

		rr.Generated = true

		return gcFn(ctx, rr)
	}
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
