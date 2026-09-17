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

	CodeFlarePresentMessage = `Failed upgrade: CodeFlare component is present in the cluster. It must be uninstalled to proceed with Ray component upgrade.
To uninstall it, you should delete all RayClusters resources from the cluster, delete the CodeFlare component resource and recreate the RayClusters.`

	// FinalizerName is the platform-contract finalizer. It keeps the Ray CR
	// alive until owned operands (including cluster-scoped webhooks/SCC) are
	// cleaned up. CRDs are not removed.
	FinalizerName = "platform.opendatahub.io/finalizer"

	// PlatformConfigMapName is the per-module ConfigMap managed by the platform
	// operator. It contains both module-specific distribution data and the
	// platform-version handshake value.
	PlatformConfigMapName = "odh-ray-config"
	PlatformNameKey       = "distribution.name"
	PlatformVersionKey    = "distribution.version"
	PlatformHandshakeKey  = "platformVersion"

	StandaloneDistributionName = "Standalone"

	StandaloneDistributionVersion = "0.0.0"

	KubeRayReleaseName = "KubeRay"
	KubeRayRepoURL     = "https://github.com/opendatahub-io/kuberay"
	KubeRayImageParam  = "odh-kuberay-operator-controller-image"

	MaxReleaseVersionLength = 64
)
