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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

func TestObservedGenerationAction(t *testing.T) {
	ray := &componentsv1alpha1.Ray{
		ObjectMeta: metav1.ObjectMeta{
			Name:       constants.InstanceName,
			Generation: 7,
		},
	}

	rr := &types.ReconciliationRequest{Instance: ray}
	if err := observedGenerationAction()(context.Background(), rr); err != nil {
		t.Fatalf("observedGenerationAction: %v", err)
	}
	if ray.Status.ObservedGeneration != 7 {
		t.Fatalf("observedGeneration = %d, want 7", ray.Status.ObservedGeneration)
	}
}

func TestDegradedAction(t *testing.T) {
	ray := &componentsv1alpha1.Ray{
		ObjectMeta: metav1.ObjectMeta{
			Name:       constants.InstanceName,
			Generation: 3,
		},
	}
	mgr := conditions.NewManager(ray, "Ready", constants.ConditionDeploymentsAvailable, constants.ConditionDegraded)

	rr := &types.ReconciliationRequest{Instance: ray, Conditions: mgr}
	if err := degradedAction()(context.Background(), rr); err != nil {
		t.Fatalf("degradedAction: %v", err)
	}

	c := mgr.GetCondition(constants.ConditionDegraded)
	if c == nil {
		t.Fatal("expected Degraded condition")
	}
	if c.Status != metav1.ConditionFalse {
		t.Fatalf("Degraded status = %s, want False", c.Status)
	}
	if c.Reason != "AsExpected" {
		t.Fatalf("Degraded reason = %q, want AsExpected", c.Reason)
	}
}

func TestDistributionActionWaitsForDeployments(t *testing.T) {
	ray := &componentsv1alpha1.Ray{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InstanceName},
		Spec:       componentsv1alpha1.RaySpec{ApplicationsNamespace: "redhat-ods-applications"},
	}
	ray.Status.Distribution = componentsv1alpha1.Distribution{Name: "kept", Version: "1.0.0"}

	rr := &types.ReconciliationRequest{
		Instance: ray,
		Client:   fake.NewClientBuilder().Build(),
	}
	if err := distributionAction("")(context.Background(), rr); err != nil {
		t.Fatalf("distributionAction: %v", err)
	}
	if ray.Status.Distribution.Name != "kept" {
		t.Fatalf("distribution = %#v, want unchanged while deployments unavailable", ray.Status.Distribution)
	}
}

func TestDistributionActionStandaloneWhenConfigMapAbsent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := componentsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Ray scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	ray := &componentsv1alpha1.Ray{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InstanceName},
		Spec:       componentsv1alpha1.RaySpec{ApplicationsNamespace: "redhat-ods-applications"},
		Status: componentsv1alpha1.RayStatus{
			Status: common.Status{
				Conditions: []common.Condition{{
					Type:   constants.ConditionDeploymentsAvailable,
					Status: metav1.ConditionTrue,
				}},
			},
		},
	}

	rr := &types.ReconciliationRequest{
		Instance: ray,
		Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
	if err := distributionAction(testManifestsPath)(context.Background(), rr); err != nil {
		t.Fatalf("distributionAction: %v", err)
	}
	if ray.Status.Distribution.Name != constants.StandaloneDistributionName {
		t.Fatalf("distribution.name = %q, want Standalone", ray.Status.Distribution.Name)
	}
	if ray.Status.Distribution.Version != "0.0.0" {
		t.Fatalf("distribution.version = %q, want 0.0.0", ray.Status.Distribution.Version)
	}
}

func TestDistributionActionFromConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := componentsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Ray scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.PlatformConfigMapName,
			Namespace: "redhat-ods-applications",
		},
		Data: map[string]string{
			constants.PlatformNameKey:    "SelfManagedRHOAI",
			constants.PlatformVersionKey: "3.5.1",
		},
	}
	ray := &componentsv1alpha1.Ray{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InstanceName},
		Spec:       componentsv1alpha1.RaySpec{ApplicationsNamespace: "redhat-ods-applications"},
		Status: componentsv1alpha1.RayStatus{
			Status: common.Status{
				Conditions: []common.Condition{{
					Type:   constants.ConditionDeploymentsAvailable,
					Status: metav1.ConditionTrue,
				}},
			},
		},
	}

	rr := &types.ReconciliationRequest{
		Instance: ray,
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build(),
	}
	if err := distributionAction("")(context.Background(), rr); err != nil {
		t.Fatalf("distributionAction: %v", err)
	}
	if ray.Status.Distribution.Name != "SelfManagedRHOAI" || ray.Status.Distribution.Version != "3.5.1" {
		t.Fatalf("distribution = %#v, want SelfManagedRHOAI/3.5.1", ray.Status.Distribution)
	}
}

func TestDistributionActionSkipsInvalidVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := componentsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Ray scheme: %v", err)
	}
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
	ray := &componentsv1alpha1.Ray{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InstanceName},
		Spec:       componentsv1alpha1.RaySpec{ApplicationsNamespace: "redhat-ods-applications"},
		Status: componentsv1alpha1.RayStatus{
			Distribution: componentsv1alpha1.Distribution{Name: "kept", Version: "1.0.0"},
			Status: common.Status{
				Conditions: []common.Condition{{
					Type:   constants.ConditionDeploymentsAvailable,
					Status: metav1.ConditionTrue,
				}},
			},
		},
	}

	rr := &types.ReconciliationRequest{
		Instance: ray,
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build(),
	}
	if err := distributionAction("")(context.Background(), rr); err != nil {
		t.Fatalf("distributionAction: %v", err)
	}
	if ray.Status.Distribution.Name != "kept" || ray.Status.Distribution.Version != "1.0.0" {
		t.Fatalf("distribution = %#v, want previous value retained", ray.Status.Distribution)
	}
}
