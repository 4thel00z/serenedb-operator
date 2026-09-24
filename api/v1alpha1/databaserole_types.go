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

// PasswordSecretRef points at a Kubernetes Secret key holding a role password.
type PasswordSecretRef struct {
	// Name of the Secret in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key inside the Secret.
	// +kubebuilder:default=password
	// +optional
	Key string `json:"key,omitempty"`
}

// DatabaseRoleSpec defines a role inside a SereneDB server. Roles are global to the server.
type DatabaseRoleSpec struct {
	// Cluster is the SereneDB that hosts the role.
	Cluster ClusterRef `json:"cluster"`

	// Name of the role on the server. Defaults to the object name.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	// +optional
	Name string `json:"name,omitempty"`

	// Login lets the role start client connections.
	// +optional
	Login bool `json:"login,omitempty"`

	// Superuser bypasses every permission check.
	// +optional
	Superuser bool `json:"superuser,omitempty"`

	// CreateDB lets the role create databases.
	// +optional
	CreateDB bool `json:"createDB,omitempty"`

	// CreateRole lets the role create, alter and drop other roles.
	// +optional
	CreateRole bool `json:"createRole,omitempty"`

	// Inherit makes the role use privileges of roles it is a member of.
	// +kubebuilder:default=true
	// +optional
	Inherit *bool `json:"inherit,omitempty"`

	// ConnectionLimit is stored by the server for compatibility and not enforced. Nil leaves it unset.
	// +optional
	ConnectionLimit *int32 `json:"connectionLimit,omitempty"`

	// ValidUntil is the timestamp after which the password stops working, in any format the server accepts.
	// +optional
	ValidUntil string `json:"validUntil,omitempty"`

	// PasswordSecret holds the password. It is applied on creation and again whenever the Secret changes.
	// +optional
	PasswordSecret *PasswordSecretRef `json:"passwordSecret,omitempty"`

	// InRoles lists roles this role is a member of. Memberships not listed here are revoked.
	// +optional
	InRoles []string `json:"inRoles,omitempty"`

	// ReclaimPolicy decides whether the role is dropped when this object is deleted.
	// +kubebuilder:default=Retain
	// +kubebuilder:validation:Enum=Retain;Delete
	// +optional
	ReclaimPolicy string `json:"reclaimPolicy,omitempty"`
}

// DatabaseRoleStatus defines the observed state of DatabaseRole.
type DatabaseRoleStatus struct {
	DependentStatus `json:",inline"`

	// PasswordSecretVersion is the resourceVersion of the password Secret last applied to the role.
	// +optional
	PasswordSecretVersion string `json:"passwordSecretVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DatabaseRole is a role created inside a SereneDB server.
type DatabaseRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseRoleSpec   `json:"spec,omitempty"`
	Status DatabaseRoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseRoleList contains a list of DatabaseRole.
type DatabaseRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseRole{}, &DatabaseRoleList{})
}
