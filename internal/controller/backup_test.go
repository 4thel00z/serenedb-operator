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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
	"github.com/4thel00z/serenedb-operator/internal/resources"
)

func backupPhase(backup *databasev1alpha1.Backup) func() string {
	return func() string {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), backup); err != nil {
			return ""
		}
		return backup.Status.Phase
	}
}

func getSnapshot(namespace, name string) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(resources.VolumeSnapshotGVK)
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, u)
	return u, err
}

func setSnapshotStatus(namespace, name string, status map[string]any) {
	Eventually(func() error {
		u, err := getSnapshot(namespace, name)
		if err != nil {
			return err
		}
		u.Object["status"] = status
		return k8sClient.Status().Update(ctx, u)
	}, eventuallyTimeout, pollInterval).Should(Succeed())
}

var _ = Describe("Backup controller", func() {
	It("checkpoints, requests a snapshot of the data claim, and completes when it is ready", func() {
		ns := newNamespace()
		backup := &databasev1alpha1.Backup{
			ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: ns},
			Spec:       databasev1alpha1.BackupSpec{Cluster: databasev1alpha1.ClusterRef{Name: "srv"}, VolumeSnapshotClassName: ptr.To("csi-fast")},
		}
		Expect(k8sClient.Create(ctx, backup)).To(Succeed())
		Eventually(backupPhase(backup), eventuallyTimeout, pollInterval).Should(Equal(databasev1alpha1.BackupPhasePending))
		Expect(backup.Status.Error).To(ContainSubstring("not found"))

		readyCluster(ns)
		Eventually(backupPhase(backup), eventuallyTimeout, pollInterval).Should(Equal(databasev1alpha1.BackupPhaseRunning))
		Expect(fakeSQL.Executed("CHECKPOINT")).To(BeTrue())
		Expect(backup.Status.SnapshotName).To(Equal("nightly"))
		Expect(backup.Status.StartedAt).NotTo(BeNil())

		snapshot, err := getSnapshot(ns, "nightly")
		Expect(err).NotTo(HaveOccurred())
		source, _, _ := unstructured.NestedString(snapshot.Object, "spec", "source", "persistentVolumeClaimName")
		Expect(source).To(Equal("data-srv-0"))
		class, _, _ := unstructured.NestedString(snapshot.Object, "spec", "volumeSnapshotClassName")
		Expect(class).To(Equal("csi-fast"))
		Expect(metav1.IsControlledBy(snapshot, backup)).To(BeTrue())

		setSnapshotStatus(ns, "nightly", map[string]any{"readyToUse": true})
		Eventually(backupPhase(backup), eventuallyTimeout, pollInterval).Should(Equal(databasev1alpha1.BackupPhaseCompleted))
		Expect(backup.Status.CompletedAt).NotTo(BeNil())
	})

	It("fails when the snapshot reports an error", func() {
		ns := newNamespace()
		readyCluster(ns)
		backup := &databasev1alpha1.Backup{
			ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: ns},
			Spec:       databasev1alpha1.BackupSpec{Cluster: databasev1alpha1.ClusterRef{Name: "srv"}},
		}
		Expect(k8sClient.Create(ctx, backup)).To(Succeed())
		Eventually(backupPhase(backup), eventuallyTimeout, pollInterval).Should(Equal(databasev1alpha1.BackupPhaseRunning))
		setSnapshotStatus(ns, "broken", map[string]any{"readyToUse": false, "error": map[string]any{"message": "no space"}})
		Eventually(backupPhase(backup), eventuallyTimeout, pollInterval).Should(Equal(databasev1alpha1.BackupPhaseFailed))
		Expect(backup.Status.Error).To(Equal("no space"))
	})
})

var _ = Describe("ScheduledBackup controller", func() {
	It("creates an immediate backup, labels it, and prunes beyond keep", func() {
		ns := newNamespace()
		schedule := &databasev1alpha1.ScheduledBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "hourly", Namespace: ns},
			Spec: databasev1alpha1.ScheduledBackupSpec{
				Cluster:   databasev1alpha1.ClusterRef{Name: "srv"},
				Schedule:  "0 * * * *",
				Immediate: true,
				Keep:      ptr.To[int32](1),
			},
		}
		Expect(k8sClient.Create(ctx, schedule)).To(Succeed())
		Eventually(func() string {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(schedule), schedule)).To(Succeed())
			return schedule.Status.LastBackup
		}, eventuallyTimeout, pollInterval).ShouldNot(BeEmpty())
		Expect(schedule.Status.NextScheduleTime).NotTo(BeNil())

		first := &databasev1alpha1.Backup{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: schedule.Status.LastBackup}, first)).To(Succeed())
		Expect(first.Labels[databasev1alpha1.ScheduledBackupLabel]).To(Equal("hourly"))
		Expect(first.Spec.Cluster.Name).To(Equal("srv"))

		By("pruning: two completed backups with keep 1 leaves the newest")
		markBackupCompleted(first)
		second := &databasev1alpha1.Backup{
			ObjectMeta: metav1.ObjectMeta{Name: "hourly-manual", Namespace: ns, Labels: map[string]string{databasev1alpha1.ScheduledBackupLabel: "hourly"}},
			Spec:       databasev1alpha1.BackupSpec{Cluster: databasev1alpha1.ClusterRef{Name: "srv"}},
		}
		Expect(k8sClient.Create(ctx, second)).To(Succeed())
		markBackupCompleted(second)

		poke := 0
		Eventually(func() int {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(schedule), schedule)).To(Succeed())
			schedule.Status.LastScheduleTime = &metav1.Time{Time: metav1.Now().Add(-2 * time.Hour)}
			Expect(k8sClient.Status().Update(ctx, schedule)).To(Succeed())
			poke++
			updateScheduleSpec(schedule, func(s *databasev1alpha1.ScheduledBackup) {
				s.Spec.VolumeSnapshotClassName = ptr.To(fmt.Sprintf("poke-%d", poke))
			})
			return completedBackups(ns, "hourly")
		}, eventuallyTimeout, time.Second).Should(Equal(1))
	})

	It("reports an invalid schedule and honors suspend", func() {
		ns := newNamespace()
		schedule := &databasev1alpha1.ScheduledBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: ns},
			Spec:       databasev1alpha1.ScheduledBackupSpec{Cluster: databasev1alpha1.ClusterRef{Name: "srv"}, Schedule: "not a cron"},
		}
		Expect(k8sClient.Create(ctx, schedule)).To(Succeed())
		reason := readyReason(schedule, func() []metav1.Condition { return schedule.Status.Conditions })
		Eventually(reason, eventuallyTimeout, pollInterval).Should(Equal(reasonInvalidSchedule))

		updateScheduleSpec(schedule, func(s *databasev1alpha1.ScheduledBackup) { s.Spec.Schedule = "0 2 * * *"; s.Spec.Suspend = true })
		Eventually(reason, eventuallyTimeout, pollInterval).Should(Equal(reasonSuspended))
		list := &databasev1alpha1.BackupList{}
		Expect(k8sClient.List(ctx, list, client.InNamespace(ns))).To(Succeed())
		Expect(list.Items).To(BeEmpty())
	})
})

// completedBackups counts the Completed Backups a schedule has produced.
func completedBackups(namespace, schedule string) int {
	list := &databasev1alpha1.BackupList{}
	Expect(k8sClient.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{databasev1alpha1.ScheduledBackupLabel: schedule})).To(Succeed())
	n := 0
	for _, b := range list.Items {
		if b.Status.Phase == databasev1alpha1.BackupPhaseCompleted {
			n++
		}
	}
	return n
}

// markBackupCompleted waits for the controller to record its Pending status, so the test's update is not overwritten.
func markBackupCompleted(backup *databasev1alpha1.Backup) {
	Eventually(func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), backup); err != nil {
			return err
		}
		if backup.Status.Phase != databasev1alpha1.BackupPhasePending {
			return fmt.Errorf("phase %q, waiting for Pending", backup.Status.Phase)
		}
		backup.Status.Phase = databasev1alpha1.BackupPhaseCompleted
		return k8sClient.Status().Update(ctx, backup)
	}, eventuallyTimeout, pollInterval).Should(Succeed())
}

func updateScheduleSpec(schedule *databasev1alpha1.ScheduledBackup, mutate func(*databasev1alpha1.ScheduledBackup)) {
	Eventually(func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(schedule), schedule); err != nil {
			return err
		}
		mutate(schedule)
		return k8sClient.Update(ctx, schedule)
	}, eventuallyTimeout, pollInterval).Should(Succeed())
}
