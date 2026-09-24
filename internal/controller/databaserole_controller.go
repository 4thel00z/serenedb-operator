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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
	"github.com/4thel00z/serenedb-operator/internal/sqlexec"
	"github.com/4thel00z/serenedb-operator/internal/statements"
)

// DatabaseRoleReconciler manages roles inside a SereneDB server.
type DatabaseRoleReconciler struct {
	dependents
}

// +kubebuilder:rbac:groups=database.serenedb.com,resources=databaseroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.serenedb.com,resources=databaseroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.serenedb.com,resources=databaseroles/finalizers,verbs=update

// Reconcile converges the role's attributes, password and memberships, and drops it on deletion when the policy says so.
func (r *DatabaseRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	role := &databasev1alpha1.DatabaseRole{}
	if err := r.Get(ctx, req.NamespacedName, role); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	name := nameOrDefault(role.Spec.Name, role.Name)

	if !role.DeletionTimestamp.IsZero() {
		var drop func(context.Context, sqlexec.Session) error
		if role.Spec.ReclaimPolicy == databasev1alpha1.ReclaimDelete {
			drop = func(ctx context.Context, s sqlexec.Session) error { return dropRole(ctx, s, name) }
		}
		return r.finalize(ctx, role, role.Spec.Cluster.Name, drop)
	}
	if added, err := r.ensureFinalizer(ctx, role); added || err != nil {
		return ctrl.Result{}, err
	}

	original := role.DeepCopy()
	blockedOn, applyErr := r.apply(ctx, role, name)
	result := outcome(&role.Status.Conditions, role.Generation, blockedOn, applyErr)
	role.Status.ObservedGeneration = role.Generation
	if err := r.Status().Patch(ctx, role, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return result, nil
}

func (r *DatabaseRoleReconciler) apply(ctx context.Context, role *databasev1alpha1.DatabaseRole, name string) (*blocked, error) {
	password, version, blockedOn, err := r.password(ctx, role)
	if blockedOn != nil || err != nil {
		return blockedOn, err
	}
	session, blockedOn, err := r.openSession(ctx, role.Namespace, role.Spec.Cluster.Name)
	if blockedOn != nil || err != nil {
		return blockedOn, err
	}
	defer func() { _ = session.Close(ctx) }()

	query, err := statements.RoleExists(name)
	if err != nil {
		return &blocked{reasonInvalidSpec, err.Error()}, nil
	}
	present, err := exists(ctx, session, query)
	if err != nil {
		return nil, err
	}

	attrs := roleAttributes(role.Spec)
	if password != nil && (!present || version != role.Status.PasswordSecretVersion) {
		attrs.Password = password
	}
	statement, err := roleStatement(name, attrs, present)
	if err != nil {
		return &blocked{reasonInvalidSpec, err.Error()}, nil
	}
	if err := session.Exec(ctx, statement); err != nil {
		return nil, err
	}
	if err := r.converge(ctx, session, name, role.Spec.InRoles); err != nil {
		return nil, err
	}
	role.Status.PasswordSecretVersion = version
	return nil, nil
}

// password reads the role password from its Secret, returning the value and the Secret's resourceVersion.
func (r *DatabaseRoleReconciler) password(ctx context.Context, role *databasev1alpha1.DatabaseRole) (*string, string, *blocked, error) {
	ref := role.Spec.PasswordSecret
	if ref == nil {
		return nil, "", nil, nil
	}
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: role.Namespace, Name: ref.Name}, secret)
	if apierrors.IsNotFound(err) {
		return nil, "", &blocked{reasonPasswordSecretMissing, fmt.Sprintf("secret %q not found", ref.Name)}, nil
	}
	if err != nil {
		return nil, "", nil, fmt.Errorf("get password secret: %w", err)
	}
	key := nameOrDefault(ref.Key, "password")
	value, ok := secret.Data[key]
	if !ok {
		return nil, "", &blocked{reasonPasswordSecretMissing, fmt.Sprintf("secret %q has no key %q", ref.Name, key)}, nil
	}
	password := string(value)
	return &password, secret.ResourceVersion, nil, nil
}

// converge grants the memberships that are missing and revokes the ones no longer listed.
func (r *DatabaseRoleReconciler) converge(ctx context.Context, session sqlexec.Session, name string, wanted []string) error {
	query, err := statements.RoleMemberships(name)
	if err != nil {
		return err
	}
	rows, err := session.Query(ctx, query)
	if err != nil {
		return err
	}
	current := make(map[string]bool, len(rows))
	for _, row := range rows {
		current[row[0]] = true
	}
	desired := make(map[string]bool, len(wanted))
	for _, group := range wanted {
		desired[group] = true
		if current[group] {
			continue
		}
		if err := session.Exec(ctx, statements.GrantMembership(group, name)); err != nil {
			return err
		}
	}
	for group := range current {
		if desired[group] {
			continue
		}
		if err := session.Exec(ctx, statements.RevokeMembership(group, name)); err != nil {
			return err
		}
	}
	return nil
}

func roleAttributes(spec databasev1alpha1.DatabaseRoleSpec) statements.RoleAttributes {
	inherit := true
	if spec.Inherit != nil {
		inherit = *spec.Inherit
	}
	return statements.RoleAttributes{
		Login:           spec.Login,
		Superuser:       spec.Superuser,
		CreateDB:        spec.CreateDB,
		CreateRole:      spec.CreateRole,
		Inherit:         inherit,
		ConnectionLimit: spec.ConnectionLimit,
		ValidUntil:      spec.ValidUntil,
	}
}

func roleStatement(name string, attrs statements.RoleAttributes, present bool) (string, error) {
	if present {
		return statements.AlterRole(name, attrs)
	}
	return statements.CreateRole(name, attrs)
}

func dropRole(ctx context.Context, session sqlexec.Session, name string) error {
	query, err := statements.RoleExists(name)
	if err != nil {
		return err
	}
	present, err := exists(ctx, session, query)
	if err != nil || !present {
		return err
	}
	return session.Exec(ctx, statements.DropRole(name))
}

// SetupWithManager registers the controller with indexes on the cluster reference and the password Secret.
func (r *DatabaseRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	indexer := mgr.GetFieldIndexer()
	err := indexer.IndexField(context.Background(), &databasev1alpha1.DatabaseRole{}, clusterIndex,
		func(obj client.Object) []string {
			return []string{obj.(*databasev1alpha1.DatabaseRole).Spec.Cluster.Name}
		})
	if err != nil {
		return err
	}
	err = indexer.IndexField(context.Background(), &databasev1alpha1.DatabaseRole{}, secretIndex,
		func(obj client.Object) []string {
			ref := obj.(*databasev1alpha1.DatabaseRole).Spec.PasswordSecret
			if ref == nil {
				return nil
			}
			return []string{ref.Name}
		})
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.DatabaseRole{}).
		Watches(&databasev1alpha1.SereneDB{}, requestsByIndex(mgr.GetClient(), &databasev1alpha1.DatabaseRoleList{}, clusterIndex)).
		Watches(&corev1.Secret{}, requestsByIndex(mgr.GetClient(), &databasev1alpha1.DatabaseRoleList{}, secretIndex)).
		Named("databaserole").
		Complete(r)
}
