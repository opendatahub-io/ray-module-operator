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
	fwpredicates "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
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
// Cluster-scoped Ray CRs record events in namespace "default". Operand
// kuberay-operator ClusterRole also lists events; privilege-escalation
// requires matching verbs on the module SA.
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// Operand ClusterRoles the module SSA-applies. Kubernetes privilege-escalation
// rules require the applier to already hold the same verbs.
// +kubebuilder:rbac:groups="",resources=pods;pods/status;pods/proxy;pods/resize;secrets;services/proxy;services/status,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates;issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways;httproutes;referencegrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses;ingressclasses;networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.openshift.io,resources=kubeapiservers;kubeapiservers/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters;rayjobs;rayservices;raycronjobs,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters/finalizers;rayjobs/finalizers;rayservices/finalizers;raycronjobs/finalizers,verbs=get;update
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters/status;rayjobs/status;rayservices/status;raycronjobs/status,verbs=get;list;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// SetupWithManager wires the Ray reconciler pipeline into the controller
// manager. The pipeline is: management state → releases → manifest init →
// kustomize render → notebook ClusterRole → deploy (SSA, ForceOwnership) → deployment status →
// distribution → Degraded → observedGeneration → garbage collect.
// WithFinalizer cleans owned operands on Ray CR deletion (webhooks, SCC); CRDs stay.
func SetupWithManager(ctx context.Context, mgr ctrl.Manager, manifestsBasePath string) error {
	nsFn := namespaceFn

	_, err := reconciler.ReconcilerFor(mgr, &componentsv1alpha1.Ray{}, builder.WithPredicates(predicate.Or(
		fwpredicates.DefaultPredicate,
		deletionTimestampSetPredicate(),
	))).
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
		WithAction(notebookClusterRoleAction()).
		WithAction(deploy.NewAction(
			deploy.WithFieldOwner(constants.FieldOwner),
			deploy.WithPartOfLabelDefault(constants.ComponentName),
			deploy.WithApplyOrder(),
		)).
		WithAction(deploymentStatusAction(nsFn)).
		WithAction(distributionAction(manifestsBasePath)).
		WithAction(degradedAction()).
		WithAction(observedGenerationAction()).
		WithAction(reconcileGCAction(nsFn)).
		WithFinalizer(deletionCleanupAction(nsFn)).
		Build(ctx)

	return err
}

// deletionTimestampSetPredicate catches kubectl/client deletes. Framework
// DefaultPredicate only fires on generation/label/annotation changes, so a
// delete that only sets deletionTimestamp never ran the finalizer path —
// which stuck "delete while already Removed" in envtest/CI.
func deletionTimestampSetPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}

			return e.ObjectOld.GetDeletionTimestamp().IsZero() && !e.ObjectNew.GetDeletionTimestamp().IsZero()
		},
	}
}

func namespaceFn(_ context.Context, rr *types.ReconciliationRequest) (string, error) {
	ray := rr.Instance.(*componentsv1alpha1.Ray)

	return ray.Spec.ApplicationsNamespace, nil
}

func waitForNamespace(ctx context.Context, rr *types.ReconciliationRequest) bool {
	ray := rr.Instance.(*componentsv1alpha1.Ray)
	if ray.Spec.ApplicationsNamespace != "" {
		return false
	}

	// The framework adds the finalizer before PreApply. Drop it while nothing
	// is deployed; otherwise a stale apply after Delete never takes the
	// delete path and the CR sticks.
	if controllerutil.RemoveFinalizer(ray, constants.FinalizerName) {
		if err := rr.Client.Update(ctx, ray); err != nil && !k8serr.IsConflict(err) && !k8serr.IsNotFound(err) {
			return true
		}
	}

	return true
}

// reconcileGCAction uses owned-only GC while Managed so kube-generated
// children (EndpointSlices) and unlabeled leftovers are not deleted on
// every pass. Removed still collects labeled webhooks/SCC without ownerRef.
func reconcileGCAction(nsFn actions.Getter[string]) actions.Fn {
	bg := gc.WithDeletePropagationPolicy(metav1.DeletePropagationBackground)
	managedGC := gc.NewAction(nsFn, bg)
	removedGC := gc.NewAction(nsFn, bg,
		gc.WithOnlyCollectOwned(false),
		gc.WithObjectPredicate(func(_ *types.ReconciliationRequest, _ unstructured.Unstructured) (bool, error) {
			return true, nil
		}),
	)
	return func(ctx context.Context, rr *types.ReconciliationRequest) error {
		removed, _ := rr.Extensions[constants.ExtKeyRemoved].(bool)
		if removed {
			return removedGC(ctx, rr)
		}

		return managedGC(ctx, rr)
	}
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

		if err := deleteNotebookClusterRole(ctx, rr); err != nil {
			return err
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
