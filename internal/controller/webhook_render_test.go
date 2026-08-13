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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/kustomize"
)

const (
	renderTestdataPath = "testdata/render"
	testTargetNS       = "test-apps"
)

func renderWebhookTestdata(t *testing.T) map[gvk][]unstructured.Unstructured {
	t.Helper()
	overlayPath := filepath.Join(renderTestdataPath, "kuberay", "openshift")
	overlayPath = copyRenderTestdata(t, overlayPath)

	if err := applyParams(overlayPath, nil, map[string]string{"namespace": testTargetNS}); err != nil {
		t.Fatalf("applyParams failed: %v", err)
	}

	engine := kustomize.NewEngine()
	resources, err := engine.Render(overlayPath, kustomize.WithNamespace(testTargetNS))
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	return indexByGVK(resources)
}

func TestWebhookRender_MutatingWebhookConfiguration(t *testing.T) {
	idx := renderWebhookTestdata(t)
	key := gvk{"admissionregistration.k8s.io/v1", "MutatingWebhookConfiguration"}
	mwcs := idx[key]
	if len(mwcs) != 1 {
		t.Fatalf("expected 1 MutatingWebhookConfiguration, got %d", len(mwcs))
	}
	mwc := mwcs[0]

	if name := mwc.GetName(); name != "kuberay-mutating-webhook-configuration" {
		t.Errorf("name = %q, want %q", name, "kuberay-mutating-webhook-configuration")
	}

	annotations := mwc.GetAnnotations()
	if v, ok := annotations["cert-manager.io/inject-ca-from"]; !ok {
		t.Error("missing cert-manager.io/inject-ca-from annotation")
	} else if want := testTargetNS + "/serving-cert"; v != want {
		t.Errorf("cert-manager.io/inject-ca-from = %q, want %q", v, want)
	}

	assertNoServingCertAnnotations(t, annotations)
}

func TestWebhookRender_NoValidatingWebhookConfiguration(t *testing.T) {
	idx := renderWebhookTestdata(t)
	key := gvk{"admissionregistration.k8s.io/v1", "ValidatingWebhookConfiguration"}
	if vwcs := idx[key]; len(vwcs) != 0 {
		t.Errorf("expected 0 ValidatingWebhookConfigurations, got %d", len(vwcs))
	}
}

func TestWebhookRender_Certificate(t *testing.T) {
	idx := renderWebhookTestdata(t)
	key := gvk{"cert-manager.io/v1", "Certificate"}
	certs := idx[key]
	if len(certs) != 1 {
		t.Fatalf("expected 1 Certificate, got %d", len(certs))
	}
	cert := certs[0]

	secretName, _, _ := unstructured.NestedString(cert.Object, "spec", "secretName")
	if secretName != "kuberay-webhook-server-cert" {
		t.Errorf("secretName = %q, want %q", secretName, "kuberay-webhook-server-cert")
	}

	dnsNames, _, _ := unstructured.NestedStringSlice(cert.Object, "spec", "dnsNames")
	wantDNS := []string{
		"kuberay-webhook-service." + testTargetNS + ".svc",
		"kuberay-webhook-service." + testTargetNS + ".svc.cluster.local",
	}
	if !stringSliceEqual(dnsNames, wantDNS) {
		t.Errorf("dnsNames = %v, want %v", dnsNames, wantDNS)
	}
}

func TestWebhookRender_Issuer(t *testing.T) {
	idx := renderWebhookTestdata(t)
	key := gvk{"cert-manager.io/v1", "Issuer"}
	issuers := idx[key]
	if len(issuers) != 1 {
		t.Fatalf("expected 1 Issuer, got %d", len(issuers))
	}
	if name := issuers[0].GetName(); name != "selfsigned-issuer" {
		t.Errorf("name = %q, want %q", name, "selfsigned-issuer")
	}
}

func TestWebhookRender_WebhookService(t *testing.T) {
	idx := renderWebhookTestdata(t)
	svc := findResource(t, idx, gvk{"v1", "Service"}, "kuberay-webhook-service")

	ports, _, _ := unstructured.NestedSlice(svc.Object, "spec", "ports")
	if len(ports) == 0 {
		t.Fatal("expected at least one port on webhook service")
	}
	p := ports[0].(map[string]any)
	port, _, _ := unstructured.NestedInt64(p, "port")
	targetPort, _, _ := unstructured.NestedInt64(p, "targetPort")
	if port != 443 {
		t.Errorf("port = %d, want 443", port)
	}
	if targetPort != 9443 {
		t.Errorf("targetPort = %d, want 9443", targetPort)
	}

	assertNoServingCertAnnotations(t, svc.GetAnnotations())
}

func TestWebhookRender_Deployment(t *testing.T) {
	idx := renderWebhookTestdata(t)
	dep := findResource(t, idx, gvk{"apps/v1", "Deployment"}, "kuberay-operator")

	containers, _, _ := unstructured.NestedSlice(dep.Object,
		"spec", "template", "spec", "containers")
	if len(containers) == 0 {
		t.Fatal("no containers in deployment")
	}

	c := containers[0].(map[string]any)

	assertEnvVar(t, c, "ENABLE_WEBHOOKS", "true")
	assertContainerPort(t, c, 9443)
	assertCertVolume(t, dep)
}

func TestWebhookRender_NoNamespaceResource(t *testing.T) {
	idx := renderWebhookTestdata(t)
	key := gvk{"v1", "Namespace"}
	if ns := idx[key]; len(ns) != 0 {
		t.Errorf("expected 0 Namespace resources, got %d", len(ns))
	}
}

func findResource(t *testing.T, idx map[gvk][]unstructured.Unstructured, key gvk, name string) unstructured.Unstructured {
	t.Helper()
	for _, r := range idx[key] {
		if r.GetName() == name {
			return r
		}
	}
	t.Fatalf("%s/%s %q not found", key.apiVersion, key.kind, name)
	return unstructured.Unstructured{}
}

func assertEnvVar(t *testing.T, container map[string]any, name, value string) {
	t.Helper()
	envVars, _, _ := unstructured.NestedSlice(container, "env")
	for _, e := range envVars {
		em := e.(map[string]any)
		if em["name"] == name && em["value"] == value {
			return
		}
	}
	t.Errorf("env var %s=%s not found", name, value)
}

func assertContainerPort(t *testing.T, container map[string]any, want int64) {
	t.Helper()
	ports, _, _ := unstructured.NestedSlice(container, "ports")
	for _, p := range ports {
		pm := p.(map[string]any)
		if port, ok := pm["containerPort"]; ok {
			if portVal, ok := port.(int64); ok && portVal == want {
				return
			}
		}
	}
	t.Errorf("containerPort %d not found", want)
}

func assertCertVolume(t *testing.T, dep unstructured.Unstructured) {
	t.Helper()
	volumes, _, _ := unstructured.NestedSlice(dep.Object,
		"spec", "template", "spec", "volumes")
	for _, v := range volumes {
		vm := v.(map[string]any)
		if sec, ok := vm["secret"].(map[string]any); ok {
			if sec["secretName"] == "kuberay-webhook-server-cert" {
				return
			}
		}
	}
	t.Error("volume with secretName kuberay-webhook-server-cert not found")
}

type gvk struct {
	apiVersion string
	kind       string
}

func indexByGVK(resources []unstructured.Unstructured) map[gvk][]unstructured.Unstructured {
	m := make(map[gvk][]unstructured.Unstructured)
	for _, r := range resources {
		key := gvk{r.GetAPIVersion(), r.GetKind()}
		m[key] = append(m[key], r)
	}
	return m
}

func assertNoServingCertAnnotations(t *testing.T, annotations map[string]string) {
	t.Helper()
	for k := range annotations {
		if k == "service.beta.openshift.io/serving-cert-secret-name" ||
			k == "service.beta.openshift.io/inject-cabundle" {
			t.Errorf("unexpected OpenShift serving-cert annotation: %s", k)
		}
	}
}

func stringSliceEqual(a, b []string) bool {
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

func copyRenderTestdata(t *testing.T, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "openshift")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}
