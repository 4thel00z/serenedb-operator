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
	"maps"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
	"github.com/4thel00z/serenedb-operator/internal/sqlexec"
	"github.com/4thel00z/serenedb-operator/internal/statements"
)

// ServerSecretReconciler manages persistent secrets in a SereneDB server's secrets manager.
type ServerSecretReconciler struct {
	dependents
}

// +kubebuilder:rbac:groups=database.serenedb.com,resources=serversecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.serenedb.com,resources=serversecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.serenedb.com,resources=serversecrets/finalizers,verbs=update

// Reconcile creates or replaces the server secret and always drops it on deletion.
func (r *ServerSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	secret := &databasev1alpha1.ServerSecret{}
	if err := r.Get(ctx, req.NamespacedName, secret); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	name := nameOrDefault(secret.Spec.Name, secret.Name)

	if !secret.DeletionTimestamp.IsZero() {
		drop := func(ctx context.Context, s sqlexec.Session) error { return dropServerSecret(ctx, s, name) }
		return r.finalize(ctx, secret, secret.Spec.Cluster.Name, drop)
	}
	if added, err := r.ensureFinalizer(ctx, secret); added || err != nil {
		return ctrl.Result{}, err
	}

	original := secret.DeepCopy()
	blockedOn, applyErr := r.apply(ctx, secret, name)
	result := outcome(&secret.Status.Conditions, secret.Generation, blockedOn, applyErr)
	secret.Status.ObservedGeneration = secret.Generation
	if err := r.Status().Patch(ctx, secret, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return result, nil
}

func (r *ServerSecretReconciler) apply(ctx context.Context, secret *databasev1alpha1.ServerSecret, name string) (*blocked, error) {
	options, version, blockedOn, err := r.options(ctx, secret)
	if blockedOn != nil || err != nil {
		return blockedOn, err
	}
	statement, err := statements.CreateSecret(name, secret.Spec.Type, secret.Spec.Scope, options)
	if err != nil {
		return &blocked{reasonInvalidSpec, err.Error()}, nil
	}
	session, blockedOn, err := r.openSession(ctx, secret.Namespace, secret.Spec.Cluster.Name)
	if blockedOn != nil || err != nil {
		return blockedOn, err
	}
	defer func() { _ = session.Close(ctx) }()
	if err := session.Exec(ctx, statement); err != nil {
		return nil, err
	}
	secret.Status.ValuesSecretVersion = version
	return nil, nil
}

// options merges the inline options with every key of the referenced Kubernetes Secret, the Secret winning.
func (r *ServerSecretReconciler) options(ctx context.Context, secret *databasev1alpha1.ServerSecret) (map[string]string, string, *blocked, error) {
	options := make(map[string]string, len(secret.Spec.Options))
	maps.Copy(options, secret.Spec.Options)
	if secret.Spec.ValuesFrom == nil {
		return options, "", nil, nil
	}
	values := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: secret.Spec.ValuesFrom.Name}, values)
	if apierrors.IsNotFound(err) {
		return nil, "", &blocked{reasonValuesSecretMissing, fmt.Sprintf("secret %q not found", secret.Spec.ValuesFrom.Name)}, nil
	}
	if err != nil {
		return nil, "", nil, fmt.Errorf("get values secret: %w", err)
	}
	for k, v := range values.Data {
		options[k] = string(v)
	}
	return options, values.ResourceVersion, nil, nil
}

func dropServerSecret(ctx context.Context, session sqlexec.Session, name string) error {
	query, err := statements.SecretExists(name)
	if err != nil {
		return err
	}
	present, err := exists(ctx, session, query)
	if err != nil || !present {
		return err
	}
	return session.Exec(ctx, statements.DropSecret(name))
}

// SetupWithManager registers the controller with indexes on the cluster reference and the values Secret.
func (r *ServerSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	indexer := mgr.GetFieldIndexer()
	err := indexer.IndexField(context.Background(), &databasev1alpha1.ServerSecret{}, clusterIndex,
		func(obj client.Object) []string {
			return []string{obj.(*databasev1alpha1.ServerSecret).Spec.Cluster.Name}
		})
	if err != nil {
		return err
	}
	err = indexer.IndexField(context.Background(), &databasev1alpha1.ServerSecret{}, secretIndex,
		func(obj client.Object) []string {
			ref := obj.(*databasev1alpha1.ServerSecret).Spec.ValuesFrom
			if ref == nil {
				return nil
			}
			return []string{ref.Name}
		})
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.ServerSecret{}).
		Watches(&databasev1alpha1.SereneDB{}, requestsByIndex(mgr.GetClient(), &databasev1alpha1.ServerSecretList{}, clusterIndex)).
		Watches(&corev1.Secret{}, requestsByIndex(mgr.GetClient(), &databasev1alpha1.ServerSecretList{}, secretIndex)).
		Named("serversecret").
		Complete(r)
}
