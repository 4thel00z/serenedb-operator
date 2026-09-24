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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerSecretSpec defines a persistent secret in the SereneDB secrets manager, the credentials
// the server uses to reach object storage, HTTP endpoints, Iceberg catalogs and remote databases.
// The server stores persistent secrets unencrypted on its data volume.
type ServerSecretSpec struct {
	// Cluster is the SereneDB that holds the secret.
	Cluster ClusterRef `json:"cluster"`

	// Name of the secret on the server. Defaults to the object name.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	// +optional
	Name string `json:"name,omitempty"`

	// Type selects the service the secret is for.
	// +kubebuilder:validation:Enum=azure;gcs;http;huggingface;iceberg;openai;postgres;r2;s3
	Type string `json:"type"`

	// Scope is a path prefix the secret applies to, for example s3://bucket/. Empty applies to every path of the type.
	// +optional
	Scope string `json:"scope,omitempty"`

	// Options are non-sensitive CREATE SECRET options such as REGION or ENDPOINT, keyed by option name.
	// +optional
	Options map[string]string `json:"options,omitempty"`

	// ValuesFrom names a Kubernetes Secret whose every key becomes a CREATE SECRET option, such as KEY_ID and SECRET.
	// +optional
	ValuesFrom *corev1.LocalObjectReference `json:"valuesFrom,omitempty"`
}

// ServerSecretStatus defines the observed state of ServerSecret.
type ServerSecretStatus struct {
	DependentStatus `json:",inline"`

	// ValuesSecretVersion is the resourceVersion of the Kubernetes Secret last applied to the server.
	// +optional
	ValuesSecretVersion string `json:"valuesSecretVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServerSecret is a persistent secret in a SereneDB server's secrets manager.
type ServerSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerSecretSpec   `json:"spec,omitempty"`
	Status ServerSecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerSecretList contains a list of ServerSecret.
type ServerSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerSecret{}, &ServerSecretList{})
}
