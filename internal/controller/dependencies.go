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

	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/sanitycheck"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

var (
	codeFlareGVK = schema.GroupVersionKind{
		Group:   "components.platform.opendatahub.io",
		Version: "v1alpha1",
		Kind:    "CodeFlare",
	}

	certManagerGVKs = []schema.GroupVersionKind{
		{Group: "cert-manager.io", Version: "v1", Kind: "Issuer"},
		{Group: "cert-manager.io", Version: "v1", Kind: "Certificate"},
	}
)

func codeFlareSanityCheckAction() actions.Fn {
	check := sanitycheck.NewAction(
		sanitycheck.WithUnwantedResource(codeFlareGVK, constants.CodeFlarePresentMessage),
	)

	return func(ctx context.Context, rr *types.ReconciliationRequest) error {
		if rr.Extensions != nil {
			if removed, _ := rr.Extensions[constants.ExtKeyRemoved].(bool); removed {
				return nil
			}
		}

		return check(ctx, rr)
	}
}

func certManagerRequirementAction() actions.Fn {
	return func(ctx context.Context, rr *types.ReconciliationRequest) error {
		if rr.Extensions != nil {
			if removed, _ := rr.Extensions[constants.ExtKeyRemoved].(bool); removed {
				return nil
			}
		}

		required := requiredCertManagerGVKs(rr.Resources)
		for _, gvk := range required {
			available, err := cluster.HasCRD(ctx, rr.Client, gvk)
			if err != nil {
				return fmt.Errorf("check cert-manager %s CRD: %w", gvk.Kind, err)
			}
			if !available {
				return fmt.Errorf("cert-manager is required by the Ray module: %s CRD is not installed", gvk)
			}
		}

		return nil
	}
}

func requiredCertManagerGVKs(resources []unstructured.Unstructured) []schema.GroupVersionKind {
	required := make([]schema.GroupVersionKind, 0, len(certManagerGVKs))
	for _, requiredGVK := range certManagerGVKs {
		for _, resource := range resources {
			if resource.GroupVersionKind() == requiredGVK {
				required = append(required, requiredGVK)
				break
			}
		}
	}

	return required
}
