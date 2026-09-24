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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
	"github.com/4thel00z/serenedb-operator/internal/sqlexec"
	"github.com/4thel00z/serenedb-operator/internal/statements"
)

// DatabaseReconciler creates databases inside a SereneDB server.
type DatabaseReconciler struct {
	dependents
}

// +kubebuilder:rbac:groups=database.serenedb.com,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.serenedb.com,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.serenedb.com,resources=databases/finalizers,verbs=update

// Reconcile makes the database exist on the server and drops it on deletion when the policy says so.
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	db := &databasev1alpha1.Database{}
	if err := r.Get(ctx, req.NamespacedName, db); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	name := nameOrDefault(db.Spec.Name, db.Name)

	if !db.DeletionTimestamp.IsZero() {
		var drop func(context.Context, sqlexec.Session) error
		if db.Spec.ReclaimPolicy == databasev1alpha1.ReclaimDelete {
			drop = func(ctx context.Context, s sqlexec.Session) error { return dropDatabase(ctx, s, name) }
		}
		return r.finalize(ctx, db, db.Spec.Cluster.Name, drop)
	}
	if added, err := r.ensureFinalizer(ctx, db); added || err != nil {
		return ctrl.Result{}, err
	}

	original := db.DeepCopy()
	blockedOn, applyErr := r.apply(ctx, db, name)
	result := outcome(&db.Status.Conditions, db.Generation, blockedOn, applyErr)
	db.Status.ObservedGeneration = db.Generation
	if err := r.Status().Patch(ctx, db, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return result, nil
}

func (r *DatabaseReconciler) apply(ctx context.Context, db *databasev1alpha1.Database, name string) (*blocked, error) {
	session, blockedOn, err := r.openSession(ctx, db.Namespace, db.Spec.Cluster.Name)
	if blockedOn != nil || err != nil {
		return blockedOn, err
	}
	defer func() { _ = session.Close(ctx) }()

	query, err := statements.DatabaseExists(name)
	if err != nil {
		return &blocked{reasonInvalidSpec, err.Error()}, nil
	}
	present, err := exists(ctx, session, query)
	if err != nil {
		return nil, err
	}
	if present {
		return nil, nil
	}
	return nil, session.Exec(ctx, statements.CreateDatabase(name))
}

func dropDatabase(ctx context.Context, session sqlexec.Session, name string) error {
	query, err := statements.DatabaseExists(name)
	if err != nil {
		return err
	}
	present, err := exists(ctx, session, query)
	if err != nil || !present {
		return err
	}
	return session.Exec(ctx, statements.DropDatabase(name))
}

// SetupWithManager registers the controller, indexes the cluster reference and watches SereneDB.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &databasev1alpha1.Database{}, clusterIndex,
		func(obj client.Object) []string { return []string{obj.(*databasev1alpha1.Database).Spec.Cluster.Name} })
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.Database{}).
		Watches(&databasev1alpha1.SereneDB{}, requestsByIndex(mgr.GetClient(), &databasev1alpha1.DatabaseList{}, clusterIndex)).
		Named("database").
		Complete(r)
}
