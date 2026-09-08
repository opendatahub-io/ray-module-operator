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
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type stubMapper struct {
	meta.RESTMapper
	known map[schema.GroupKind]bool
}

func (m *stubMapper) RESTMapping(gk schema.GroupKind, _ ...string) (*meta.RESTMapping, error) {
	if m.known[gk] {
		return &meta.RESTMapping{}, nil
	}
	return nil, &meta.NoKindMatchError{GroupKind: gk}
}

func makeResource(group, kind, name string) unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: "v1", Kind: kind})
	u.SetName(name)
	return u
}

func TestFilterPlatformResources_NoSCCAPI(t *testing.T) {
	mapper := &stubMapper{known: map[schema.GroupKind]bool{}}
	action := filterPlatformResources(mapper)

	rr := &types.ReconciliationRequest{
		Resources: []unstructured.Unstructured{
			makeResource("apps", "Deployment", "kuberay-operator"),
			makeResource("security.openshift.io", "SecurityContextConstraints", "run-as-ray-user"),
			makeResource("", "ConfigMap", "ray-config"),
		},
	}

	if err := action(context.Background(), rr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rr.Resources) != 2 {
		t.Fatalf("expected 2 resources after filtering, got %d", len(rr.Resources))
	}
	for _, r := range rr.Resources {
		if r.GetKind() == "SecurityContextConstraints" {
			t.Error("SCC resource should have been filtered out")
		}
	}
}

func TestFilterPlatformResources_WithSCCAPI(t *testing.T) {
	mapper := &stubMapper{known: map[schema.GroupKind]bool{
		{Group: "security.openshift.io", Kind: "SecurityContextConstraints"}: true,
	}}
	action := filterPlatformResources(mapper)

	rr := &types.ReconciliationRequest{
		Resources: []unstructured.Unstructured{
			makeResource("apps", "Deployment", "kuberay-operator"),
			makeResource("security.openshift.io", "SecurityContextConstraints", "run-as-ray-user"),
			makeResource("", "ConfigMap", "ray-config"),
		},
	}

	if err := action(context.Background(), rr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rr.Resources) != 3 {
		t.Fatalf("expected all 3 resources preserved on OpenShift, got %d", len(rr.Resources))
	}
}

func TestFilterPlatformResources_EmptyResources(t *testing.T) {
	mapper := &stubMapper{known: map[schema.GroupKind]bool{}}
	action := filterPlatformResources(mapper)

	rr := &types.ReconciliationRequest{}

	if err := action(context.Background(), rr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rr.Resources) != 0 {
		t.Fatalf("expected 0 resources, got %d", len(rr.Resources))
	}
}
