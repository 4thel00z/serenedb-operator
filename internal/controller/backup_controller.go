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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
	"github.com/4thel00z/serenedb-operator/internal/resources"
	"github.com/4thel00z/serenedb-operator/internal/sqlexec"
)

const snapshotPollInterval = 15 * time.Second

// BackupReconciler takes volume snapshots of a SereneDB data volume after a CHECKPOINT.
type BackupReconciler struct {
	dependents
	Scheme *runtimeScheme
}

// +kubebuilder:rbac:groups=database.serenedb.com,resources=backups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.serenedb.com,resources=backups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.serenedb.com,resources=backups/finalizers,verbs=update
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives a Backup from Pending through Running to Completed or Failed.
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	backup := &databasev1alpha1.Backup{}
	if err := r.Get(ctx, req.NamespacedName, backup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !backup.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	original := backup.DeepCopy()
	var result ctrl.Result
	var err error
	switch backup.Status.Phase {
	case databasev1alpha1.BackupPhaseCompleted, databasev1alpha1.BackupPhaseFailed:
		return ctrl.Result{}, nil
	case databasev1alpha1.BackupPhaseRunning:
		result, err = r.observe(ctx, backup)
	default:
		result, err = r.start(ctx, backup)
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if statusErr := r.Status().Patch(ctx, backup, client.MergeFrom(original)); statusErr != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", statusErr)
	}
	return result, nil
}

// start checkpoints the server and requests the snapshot.
func (r *BackupReconciler) start(ctx context.Context, backup *databasev1alpha1.Backup) (ctrl.Result, error) {
	if backup.Spec.Method != "" && backup.Spec.Method != databasev1alpha1.BackupMethodVolumeSnapshot {
		fail(backup, fmt.Sprintf("unsupported method %q", backup.Spec.Method))
		return ctrl.Result{}, nil
	}
	db := &databasev1alpha1.SereneDB{}
	err := r.Get(ctx, types.NamespacedName{Namespace: backup.Namespace, Name: backup.Spec.Cluster.Name}, db)
	if apierrors.IsNotFound(err) {
		pending(backup, fmt.Sprintf("serenedb %q not found", backup.Spec.Cluster.Name))
		return ctrl.Result{RequeueAfter: missingDependencyRequeue}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get serenedb: %w", err)
	}
	session, blockedOn, err := r.openSession(ctx, backup.Namespace, backup.Spec.Cluster.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	if blockedOn != nil {
		pending(backup, blockedOn.message)
		return ctrl.Result{RequeueAfter: missingDependencyRequeue}, nil
	}
	defer func() { _ = session.Close(ctx) }()
	if err := session.Exec(ctx, "CHECKPOINT"); err != nil {
		pending(backup, "checkpoint: "+err.Error())
		return ctrl.Result{RequeueAfter: missingDependencyRequeue}, nil
	}
	started := metav1.Now()

	snapshot := resources.VolumeSnapshot(backup, resources.DataClaimName(db))
	if err := controllerutil.SetControllerReference(backup, snapshot, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("set snapshot owner: %w", err)
	}
	if err := r.Create(ctx, snapshot); err != nil && !apierrors.IsAlreadyExists(err) {
		if apierrors.IsNotFound(err) || isNoKindMatch(err) {
			fail(backup, "the cluster has no VolumeSnapshot API; install a CSI snapshot controller")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("create volumesnapshot: %w", err)
	}
	backup.Status.Phase = databasev1alpha1.BackupPhaseRunning
	backup.Status.StartedAt = &started
	backup.Status.SnapshotName = snapshot.GetName()
	backup.Status.Error = ""
	return ctrl.Result{RequeueAfter: snapshotPollInterval}, nil
}

// observe waits for the snapshot to become ready to use.
func (r *BackupReconciler) observe(ctx context.Context, backup *databasev1alpha1.Backup) (ctrl.Result, error) {
	snapshot := &unstructured.Unstructured{}
	snapshot.SetGroupVersionKind(resources.VolumeSnapshotGVK)
	err := r.Get(ctx, types.NamespacedName{Namespace: backup.Namespace, Name: backup.Status.SnapshotName}, snapshot)
	if apierrors.IsNotFound(err) {
		fail(backup, fmt.Sprintf("volumesnapshot %q disappeared", backup.Status.SnapshotName))
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get volumesnapshot: %w", err)
	}
	ready, failure := resources.SnapshotOutcome(snapshot)
	if failure != "" {
		fail(backup, failure)
		return ctrl.Result{}, nil
	}
	if !ready {
		return ctrl.Result{RequeueAfter: snapshotPollInterval}, nil
	}
	now := metav1.Now()
	backup.Status.Phase = databasev1alpha1.BackupPhaseCompleted
	backup.Status.CompletedAt = &now
	return ctrl.Result{}, nil
}

func pending(backup *databasev1alpha1.Backup, message string) {
	backup.Status.Phase = databasev1alpha1.BackupPhasePending
	backup.Status.Error = message
}

func fail(backup *databasev1alpha1.Backup, message string) {
	backup.Status.Phase = databasev1alpha1.BackupPhaseFailed
	backup.Status.Error = message
}

// WithClient sets the API client, scheme and SQL connector.
func (r *BackupReconciler) WithClient(c client.Client, scheme *runtimeScheme, connector sqlexec.Connector) *BackupReconciler {
	r.dependents = dependents{Client: c, Connector: connector}
	r.Scheme = scheme
	return r
}

// SetupWithManager registers the controller. The VolumeSnapshot watch is added only when the cluster serves that API;
// otherwise the Running phase is polled.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &databasev1alpha1.Backup{}, clusterIndex,
		func(obj client.Object) []string { return []string{obj.(*databasev1alpha1.Backup).Spec.Cluster.Name} })
	if err != nil {
		return err
	}
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.Backup{}).
		Watches(&databasev1alpha1.SereneDB{}, requestsByIndex(mgr.GetClient(), &databasev1alpha1.BackupList{}, clusterIndex)).
		Named("backup")
	snapshot := &unstructured.Unstructured{}
	snapshot.SetGroupVersionKind(resources.VolumeSnapshotGVK)
	if _, err := mgr.GetRESTMapper().RESTMapping(resources.VolumeSnapshotGVK.GroupKind(), resources.VolumeSnapshotGVK.Version); err == nil {
		builder = builder.Owns(snapshot)
	} else {
		logf.Log.Info("VolumeSnapshot API not served; backups will poll snapshot status", "reason", err.Error())
	}
	return builder.Complete(r)
}
