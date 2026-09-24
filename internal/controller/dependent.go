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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
	"github.com/4thel00z/serenedb-operator/internal/resources"
	"github.com/4thel00z/serenedb-operator/internal/sqlexec"
)

const (
	reasonClusterNotFound       = "ClusterNotFound"
	reasonClusterNotReady       = "ClusterNotReady"
	reasonConnectionFailed      = "ConnectionFailed"
	reasonSQLError              = "SQLError"
	reasonApplied               = "Applied"
	reasonPasswordSecretMissing = "PasswordSecretMissing"
	reasonValuesSecretMissing   = "ValuesSecretMissing"
	reasonInvalidSpec           = "InvalidSpec"

	clusterIndex = "spec.cluster.name"
	secretIndex  = "spec.secretName"

	resyncInterval = 10 * time.Minute
)

// dependents holds what the Database, DatabaseRole and ServerSecret reconcilers share: the API client
// and the way to open a SQL session on a SereneDB.
type dependents struct {
	client.Client
	Connector sqlexec.Connector
}

// openSession connects to the named SereneDB as the superuser. A blocked result means the caller
// should report the reason and retry later.
func (d *dependents) openSession(ctx context.Context, namespace, clusterName string) (sqlexec.Session, *blocked, error) {
	db := &databasev1alpha1.SereneDB{}
	err := d.Get(ctx, types.NamespacedName{Namespace: namespace, Name: clusterName}, db)
	if apierrors.IsNotFound(err) {
		return nil, &blocked{reasonClusterNotFound, fmt.Sprintf("serenedb %q not found", clusterName)}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get serenedb: %w", err)
	}
	if !meta.IsStatusConditionTrue(db.Status.Conditions, databasev1alpha1.ConditionReady) {
		return nil, &blocked{reasonClusterNotReady, fmt.Sprintf("serenedb %q is not ready", clusterName)}, nil
	}
	password, blockedOn, err := d.superuserPassword(ctx, db)
	if blockedOn != nil || err != nil {
		return nil, blockedOn, err
	}
	session, err := d.Connector(ctx, sqlexec.Target{
		Host:     resources.Host(db),
		Port:     resources.PostgresPort(db),
		User:     "postgres",
		Password: password,
		TLS:      db.Spec.TLS.Enabled,
	})
	if err != nil {
		return nil, &blocked{reasonConnectionFailed, err.Error()}, nil
	}
	return session, nil, nil
}

func (d *dependents) superuserPassword(ctx context.Context, db *databasev1alpha1.SereneDB) (string, *blocked, error) {
	secret := &corev1.Secret{}
	err := d.Get(ctx, types.NamespacedName{Namespace: db.Namespace, Name: resources.SecretName(db)}, secret)
	if apierrors.IsNotFound(err) {
		return "", &blocked{reasonSecretMissing, fmt.Sprintf("superuser secret %q not found", resources.SecretName(db))}, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("get superuser secret: %w", err)
	}
	password, ok := secret.Data[resources.PasswordKey(db)]
	if !ok {
		return "", &blocked{reasonSecretKeyMissing, fmt.Sprintf("superuser secret %q has no key %q", secret.Name, resources.PasswordKey(db))}, nil
	}
	return string(password), nil, nil
}

// clusterGone reports whether the SereneDB an object depends on is absent or being deleted, in which
// case server-side cleanup is skipped and the finalizer simply removed.
func (d *dependents) clusterGone(ctx context.Context, namespace, clusterName string) (bool, error) {
	db := &databasev1alpha1.SereneDB{}
	err := d.Get(ctx, types.NamespacedName{Namespace: namespace, Name: clusterName}, db)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("get serenedb: %w", err)
	}
	return !db.DeletionTimestamp.IsZero(), nil
}

// finalize runs the server-side drop when one is wanted and the SereneDB still exists, then removes
// the finalizer. It returns a requeue when the server cannot be reached yet.
func (d *dependents) finalize(ctx context.Context, obj client.Object, clusterName string, drop func(context.Context, sqlexec.Session) error) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(obj, databasev1alpha1.DependentFinalizer) {
		return ctrl.Result{}, nil
	}
	gone, err := d.clusterGone(ctx, obj.GetNamespace(), clusterName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if drop != nil && !gone {
		session, blockedOn, err := d.openSession(ctx, obj.GetNamespace(), clusterName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if blockedOn != nil {
			return ctrl.Result{RequeueAfter: missingDependencyRequeue}, nil
		}
		defer func() { _ = session.Close(ctx) }()
		if err := drop(ctx, session); err != nil {
			return ctrl.Result{}, fmt.Errorf("drop on server: %w", err)
		}
	}
	controllerutil.RemoveFinalizer(obj, databasev1alpha1.DependentFinalizer)
	if err := d.Update(ctx, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(fmt.Errorf("remove finalizer: %w", err))
	}
	return ctrl.Result{}, nil
}

// ensureFinalizer adds the finalizer and reports whether an update was issued.
func (d *dependents) ensureFinalizer(ctx context.Context, obj client.Object) (bool, error) {
	if controllerutil.ContainsFinalizer(obj, databasev1alpha1.DependentFinalizer) {
		return false, nil
	}
	controllerutil.AddFinalizer(obj, databasev1alpha1.DependentFinalizer)
	if err := d.Update(ctx, obj); err != nil {
		return false, fmt.Errorf("add finalizer: %w", err)
	}
	return true, nil
}

// exists runs a query and reports whether it returned any row.
func exists(ctx context.Context, session sqlexec.Session, query string) (bool, error) {
	rows, err := session.Query(ctx, query)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func setReady(conditions *[]metav1.Condition, generation int64, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               databasev1alpha1.ConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// outcome translates the result of applying an object into its Ready condition and requeue.
func outcome(conditions *[]metav1.Condition, generation int64, blockedOn *blocked, err error) ctrl.Result {
	if err != nil {
		setReady(conditions, generation, metav1.ConditionFalse, reasonSQLError, err.Error())
		return ctrl.Result{RequeueAfter: missingDependencyRequeue}
	}
	if blockedOn != nil {
		setReady(conditions, generation, metav1.ConditionFalse, blockedOn.reason, blockedOn.message)
		return ctrl.Result{RequeueAfter: missingDependencyRequeue}
	}
	setReady(conditions, generation, metav1.ConditionTrue, reasonApplied, "applied on the server")
	return ctrl.Result{RequeueAfter: resyncInterval}
}

func nameOrDefault(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

// requestsByIndex maps an event on an indexed object to reconcile requests for every object in the
// namespace whose index value equals the object's name.
func requestsByIndex(c client.Client, list client.ObjectList, index string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		fresh := list.DeepCopyObject().(client.ObjectList)
		err := c.List(ctx, fresh, client.InNamespace(obj.GetNamespace()), client.MatchingFields{index: obj.GetName()})
		if err != nil {
			return nil
		}
		items, err := meta.ExtractList(fresh)
		if err != nil {
			return nil
		}
		requests := make([]reconcile.Request, 0, len(items))
		for _, item := range items {
			o, ok := item.(client.Object)
			if !ok {
				continue
			}
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(o)})
		}
		return requests
	})
}

// WithClient sets the API client and SQL connector.
func (r *DatabaseReconciler) WithClient(c client.Client, connector sqlexec.Connector) *DatabaseReconciler {
	r.dependents = dependents{Client: c, Connector: connector}
	return r
}

// WithClient sets the API client and SQL connector.
func (r *DatabaseRoleReconciler) WithClient(c client.Client, connector sqlexec.Connector) *DatabaseRoleReconciler {
	r.dependents = dependents{Client: c, Connector: connector}
	return r
}

// WithClient sets the API client and SQL connector.
func (r *ServerSecretReconciler) WithClient(c client.Client, connector sqlexec.Connector) *ServerSecretReconciler {
	r.dependents = dependents{Client: c, Connector: connector}
	return r
}

type runtimeScheme = runtime.Scheme

func isNoKindMatch(err error) bool {
	return meta.IsNoMatchError(err)
}
