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

package resources

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
)

// VolumeSnapshotGVK identifies the CSI snapshot API the operator uses without importing its client.
var VolumeSnapshotGVK = schema.GroupVersionKind{Group: "snapshot.storage.k8s.io", Version: "v1", Kind: "VolumeSnapshot"}

// DataClaimName returns the PVC that holds the data directory of a SereneDB.
func DataClaimName(db *databasev1alpha1.SereneDB) string {
	if db.Spec.Persistence.ExistingClaim != "" {
		return db.Spec.Persistence.ExistingClaim
	}
	return fmt.Sprintf("%s-%s-0", dataVolume, Name(db))
}

// VolumeSnapshot builds the snapshot request for a Backup.
func VolumeSnapshot(backup *databasev1alpha1.Backup, claimName string) *unstructured.Unstructured {
	source := map[string]any{"persistentVolumeClaimName": claimName}
	spec := map[string]any{"source": source}
	if backup.Spec.VolumeSnapshotClassName != nil {
		spec["volumeSnapshotClassName"] = *backup.Spec.VolumeSnapshotClassName
	}
	u := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      backup.Name,
			"namespace": backup.Namespace,
			"labels":    map[string]any{"app.kubernetes.io/managed-by": ManagedBy, "app.kubernetes.io/instance": backup.Spec.Cluster.Name},
		},
		"spec": spec,
	}}
	u.SetGroupVersionKind(VolumeSnapshotGVK)
	return u
}

// SnapshotOutcome reads a VolumeSnapshot status: ready reports readyToUse, failure carries status.error.message.
func SnapshotOutcome(u *unstructured.Unstructured) (ready bool, failure string) {
	ready, _, _ = unstructured.NestedBool(u.Object, "status", "readyToUse")
	failure, _, _ = unstructured.NestedString(u.Object, "status", "error", "message")
	return ready, failure
}
