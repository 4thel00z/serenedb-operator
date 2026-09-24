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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// BackupMethodVolumeSnapshot snapshots the data volume through the CSI snapshot API after a CHECKPOINT.
	BackupMethodVolumeSnapshot = "volumeSnapshot"

	// BackupPhasePending means the backup has not started.
	BackupPhasePending = "Pending"
	// BackupPhaseRunning means the snapshot has been requested and is not yet ready to use.
	BackupPhaseRunning = "Running"
	// BackupPhaseCompleted means the snapshot is ready to use.
	BackupPhaseCompleted = "Completed"
	// BackupPhaseFailed means the backup stopped with an error recorded in status.error.
	BackupPhaseFailed = "Failed"

	// ScheduledBackupLabel marks Backups created by a ScheduledBackup with its name.
	ScheduledBackupLabel = "database.serenedb.com/scheduled-backup"
)

// BackupSpec requests one backup of a SereneDB.
type BackupSpec struct {
	// Cluster is the SereneDB to back up.
	Cluster ClusterRef `json:"cluster"`

	// Method selects how the backup is taken. volumeSnapshot runs CHECKPOINT and creates a VolumeSnapshot of the data volume.
	// +kubebuilder:default=volumeSnapshot
	// +kubebuilder:validation:Enum=volumeSnapshot
	// +optional
	Method string `json:"method,omitempty"`

	// VolumeSnapshotClassName selects the VolumeSnapshotClass. Nil uses the cluster default.
	// +optional
	VolumeSnapshotClassName *string `json:"volumeSnapshotClassName,omitempty"`
}

// BackupStatus defines the observed state of Backup.
type BackupStatus struct {
	// Phase is Pending, Running, Completed or Failed.
	// +optional
	Phase string `json:"phase,omitempty"`

	// StartedAt is when the CHECKPOINT ran.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is when the snapshot became ready to use.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// SnapshotName is the VolumeSnapshot holding the backup.
	// +optional
	SnapshotName string `json:"snapshotName,omitempty"`

	// Error explains a Failed phase.
	// +optional
	Error string `json:"error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="Method",type=string,JSONPath=`.spec.method`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Snapshot",type=string,JSONPath=`.status.snapshotName`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Backup is one backup of a SereneDB data volume.
type Backup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupSpec   `json:"spec,omitempty"`
	Status BackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BackupList contains a list of Backup.
type BackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Backup{}, &BackupList{})
}
