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
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

// imageParamMap maps params.env keys to the RELATED_IMAGE_* environment
// variables that Konflux / CSV injection sets at runtime.
var imageParamMap = map[string]string{
	"odh-kuberay-operator-controller-image": "RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE",
	"odh-kube-rbac-proxy-image":             "RELATED_IMAGE_ODH_KUBE_RBAC_PROXY_IMAGE",
}

// applyImageParamsAction rewrites params.env on disk before kustomize
// renders manifests, injecting the target namespace and any RELATED_IMAGE_*
// overrides. This matches the ODH operator's ApplyParams pattern.
func applyImageParamsAction(basePath string) actions.Fn {
	return func(_ context.Context, rr *types.ReconciliationRequest) error {
		removed, _ := rr.Extensions[constants.ExtKeyRemoved].(bool)
		if removed {
			return nil
		}

		ray := rr.Instance.(*componentsv1alpha1.Ray)
		componentPath := filepath.Join(basePath, constants.ManifestPath, constants.ManifestOverlay)

		return applyParams(componentPath, "params.env", imageParamMap,
			map[string]string{"namespace": ray.Spec.ApplicationsNamespace},
		)
	}
}

// applyParams reads a key=value params file, overrides values from
// RELATED_IMAGE_* environment variables and extra parameter maps, then
// writes the result back atomically via tmp+rename.
func applyParams(componentPath, file string, imageParamsMap map[string]string, extraParamsMaps ...map[string]string) error {
	paramsFile := filepath.Join(componentPath, file)

	paramsEnvMap, err := parseParams(paramsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	updated := 0

	for key := range paramsEnvMap {
		envVar, ok := imageParamsMap[key]
		if !ok {
			continue
		}
		relatedImageValue := os.Getenv(envVar)
		if relatedImageValue != "" {
			updated |= updateParam(&paramsEnvMap, key, relatedImageValue)
		}
	}

	for _, extraParamsMap := range extraParamsMaps {
		for key, value := range extraParamsMap {
			updated |= updateParam(&paramsEnvMap, key, value)
		}
	}

	if updated == 0 {
		return nil
	}

	tmp, err := os.CreateTemp(componentPath, "params.env-")
	if err != nil {
		return err
	}
	defer tmp.Close()

	writer := bufio.NewWriter(tmp)
	for key, value := range paramsEnvMap {
		if _, err := fmt.Fprintf(writer, "%s=%s\n", key, value); err != nil {
			_ = os.Remove(tmp.Name())
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("failed to write params: %w", err)
	}

	if err := os.Rename(tmp.Name(), paramsFile); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("failed rename %s to %s: %w", tmp.Name(), paramsFile, err)
	}

	return nil
}

func parseParams(fileName string) (map[string]string, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	params := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, found := strings.Cut(line, "=")
		if found {
			params[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return params, nil
}

func updateParam(m *map[string]string, key, val string) int {
	old := (*m)[key]
	if old == val {
		return 0
	}

	(*m)[key] = val
	return 1
}
