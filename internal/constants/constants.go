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

package constants

const (
	InstanceName  = "default-ray"
	FieldOwner    = "ray"
	ComponentName = "ray"

	ManifestsBasePath = "/opt/manifests"
	ManifestPath      = "kuberay"
	ManifestOverlay   = "openshift"

	ConditionDeploymentsAvailable = "DeploymentsAvailable"
	ConditionDegraded             = "Degraded"

	ExtKeyRemoved = "ray.removed"

	// PlatformConfigMapName is the ConfigMap the module operator reads for
	// distribution handshake. In production the platform operator overwrites
	// these values; the module ships a default copy for standalone operation.
	PlatformConfigMapName = "opendatahub-ray-config"
	PlatformNameKey       = "distribution.name"
	PlatformVersionKey    = "distribution.version"

	StandaloneDistributionName = "Standalone"

	StandaloneDistributionVersion = "0.0.0"

	KubeRayReleaseName = "KubeRay"
	KubeRayRepoURL     = "https://github.com/opendatahub-io/kuberay"
	KubeRayImageParam  = "odh-kuberay-operator-controller-image"

	MaxReleaseVersionLength = 64
)
