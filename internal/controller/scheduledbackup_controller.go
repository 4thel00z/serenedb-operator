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
	"sort"
	"time"

	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
)

const (
	reasonScheduled       = "Scheduled"
	reasonSuspended       = "Suspended"
	reasonInvalidSchedule = "InvalidSchedule"
)

// ScheduledBackupReconciler creates Backups on a cron schedule and prunes old ones.
type ScheduledBackupReconciler struct {
	client.Client
	Now func() time.Time
}

// +kubebuilder:rbac:groups=database.serenedb.com,resources=scheduledbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.serenedb.com,resources=scheduledbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.serenedb.com,resources=scheduledbackups/finalizers,verbs=update

// Reconcile creates a Backup when one is due and requeues for the next run.
func (r *ScheduledBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	schedule := &databasev1alpha1.ScheduledBackup{}
	if err := r.Get(ctx, req.NamespacedName, schedule); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !schedule.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	original := schedule.DeepCopy()
	result, err := r.tick(ctx, schedule)
	if err != nil {
		return ctrl.Result{}, err
	}
	if statusErr := r.Status().Patch(ctx, schedule, client.MergeFrom(original)); statusErr != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", statusErr)
	}
	return result, nil
}

func (r *ScheduledBackupReconciler) tick(ctx context.Context, schedule *databasev1alpha1.ScheduledBackup) (ctrl.Result, error) {
	parsed, err := cron.ParseStandard(schedule.Spec.Schedule)
	if err != nil {
		r.setReady(schedule, metav1.ConditionFalse, reasonInvalidSchedule, err.Error())
		schedule.Status.NextScheduleTime = nil
		return ctrl.Result{}, nil
	}
	if schedule.Spec.Suspend {
		r.setReady(schedule, metav1.ConditionTrue, reasonSuspended, "no new backups are created while suspended")
		schedule.Status.NextScheduleTime = nil
		return ctrl.Result{}, nil
	}
	now := r.now()
	if r.due(schedule, parsed, now) {
		if err := r.createBackup(ctx, schedule, now); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.prune(ctx, schedule); err != nil {
			return ctrl.Result{}, err
		}
	}
	next := parsed.Next(now)
	schedule.Status.NextScheduleTime = &metav1.Time{Time: next}
	r.setReady(schedule, metav1.ConditionTrue, reasonScheduled, fmt.Sprintf("next backup at %s", next.UTC().Format(time.RFC3339)))
	return ctrl.Result{RequeueAfter: next.Sub(now) + time.Second}, nil
}

func (r *ScheduledBackupReconciler) due(schedule *databasev1alpha1.ScheduledBackup, parsed cron.Schedule, now time.Time) bool {
	last := schedule.Status.LastScheduleTime
	if last == nil {
		if schedule.Spec.Immediate {
			return true
		}
		return !now.Before(parsed.Next(schedule.CreationTimestamp.Time))
	}
	return !now.Before(parsed.Next(last.Time))
}

func (r *ScheduledBackupReconciler) createBackup(ctx context.Context, schedule *databasev1alpha1.ScheduledBackup, now time.Time) error {
	backup := &databasev1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", schedule.Name, now.UTC().Format("20060102-150405")),
			Namespace: schedule.Namespace,
			Labels:    map[string]string{databasev1alpha1.ScheduledBackupLabel: schedule.Name},
		},
		Spec: databasev1alpha1.BackupSpec{
			Cluster:                 schedule.Spec.Cluster,
			Method:                  schedule.Spec.Method,
			VolumeSnapshotClassName: schedule.Spec.VolumeSnapshotClassName,
		},
	}
	if err := r.Create(ctx, backup); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create backup: %w", err)
	}
	schedule.Status.LastScheduleTime = &metav1.Time{Time: now}
	schedule.Status.LastBackup = backup.Name
	return nil
}

// prune deletes the oldest completed Backups of this schedule beyond spec.keep.
func (r *ScheduledBackupReconciler) prune(ctx context.Context, schedule *databasev1alpha1.ScheduledBackup) error {
	if schedule.Spec.Keep == nil {
		return nil
	}
	list := &databasev1alpha1.BackupList{}
	err := r.List(ctx, list, client.InNamespace(schedule.Namespace), client.MatchingLabels{databasev1alpha1.ScheduledBackupLabel: schedule.Name})
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}
	completed := make([]databasev1alpha1.Backup, 0, len(list.Items))
	for _, b := range list.Items {
		if b.Status.Phase == databasev1alpha1.BackupPhaseCompleted {
			completed = append(completed, b)
		}
	}
	sort.Slice(completed, func(i, j int) bool {
		if !completed[i].CreationTimestamp.Equal(&completed[j].CreationTimestamp) {
			return completed[i].CreationTimestamp.After(completed[j].CreationTimestamp.Time)
		}
		return completed[i].Name > completed[j].Name
	})
	for i := int(*schedule.Spec.Keep); i < len(completed); i++ {
		if err := r.Delete(ctx, &completed[i]); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("delete backup %s: %w", completed[i].Name, err)
		}
	}
	return nil
}

func (r *ScheduledBackupReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *ScheduledBackupReconciler) setReady(schedule *databasev1alpha1.ScheduledBackup, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&schedule.Status.Conditions, metav1.Condition{
		Type:               databasev1alpha1.ConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: schedule.Generation,
	})
}

// SetupWithManager registers the controller.
func (r *ScheduledBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.ScheduledBackup{}).
		Named("scheduledbackup").
		Complete(r)
}
