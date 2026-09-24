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

// ScheduledBackupSpec creates Backups of a SereneDB on a cron schedule.
type ScheduledBackupSpec struct {
	// Cluster is the SereneDB to back up.
	Cluster ClusterRef `json:"cluster"`

	// Schedule is a five-field cron expression evaluated in UTC, for example "0 2 * * *".
	// +kubebuilder:validation:MinLength=9
	Schedule string `json:"schedule"`

	// Suspend stops new Backups from being created without deleting the schedule.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// Immediate creates one Backup right after the schedule is created.
	// +optional
	Immediate bool `json:"immediate,omitempty"`

	// Keep is how many completed Backups from this schedule to retain. Older ones are deleted. Nil keeps all.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Keep *int32 `json:"keep,omitempty"`

	// Method is copied into each Backup.
	// +kubebuilder:default=volumeSnapshot
	// +kubebuilder:validation:Enum=volumeSnapshot
	// +optional
	Method string `json:"method,omitempty"`

	// VolumeSnapshotClassName is copied into each Backup.
	// +optional
	VolumeSnapshotClassName *string `json:"volumeSnapshotClassName,omitempty"`
}

// ScheduledBackupStatus defines the observed state of ScheduledBackup.
type ScheduledBackupStatus struct {
	// LastScheduleTime is when the most recent Backup was created.
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// NextScheduleTime is when the next Backup is due.
	// +optional
	NextScheduleTime *metav1.Time `json:"nextScheduleTime,omitempty"`

	// LastBackup names the most recent Backup created by this schedule.
	// +optional
	LastBackup string `json:"lastBackup,omitempty"`

	// Conditions hold Ready, which is false when the schedule cannot be parsed.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`
// +kubebuilder:printcolumn:name="Last",type=date,JSONPath=`.status.lastScheduleTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ScheduledBackup creates Backups of a SereneDB on a schedule.
type ScheduledBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledBackupSpec   `json:"spec,omitempty"`
	Status ScheduledBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScheduledBackupList contains a list of ScheduledBackup.
type ScheduledBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScheduledBackup{}, &ScheduledBackupList{})
}
