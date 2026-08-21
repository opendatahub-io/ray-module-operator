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

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

func distributionAction(_ string) actions.Fn {
	return func(ctx context.Context, rr *types.ReconciliationRequest) error {
		if !conditions.IsStatusConditionTrue(rr.Instance, constants.ConditionDeploymentsAvailable) {
			return nil
		}

		ray := rr.Instance.(*componentsv1alpha1.Ray)

		dist, status, err := readPlatformDistribution(ctx, rr.Client, ray.Spec.ApplicationsNamespace)
		if err != nil {
			return err
		}

		switch status {
		case distributionInvalid:
			logf.FromContext(ctx).Info("skipping status.distribution update: invalid platform version in ConfigMap")
			return nil
		case distributionAbsent:
			dist = standaloneDistribution("")
		}

		if ray.Status.Distribution == dist {
			return nil
		}

		upgrade(ray, dist)

		ray.Status.Distribution = dist

		return nil
	}
}

// upgrade is the hook where version-transition logic will live once the
// platform contract defines an upgrade workflow. Today it is a no-op;
// the comparison in distributionAction still detects ConfigMap drift so
// that status.distribution is kept in sync.
func upgrade(_ *componentsv1alpha1.Ray, _ componentsv1alpha1.Distribution) {}

func degradedAction() actions.Fn {
	return func(_ context.Context, rr *types.ReconciliationRequest) error {
		rr.Conditions.MarkFalse(
			constants.ConditionDegraded,
			conditions.WithReason("AsExpected"),
			conditions.WithMessage("Module is operating normally"),
			conditions.WithObservedGeneration(rr.Instance.GetGeneration()),
		)

		return nil
	}
}

func observedGenerationAction() actions.Fn {
	return func(_ context.Context, rr *types.ReconciliationRequest) error {
		rr.Instance.GetStatus().ObservedGeneration = rr.Instance.GetGeneration()

		return nil
	}
}
