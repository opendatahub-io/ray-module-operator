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

package v1alpha1

import (
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const RayInstanceName = "default-ray"

// RaySpec defines the desired state of the Ray module.
type RaySpec struct {
	common.ManagementSpec `json:",inline"`

	// ApplicationsNamespace is the namespace where the module operator deploys
	// operand resources. Projected by the platform operator; immutable once set.
	// +optional
	ApplicationsNamespace string `json:"applicationsNamespace,omitempty"`
}

// Distribution describes the platform distribution context the module
// is currently aligned to. The module operator populates this from the
// ConfigMap values after completing any required upgrade process.
//
// +kubebuilder:object:generate=true
type Distribution struct {
	// Name is the distribution name (e.g., SelfManagedRHOAI, OpenDataHub, Standalone).
	// +optional
	Name string `json:"name,omitempty"`
	// Version is the distribution version (e.g., 3.5.1, 0.0.0).
	// +optional
	Version string `json:"version,omitempty"`
}

// RayStatus defines the observed state of the Ray module.
type RayStatus struct {
	common.Status                 `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`

	// Distribution is the platform distribution context this module is aligned to.
	// +optional
	Distribution Distribution `json:"distribution,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-ray'",message="Ray name must be default-ray"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.spec.applicationsNamespace) || oldSelf.spec.applicationsNamespace == '' || self.spec.applicationsNamespace == oldSelf.spec.applicationsNamespace",message="applicationsNamespace is immutable once set"

// Ray is the module CR reconciled by the Ray module operator.
type Ray struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RaySpec   `json:"spec,omitempty"`
	Status RayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RayList contains a list of Ray.
type RayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Ray `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Ray{}, &RayList{})
}

func (r *Ray) GetStatus() *common.Status {
	return &r.Status.Status
}

func (r *Ray) GetConditions() []common.Condition {
	return r.Status.Conditions
}

func (r *Ray) SetConditions(conditions []common.Condition) {
	r.Status.Conditions = conditions
}

func (r *Ray) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &r.Status.ComponentReleaseStatus
}

func (r *Ray) SetReleaseStatus(status common.ComponentReleaseStatus) {
	r.Status.ComponentReleaseStatus = status
}

var _ common.PlatformObject = &Ray{}
