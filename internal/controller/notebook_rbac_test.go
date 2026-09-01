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

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

func TestNotebookClusterRole(t *testing.T) {
	cr := notebookClusterRole()
	if cr.Name != notebookClusterRoleName {
		t.Fatalf("name = %q, want %q", cr.Name, notebookClusterRoleName)
	}
	if cr.Kind != "ClusterRole" {
		t.Fatalf("kind = %q, want ClusterRole", cr.Kind)
	}
	if cr.APIVersion != "rbac.authorization.k8s.io/v1" {
		t.Fatalf("apiVersion = %q", cr.APIVersion)
	}
	if cr.Labels[gc.DefaultPartOfLabelKey] != constants.ComponentName {
		t.Fatalf("part-of label = %q, want %q", cr.Labels[gc.DefaultPartOfLabelKey], constants.ComponentName)
	}
	if len(cr.Rules) != 3 {
		t.Fatalf("rules = %d, want 3", len(cr.Rules))
	}

	job := cr.Rules[0]
	if got, want := job.APIGroups, []string{"ray.io"}; !equalStrings(got, want) {
		t.Errorf("rayjobs apiGroups = %v, want %v", got, want)
	}
	if got, want := job.Resources, []string{"rayjobs", "rayjobs/status"}; !equalStrings(got, want) {
		t.Errorf("rayjobs resources = %v, want %v", got, want)
	}
	if got, want := job.Verbs, []string{"get", "list", "create", "patch", "delete"}; !equalStrings(got, want) {
		t.Errorf("rayjobs verbs = %v, want %v", got, want)
	}

	cluster := cr.Rules[1]
	if got, want := cluster.Resources, []string{"rayclusters", "rayclusters/status"}; !equalStrings(got, want) {
		t.Errorf("rayclusters resources = %v, want %v", got, want)
	}
	if got, want := cluster.Verbs, []string{"get", "list"}; !equalStrings(got, want) {
		t.Errorf("rayclusters verbs = %v, want %v", got, want)
	}

	secrets := cr.Rules[2]
	if got, want := secrets.APIGroups, []string{""}; !equalStrings(got, want) {
		t.Errorf("secrets apiGroups = %v, want %v", got, want)
	}
	if got, want := secrets.Resources, []string{"secrets"}; !equalStrings(got, want) {
		t.Errorf("secrets resources = %v, want %v", got, want)
	}
	if got, want := secrets.Verbs, []string{"create"}; !equalStrings(got, want) {
		t.Errorf("secrets verbs = %v, want %v", got, want)
	}

	if cr.Namespace != "" {
		t.Fatalf("ClusterRole namespace = %q, want empty", cr.Namespace)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
