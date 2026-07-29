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
	"os"
	"path/filepath"
	"testing"
)

func writeTestParams(t *testing.T, dir string, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "params.env"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestParams(t *testing.T, dir string) map[string]string {
	t.Helper()
	m, err := parseParams(filepath.Join(dir, "params.env"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestApplyParams_OverridesNamespace(t *testing.T) {
	dir := t.TempDir()
	writeTestParams(t, dir, "namespace=opendatahub\nodh-kuberay-operator-controller-image=quay.io/default:v1\n")

	err := applyParams(dir, "params.env", nil, map[string]string{"namespace": "my-ns"})
	if err != nil {
		t.Fatalf("applyParams: %v", err)
	}

	got := readTestParams(t, dir)
	if got["namespace"] != "my-ns" {
		t.Errorf("namespace = %q, want %q", got["namespace"], "my-ns")
	}
	if got["odh-kuberay-operator-controller-image"] != "quay.io/default:v1" {
		t.Errorf("image should be unchanged, got %q", got["odh-kuberay-operator-controller-image"])
	}
}

func TestApplyParams_EnvVarOverride(t *testing.T) {
	dir := t.TempDir()
	writeTestParams(t, dir, "namespace=opendatahub\nodh-kuberay-operator-controller-image=quay.io/default:v1\nodh-kube-rbac-proxy-image=quay.io/proxy:old\n")

	t.Setenv("RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE", "registry.redhat.io/kuberay:v2.0")
	t.Setenv("RELATED_IMAGE_ODH_KUBE_RBAC_PROXY_IMAGE", "registry.redhat.io/proxy:v2.0")

	err := applyParams(dir, "params.env", imageParamMap)
	if err != nil {
		t.Fatalf("applyParams: %v", err)
	}

	got := readTestParams(t, dir)
	if got["odh-kuberay-operator-controller-image"] != "registry.redhat.io/kuberay:v2.0" {
		t.Errorf("controller image = %q, want override", got["odh-kuberay-operator-controller-image"])
	}
	if got["odh-kube-rbac-proxy-image"] != "registry.redhat.io/proxy:v2.0" {
		t.Errorf("rbac proxy image = %q, want override", got["odh-kube-rbac-proxy-image"])
	}
}

func TestApplyParams_NoOpWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	content := "namespace=opendatahub\nodh-kuberay-operator-controller-image=quay.io/default:v1\n"
	writeTestParams(t, dir, content)

	err := applyParams(dir, "params.env", imageParamMap, map[string]string{"namespace": "opendatahub"})
	if err != nil {
		t.Fatalf("applyParams: %v", err)
	}

	got := readTestParams(t, dir)
	if got["namespace"] != "opendatahub" {
		t.Errorf("namespace = %q, want %q", got["namespace"], "opendatahub")
	}
}

func TestApplyParams_MissingFileReturnsNil(t *testing.T) {
	dir := t.TempDir()

	err := applyParams(dir, "params.env", imageParamMap)
	if err != nil {
		t.Fatalf("expected nil for missing file, got: %v", err)
	}
}
