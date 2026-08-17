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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

func TestValidateReleaseVersion(t *testing.T) {
	t.Parallel()

	tooLong := strings.Repeat("1", constants.MaxReleaseVersionLength+1)
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{name: "plain semver", version: "2.20.0", wantErr: false},
		{name: "v-prefixed semver", version: "v2.20.0", wantErr: false},
		{name: "invalid semver", version: "not-a-semver", wantErr: true},
		{name: "too long", version: tooLong, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateReleaseVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateReleaseVersion(%q) error = %v, wantErr=%v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestPlatformConfigMapper(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := componentsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Ray scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	ray := &componentsv1alpha1.Ray{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InstanceName},
		Spec: componentsv1alpha1.RaySpec{
			ApplicationsNamespace: "redhat-ods-applications",
		},
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ray).Build()
	mapper := platformConfigMapper(cli)

	reqs := mapper(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.PlatformConfigMapName,
			Namespace: "redhat-ods-applications",
		},
	})
	if len(reqs) != 1 || reqs[0].Name != constants.InstanceName {
		t.Fatalf("expected one request for %q, got %#v", constants.InstanceName, reqs)
	}

	reqs = mapper(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.PlatformConfigMapName,
			Namespace: "other-namespace",
		},
	})
	if len(reqs) != 0 {
		t.Fatalf("expected no requests for configmap in other namespace, got %#v", reqs)
	}

	reqs = mapper(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unrelated",
			Namespace: "redhat-ods-applications",
		},
	})
	if len(reqs) != 0 {
		t.Fatalf("expected no requests for unrelated configmap, got %#v", reqs)
	}
}

func TestPlatformConfigMapperNoRayCR(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := componentsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Ray scheme: %v", err)
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	mapper := platformConfigMapper(cli)

	reqs := mapper(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.PlatformConfigMapName,
			Namespace: "redhat-ods-applications",
		},
	})
	if len(reqs) != 0 {
		t.Fatalf("expected no requests when Ray CR is missing, got %#v", reqs)
	}
}

func TestReadPlatformDistributionMissingConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	dist, status, err := readPlatformDistribution(context.Background(), cli, "redhat-ods-applications")
	if err != nil {
		t.Fatalf("readPlatformDistribution: %v", err)
	}
	if status != distributionAbsent {
		t.Fatalf("status = %v, want absent; dist=%#v", status, dist)
	}
}

func TestReadPlatformDistributionFromConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.PlatformConfigMapName,
			Namespace: "redhat-ods-applications",
		},
		Data: map[string]string{
			constants.PlatformNameKey:    "OpenDataHub",
			constants.PlatformVersionKey: "2.20.0",
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	dist, status, err := readPlatformDistribution(context.Background(), cli, "redhat-ods-applications")
	if err != nil {
		t.Fatalf("readPlatformDistribution: %v", err)
	}
	if status != distributionOK {
		t.Fatalf("status = %v, want OK", status)
	}
	if dist.Name != "OpenDataHub" || dist.Version != "2.20.0" {
		t.Fatalf("dist = %#v, want OpenDataHub/2.20.0", dist)
	}
}

func TestReadPlatformDistributionInvalidVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.PlatformConfigMapName,
			Namespace: "redhat-ods-applications",
		},
		Data: map[string]string{
			constants.PlatformNameKey:    "OpenDataHub",
			constants.PlatformVersionKey: "not-a-semver",
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	_, status, err := readPlatformDistribution(context.Background(), cli, "redhat-ods-applications")
	if err != nil {
		t.Fatalf("readPlatformDistribution: %v", err)
	}
	if status != distributionInvalid {
		t.Fatalf("status = %v, want invalid", status)
	}
}

func TestReadPlatformDistributionEmptyNamespace(t *testing.T) {
	_, status, err := readPlatformDistribution(context.Background(), fake.NewClientBuilder().Build(), "")
	if err != nil {
		t.Fatalf("readPlatformDistribution: %v", err)
	}
	if status != distributionAbsent {
		t.Fatalf("status = %v, want absent", status)
	}
}

func TestReadPlatformDistributionDefaultsName(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.PlatformConfigMapName,
			Namespace: "redhat-ods-applications",
		},
		Data: map[string]string{
			constants.PlatformVersionKey: "3.5.1",
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	dist, status, err := readPlatformDistribution(context.Background(), cli, "redhat-ods-applications")
	if err != nil {
		t.Fatalf("readPlatformDistribution: %v", err)
	}
	if status != distributionOK {
		t.Fatalf("status = %v, want OK", status)
	}
	if dist.Name != constants.StandaloneDistributionName || dist.Version != "3.5.1" {
		t.Fatalf("dist = %#v, want Standalone/3.5.1", dist)
	}
}

func TestStandaloneDistribution(t *testing.T) {
	t.Parallel()

	got := standaloneDistribution("v1.6.2")
	if got.Name != constants.StandaloneDistributionName || got.Version != "v1.6.2" {
		t.Fatalf("standaloneDistribution = %#v", got)
	}

	got = standaloneDistribution("")
	if got.Version != constants.StandaloneDistributionVersion {
		t.Fatalf("empty module version = %q, want 0.0.0", got.Version)
	}
}

func TestPlatformConfigPredicate(t *testing.T) {
	if !platformConfigPredicate(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.PlatformConfigMapName},
	}) {
		t.Fatal("expected predicate to match platform config ConfigMap")
	}
	if platformConfigPredicate(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "other"},
	}) {
		t.Fatal("expected predicate to reject unrelated ConfigMap")
	}
}

func TestDesiredModuleReleases(t *testing.T) {
	t.Parallel()

	releases := desiredModuleReleases("1.4.0")
	if len(releases) != 1 {
		t.Fatalf("expected one KubeRay release, got %#v", releases)
	}
	if releases[0].Name != constants.KubeRayReleaseName || releases[0].Version != "1.4.0" {
		t.Fatalf("unexpected release: %#v", releases[0])
	}
	if releases[0].RepoURL != constants.KubeRayRepoURL {
		t.Fatalf("expected KubeRay repo URL, got %#v", releases[0])
	}

	if got := desiredModuleReleases(""); got != nil {
		t.Fatalf("expected nil releases for empty version, got %#v", got)
	}
}

func TestKubeRayVersionFromImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		image string
		want  string
	}{
		{image: "quay.io/opendatahub/kuberay-operator:v1.6.2", want: "v1.6.2"},
		{image: "registry.redhat.io/kuberay:v2.0", want: "v2.0"},
		{image: "quay.io/opendatahub/kuberay-operator@sha256:abc", want: ""},
		{image: "quay.io/opendatahub/kuberay-operator:v1.6.2@sha256:abc", want: "v1.6.2"},
		{image: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			t.Parallel()

			if got := kubeRayVersionFromImage(tt.image); got != tt.want {
				t.Fatalf("kubeRayVersionFromImage(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

func TestResolveKubeRayImagePrefersRelatedImage(t *testing.T) {
	t.Setenv("RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE", "registry.redhat.io/kuberay:v2.0")

	got := resolveKubeRayImage(testManifestsPath)
	if got != "registry.redhat.io/kuberay:v2.0" {
		t.Fatalf("resolveKubeRayImage = %q, want RELATED_IMAGE override", got)
	}
}

func TestResolveKubeRayImageFallsBackToParams(t *testing.T) {
	t.Setenv("RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE", "")

	got := resolveKubeRayImage(testManifestsPath)
	if got != "quay.io/opendatahub/kuberay-operator:v1.6.2" {
		t.Fatalf("resolveKubeRayImage = %q, want params.env default", got)
	}
}

func TestResolveKubeRayVersionDigestOnlyFallback(t *testing.T) {
	t.Setenv("RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE",
		"registry.redhat.io/rhoai/odh-kuberay-operator-controller-rhel9@sha256:abc123")

	got := resolveKubeRayVersion(testManifestsPath)
	if got != "v1.6.2" {
		t.Fatalf("resolveKubeRayVersion = %q, want v1.6.2 from params.env fallback", got)
	}
}

func TestResolveKubeRayVersionTaggedImage(t *testing.T) {
	t.Setenv("RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE",
		"registry.redhat.io/rhoai/kuberay:v2.0.0")

	got := resolveKubeRayVersion(testManifestsPath)
	if got != "v2.0.0" {
		t.Fatalf("resolveKubeRayVersion = %q, want v2.0.0 from tagged image", got)
	}
}
