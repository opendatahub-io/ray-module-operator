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
	"path/filepath"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/kustomize"
)

// RenderKustomize returns an action that renders Kustomize manifests from
// the filesystem using the root kustomize engine. This sidesteps the
// framework's Engine interface mismatch (P5) by using the root engine
// directly — it defaults to filesys.MakeFsOnDisk(), which reads the
// vendored manifests at basePath without any embed.FS adapter.
func RenderKustomize(basePath string, namespaceFn actions.Getter[string]) actions.Fn {
	return func(ctx context.Context, rr *types.ReconciliationRequest) error {
		if len(rr.Manifests) == 0 {
			return nil
		}

		ns, err := namespaceFn(ctx, rr)
		if err != nil {
			return err
		}

		engine := kustomize.NewEngine()

		for _, m := range rr.Manifests {
			path := m.Path
			if m.ContextDir != "" {
				path = filepath.Join(path, m.ContextDir)
			}
			if m.SourcePath != "" {
				path = filepath.Join(path, m.SourcePath)
			}
			path = filepath.Join(basePath, path)

			resources, err := engine.Render(path, kustomize.WithNamespace(ns))
			if err != nil {
				return fmt.Errorf("render manifests from %s: %w", path, err)
			}

			rr.Resources = append(rr.Resources, resources...)
		}

		rr.Generated = true

		return nil
	}
}
