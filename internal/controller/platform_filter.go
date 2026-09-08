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
	"sync"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
)

var sccGVK = schema.GroupVersionKind{
	Group:   "security.openshift.io",
	Version: "v1",
	Kind:    "SecurityContextConstraints",
}

// filterPlatformResources returns an action that removes OpenShift-specific
// resources (SecurityContextConstraints) from rr.Resources when the cluster
// does not have the corresponding API group registered.
//
// The check is performed once on the first reconcile and cached for the
// lifetime of the process — the cluster type does not change at runtime.
func filterPlatformResources(mapper meta.RESTMapper) actions.Fn {
	var (
		once    sync.Once
		hasSCCs bool
	)

	return func(_ context.Context, rr *types.ReconciliationRequest) error {
		once.Do(func() {
			_, err := mapper.RESTMapping(sccGVK.GroupKind(), sccGVK.Version)
			hasSCCs = err == nil
			if !hasSCCs {
				ctrl.Log.WithName("platform-filter").Info(
					"SecurityContextConstraints API not available, SCC resources will be skipped",
				)
			}
		})

		if hasSCCs {
			return nil
		}

		filtered := rr.Resources[:0]
		for _, r := range rr.Resources {
			if r.GetKind() == sccGVK.Kind && r.GroupVersionKind().Group == sccGVK.Group {
				continue
			}
			filtered = append(filtered, r)
		}
		rr.Resources = filtered

		return nil
	}
}
