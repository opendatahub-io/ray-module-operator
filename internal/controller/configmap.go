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
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	componentsv1alpha1 "github.com/opendatahub-io/ray-module-operator/api/v1alpha1"
	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

type distributionReadStatus int

const (
	distributionAbsent distributionReadStatus = iota
	distributionOK
	distributionInvalid
)

func platformConfigMapper(cli client.Client) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		if obj.GetName() != constants.PlatformConfigMapName {
			return nil
		}

		ray := &componentsv1alpha1.Ray{}
		if err := cli.Get(ctx, types.NamespacedName{Name: constants.InstanceName}, ray); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}

			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{Name: constants.InstanceName},
			}}
		}

		appsNamespace := ray.Spec.ApplicationsNamespace
		if appsNamespace == "" || obj.GetNamespace() != appsNamespace {
			return nil
		}

		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Name: constants.InstanceName},
		}}
	}
}

func platformConfigPredicate(obj client.Object) bool {
	return obj.GetName() == constants.PlatformConfigMapName
}

func readPlatformDistribution(ctx context.Context, cli client.Client, appsNamespace string) (componentsv1alpha1.Distribution, distributionReadStatus, error) {
	if appsNamespace == "" {
		return componentsv1alpha1.Distribution{}, distributionAbsent, nil
	}

	cm := &corev1.ConfigMap{}
	if err := cli.Get(ctx, types.NamespacedName{
		Name:      constants.PlatformConfigMapName,
		Namespace: appsNamespace,
	}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return componentsv1alpha1.Distribution{}, distributionAbsent, nil
		}

		return componentsv1alpha1.Distribution{}, distributionAbsent, fmt.Errorf(
			"get platform config ConfigMap %s/%s: %w",
			appsNamespace, constants.PlatformConfigMapName, err)
	}

	name := cm.Data[constants.PlatformNameKey]
	version := cm.Data[constants.PlatformVersionKey]
	if name == "" && version == "" {
		return componentsv1alpha1.Distribution{}, distributionAbsent, nil
	}

	if version != "" {
		if err := validateReleaseVersion(version); err != nil {
			return componentsv1alpha1.Distribution{}, distributionInvalid, nil
		}
	}

	if name == "" {
		name = constants.StandaloneDistributionName
	}

	return componentsv1alpha1.Distribution{Name: name, Version: version}, distributionOK, nil
}

func standaloneDistribution(moduleVersion string) componentsv1alpha1.Distribution {
	if moduleVersion == "" {
		moduleVersion = constants.StandaloneDistributionVersion
	}

	return componentsv1alpha1.Distribution{
		Name:    constants.StandaloneDistributionName,
		Version: moduleVersion,
	}
}

func validateReleaseVersion(version string) error {
	if len(version) > constants.MaxReleaseVersionLength {
		return fmt.Errorf("release version length %d exceeds maximum %d",
			len(version), constants.MaxReleaseVersionLength)
	}

	if _, err := semver.NewVersion(strings.TrimPrefix(version, "v")); err != nil {
		return fmt.Errorf("parse semantic version %q: %w", version, err)
	}

	return nil
}
