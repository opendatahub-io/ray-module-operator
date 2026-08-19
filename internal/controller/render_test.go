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
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestForceNamespacedResources_OverridesHardcodedNamespace(t *testing.T) {
	objs := []unstructured.Unstructured{
		{
			Object: map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]any{
					"name":      "kuberay-operator",
					"namespace": "redhat-ods-applications",
				},
			},
		},
		{
			Object: map[string]any{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "ClusterRole",
				"metadata": map[string]any{
					"name": "kuberay-operator",
				},
			},
		},
	}

	forceNamespacedResources("ray-module-test", objs)

	if got := objs[0].GetNamespace(); got != "ray-module-test" {
		t.Errorf("Deployment namespace = %q, want ray-module-test", got)
	}
	if got := objs[1].GetNamespace(); got != "" {
		t.Errorf("ClusterRole namespace = %q, want empty", got)
	}
}
