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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// ReclaimRetain leaves the server-side object in place when the Kubernetes object is deleted.
	ReclaimRetain = "Retain"
	// ReclaimDelete drops the server-side object when the Kubernetes object is deleted.
	ReclaimDelete = "Delete"

	// DependentFinalizer guards server-side cleanup for Database, DatabaseRole and ServerSecret.
	DependentFinalizer = "database.serenedb.com/cleanup"
)

// ClusterRef names the SereneDB in the same namespace that an object belongs to.
type ClusterRef struct {
	// Name of the SereneDB.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// DependentStatus is the observed state shared by the objects managed through SQL.
type DependentStatus struct {
	// ObservedGeneration is the spec generation the status reflects.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions hold Ready.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
