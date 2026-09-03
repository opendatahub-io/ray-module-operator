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

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/opendatahub-io/ray-module-operator/internal/constants"
)

// notebookClusterRoleName is the stable ClusterRole Workbenches keys off to
// bind Ray permissions to Notebook Service Accounts. Do not rename.
// RHOAIENG-46748.
const notebookClusterRoleName = "ray"

func notebookClusterRoleAction() actions.Fn {
	return func(ctx context.Context, rr *types.ReconciliationRequest) error {
		removed, _ := rr.Extensions[constants.ExtKeyRemoved].(bool)
		if removed {
			return deleteNotebookClusterRole(ctx, rr)
		}

		return applyNotebookClusterRole(ctx, rr)
	}
}

func applyNotebookClusterRole(ctx context.Context, rr *types.ReconciliationRequest) error {
	desired := notebookClusterRole()
	existing := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: notebookClusterRoleName},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, rr.Client, existing, func() error {
		existing.Labels = desired.Labels
		existing.Rules = desired.Rules
		// Cluster-scoped RBAC must not take a controller ownerRef: that
		// sets blockOwnerDeletion and can pin default-ray in Terminating.
		existing.OwnerReferences = nil

		return nil
	})
	if err != nil {
		return fmt.Errorf("apply notebook ClusterRole %q: %w", notebookClusterRoleName, err)
	}

	return nil
}

func deleteNotebookClusterRole(ctx context.Context, rr *types.ReconciliationRequest) error {
	cr := &rbacv1.ClusterRole{}
	err := rr.Client.Get(ctx, k8stypes.NamespacedName{Name: notebookClusterRoleName}, cr)
	if k8serr.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get notebook ClusterRole %q: %w", notebookClusterRoleName, err)
	}

	// Drop leftover controller ownerRefs (from older deploys that went
	// through SSA) so default-ray is not pinned in Terminating.
	if len(cr.OwnerReferences) > 0 {
		cr.OwnerReferences = nil
		if err := rr.Client.Update(ctx, cr); err != nil && !k8serr.IsNotFound(err) {
			return fmt.Errorf("clear ownerRefs on notebook ClusterRole %q: %w", notebookClusterRoleName, err)
		}
	}

	if err := rr.Client.Delete(ctx, cr); err != nil && !k8serr.IsNotFound(err) {
		return fmt.Errorf("delete notebook ClusterRole %q: %w", notebookClusterRoleName, err)
	}

	return nil
}

func notebookClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: notebookClusterRoleName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ray-module-operator",
				gc.DefaultPartOfLabelKey:       constants.ComponentName,
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"ray.io"},
				Resources: []string{"rayjobs"},
				Verbs:     []string{"get", "list", "create", "patch", "delete"},
			},
			{APIGroups: []string{"ray.io"}, Resources: []string{"rayjobs/status"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{"ray.io"}, Resources: []string{"rayclusters"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{"ray.io"}, Resources: []string{"rayclusters/status"}, Verbs: []string{"get", "list"}},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"create"},
			},
		},
	}
}
