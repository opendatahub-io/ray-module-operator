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
	"os"
	"path/filepath"
	"strings"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

func releasesAction(basePath string) actions.Fn {
	return func(_ context.Context, rr *types.ReconciliationRequest) error {
		ray := rr.Instance.(*componentsv1alpha1.Ray)
		ray.SetReleaseStatus(common.ComponentReleaseStatus{
			Releases: desiredModuleReleases(resolveKubeRayVersion(basePath)),
		})

		return nil
	}
}

// resolveKubeRayVersion returns the KubeRay version for status.releases.
// It first tries to extract a tag from the resolved deploy image (which may
// come from RELATED_IMAGE_*). If that image is digest-only (the RHOAI/CPaaS
// pin shape), it falls back to the tagged default in params.env.
func resolveKubeRayVersion(basePath string) string {
	if v := kubeRayVersionFromImage(resolveKubeRayImage(basePath)); v != "" {
		return v
	}

	return kubeRayVersionFromImage(defaultKubeRayImage(basePath))
}

func defaultKubeRayImage(basePath string) string {
	if basePath == "" {
		return ""
	}

	params, err := parseParams(filepath.Join(basePath, constants.ManifestPath, constants.ManifestOverlay, "params.env"))
	if err != nil {
		return ""
	}

	return params[constants.KubeRayImageParam]
}

func desiredModuleReleases(kubeRayVersion string) []common.ComponentRelease {
	if kubeRayVersion == "" {
		return nil
	}

	return []common.ComponentRelease{{
		Name:    constants.KubeRayReleaseName,
		Version: kubeRayVersion,
		RepoURL: constants.KubeRayRepoURL,
	}}
}

// resolveKubeRayImage returns the KubeRay operand image that will be deployed:
// RELATED_IMAGE_* from the platform, otherwise the vendored params.env default.
func resolveKubeRayImage(basePath string) string {
	if envName := imageParamMap[constants.KubeRayImageParam]; envName != "" {
		if related := os.Getenv(envName); related != "" {
			return related
		}
	}

	if basePath == "" {
		return ""
	}

	params, err := parseParams(filepath.Join(basePath, constants.ManifestPath, constants.ManifestOverlay, "params.env"))
	if err != nil {
		return ""
	}

	return params[constants.KubeRayImageParam]
}

func kubeRayVersionFromImage(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}

	if digest := strings.LastIndex(image, "@"); digest >= 0 {
		image = image[:digest]
	}

	slash := strings.LastIndex(image, "/")
	colon := strings.LastIndex(image, ":")
	if colon > slash {
		return image[colon+1:]
	}

	return ""
}
