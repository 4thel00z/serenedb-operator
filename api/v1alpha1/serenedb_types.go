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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultImageRepository is the upstream SereneDB server image.
	DefaultImageRepository = "serenedb/serenedb"
	// DefaultImageTag is the SereneDB release deployed when spec.image.tag is empty.
	DefaultImageTag = "26.09.2"
	// DefaultPostgresPort is the pg-wire listener port.
	DefaultPostgresPort int32 = 7890
	// DefaultHTTPPort is the Elasticsearch-compatible HTTP listener port.
	DefaultHTTPPort int32 = 9200
	// DefaultDataMountPath is where the server image expects its data directory.
	DefaultDataMountPath = "/var/lib/serenedb"
	// DefaultPasswordKey is the Secret key holding the postgres superuser password.
	DefaultPasswordKey = "postgres-password"
	// ImageUID is the numeric uid of the serenedb user in the upstream image. The image declares it by name,
	// and runAsNonRoot cannot be verified against a name, so the pod pins it.
	ImageUID int64 = 999
	// DefaultTerminationGracePeriodSeconds gives serened time to checkpoint on shutdown.
	DefaultTerminationGracePeriodSeconds int64 = 120

	// ConditionReady reports whether the database pod is serving connections.
	ConditionReady = "Ready"
	// ConditionProgressing reports whether the operator is still rolling out changes.
	ConditionProgressing = "Progressing"
)

// ImageSpec selects the SereneDB server image.
type ImageSpec struct {
	// Repository of the server image.
	// +kubebuilder:default="serenedb/serenedb"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag of the server image. Pin a release for production; "latest" is never re-pulled with IfNotPresent.
	// +kubebuilder:default="26.09.2"
	// +optional
	Tag string `json:"tag,omitempty"`

	// PullPolicy for the server image.
	// +kubebuilder:default=IfNotPresent
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// AuthSpec configures the initial postgres superuser password.
// serened honors the password only on the first boot with an empty data directory;
// rotate it later with ALTER ROLE postgres PASSWORD.
type AuthSpec struct {
	// ExistingSecret names a Secret holding the password under PasswordKey.
	// When empty the operator generates a Secret named after the SereneDB.
	// +optional
	ExistingSecret string `json:"existingSecret,omitempty"`

	// PasswordKey is the Secret key holding the password.
	// +kubebuilder:default="postgres-password"
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// PostgresListenerSpec configures the pg-wire listener.
type PostgresListenerSpec struct {
	// Port the pg-wire listener binds to.
	// +kubebuilder:default=7890
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// SSLMode is appended to the listener URL as ?sslmode=, for example "require". Empty keeps the server default.
	// +optional
	SSLMode string `json:"sslmode,omitempty"`
}

// HTTPListenerSpec configures the optional Elasticsearch-compatible HTTP listener.
type HTTPListenerSpec struct {
	// Enabled turns the HTTP listener on.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Port the HTTP listener binds to.
	// +kubebuilder:default=9200
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// CORSOrigins is passed verbatim as --http_cors_origins.
	// +optional
	CORSOrigins string `json:"corsOrigins,omitempty"`
}

// ListenersSpec configures the server listeners.
type ListenersSpec struct {
	// Postgres configures the pg-wire listener.
	// +kubebuilder:default={port: 7890}
	// +optional
	Postgres PostgresListenerSpec `json:"postgres,omitempty"`

	// HTTP configures the Elasticsearch-compatible HTTP listener.
	// +optional
	HTTP HTTPListenerSpec `json:"http,omitempty"`
}

// TLSSpec configures server TLS from a kubernetes.io/tls Secret.
type TLSSpec struct {
	// Enabled turns TLS on. Requires SecretName.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// SecretName names a kubernetes.io/tls Secret with tls.crt and tls.key. Works with cert-manager Certificates.
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// MinVersion is the minimum accepted TLS version.
	// +kubebuilder:default="1.2"
	// +kubebuilder:validation:Enum="1.2";"1.3"
	// +optional
	MinVersion string `json:"minVersion,omitempty"`
}

// ConfigSpec maps to serened flags written into the flagfile.
type ConfigSpec struct {
	// LogLevel is the minimum log severity.
	// +kubebuilder:default=info
	// +kubebuilder:validation:Enum=trace;debug;info;warning;error;fatal
	// +optional
	LogLevel string `json:"logLevel,omitempty"`

	// MaxConnections caps concurrent client connections. 0 means unlimited.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxConnections int32 `json:"maxConnections,omitempty"`

	// CPUThreads sets --cpu_threads. 0 auto-detects from the node, not the pod CPU limit.
	// +kubebuilder:validation:Minimum=0
	// +optional
	CPUThreads int32 `json:"cpuThreads,omitempty"`

	// IOThreads sets --io_threads. 0 auto-detects.
	// +kubebuilder:validation:Minimum=0
	// +optional
	IOThreads int32 `json:"ioThreads,omitempty"`

	// BackgroundThreads sets --background_threads. 0 auto-detects.
	// +kubebuilder:validation:Minimum=0
	// +optional
	BackgroundThreads int32 `json:"backgroundThreads,omitempty"`

	// AuthTimeout sets --auth_timeout.
	// +kubebuilder:default="30s"
	// +optional
	AuthTimeout string `json:"authTimeout,omitempty"`

	// IdleSessionTimeout sets --idle_session_timeout.
	// +kubebuilder:default="75s"
	// +optional
	IdleSessionTimeout string `json:"idleSessionTimeout,omitempty"`

	// PGMaxMessageBytes sets --pg_max_message_bytes. 0 keeps the built-in default.
	// +kubebuilder:validation:Minimum=0
	// +optional
	PGMaxMessageBytes int64 `json:"pgMaxMessageBytes,omitempty"`

	// HBA is pg_hba.conf content, verbatim. Empty keeps the engine default.
	// +optional
	HBA string `json:"hba,omitempty"`

	// ExtraFlags are appended verbatim to the flagfile, one "--flag=value" per entry.
	// +optional
	ExtraFlags []string `json:"extraFlags,omitempty"`
}

// RetentionPolicySpec controls the fate of the data volume.
type RetentionPolicySpec struct {
	// WhenDeleted decides whether the data PVC is removed with the SereneDB.
	// +kubebuilder:default=Retain
	// +kubebuilder:validation:Enum=Retain;Delete
	// +optional
	WhenDeleted string `json:"whenDeleted,omitempty"`

	// WhenScaled decides whether the data PVC is removed on scale down.
	// +kubebuilder:default=Retain
	// +kubebuilder:validation:Enum=Retain;Delete
	// +optional
	WhenScaled string `json:"whenScaled,omitempty"`
}

// PersistenceSpec configures the data volume. Volume claim fields are immutable
// because the StatefulSet volumeClaimTemplates cannot change after creation.
// +kubebuilder:validation:XValidation:rule="has(self.size) == has(oldSelf.size) && (!has(self.size) || self.size == oldSelf.size)",message="persistence.size is immutable"
// +kubebuilder:validation:XValidation:rule="has(self.storageClassName) == has(oldSelf.storageClassName) && (!has(self.storageClassName) || self.storageClassName == oldSelf.storageClassName)",message="persistence.storageClassName is immutable"
// +kubebuilder:validation:XValidation:rule="has(self.accessModes) == has(oldSelf.accessModes) && (!has(self.accessModes) || self.accessModes == oldSelf.accessModes)",message="persistence.accessModes is immutable"
// +kubebuilder:validation:XValidation:rule="has(self.existingClaim) == has(oldSelf.existingClaim) && (!has(self.existingClaim) || self.existingClaim == oldSelf.existingClaim)",message="persistence.existingClaim is immutable"
type PersistenceSpec struct {
	// Size of the data volume.
	// +kubebuilder:default="20Gi"
	// +optional
	Size *resource.Quantity `json:"size,omitempty"`

	// StorageClassName of the data volume. Nil uses the cluster default.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// AccessModes of the data volume.
	// +kubebuilder:default={ReadWriteOnce}
	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`

	// ExistingClaim reuses a pre-created PVC instead of a volume claim template.
	// +optional
	ExistingClaim string `json:"existingClaim,omitempty"`

	// RetentionPolicy controls the data PVC lifecycle. Requires Kubernetes 1.27 or newer.
	// +kubebuilder:default={whenDeleted: Retain, whenScaled: Retain}
	// +optional
	RetentionPolicy RetentionPolicySpec `json:"retentionPolicy,omitempty"`
}

// ServiceSpec configures the client-facing Service.
type ServiceSpec struct {
	// Type of the client Service.
	// +kubebuilder:default=ClusterIP
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +optional
	Type corev1.ServiceType `json:"type,omitempty"`

	// NodePort pins the pg-wire node port when Type is NodePort. 0 auto-assigns.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	// +optional
	NodePort int32 `json:"nodePort,omitempty"`

	// Annotations are added to the client Service.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// NetworkPolicySpec configures an ingress NetworkPolicy for the database pod.
type NetworkPolicySpec struct {
	// Enabled creates a NetworkPolicy allowing ingress only on the serving ports.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// SereneDBSpec defines the desired state of a single-node SereneDB server.
type SereneDBSpec struct {
	// Image selects the server image.
	// +kubebuilder:default={repository: "serenedb/serenedb", tag: "26.09.2", pullPolicy: IfNotPresent}
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// ImagePullSecrets for the server image.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Auth configures the initial superuser password.
	// +kubebuilder:default={passwordKey: "postgres-password"}
	// +optional
	Auth AuthSpec `json:"auth,omitempty"`

	// Listeners configures the server listeners.
	// +kubebuilder:default={postgres: {port: 7890}}
	// +optional
	Listeners ListenersSpec `json:"listeners,omitempty"`

	// TLS configures server TLS.
	// +optional
	TLS TLSSpec `json:"tls,omitempty"`

	// Config maps to serened flags.
	// +kubebuilder:default={logLevel: info, authTimeout: "30s", idleSessionTimeout: "75s"}
	// +optional
	Config ConfigSpec `json:"config,omitempty"`

	// Persistence configures the data volume.
	// +kubebuilder:default={size: "20Gi", accessModes: {ReadWriteOnce}, retentionPolicy: {whenDeleted: Retain, whenScaled: Retain}}
	// +optional
	Persistence PersistenceSpec `json:"persistence,omitempty"`

	// Resources for the server container. Recommended production start: requests equal limits, at least 2 CPU and 4Gi.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// TerminationGracePeriodSeconds is the graceful shutdown budget. Raise it for large data directories.
	// +kubebuilder:default=120
	// +kubebuilder:validation:Minimum=0
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// Service configures the client Service.
	// +kubebuilder:default={type: ClusterIP}
	// +optional
	Service ServiceSpec `json:"service,omitempty"`

	// NetworkPolicy configures pod ingress restrictions.
	// +optional
	NetworkPolicy NetworkPolicySpec `json:"networkPolicy,omitempty"`

	// Env adds environment variables to the server container, for example NUMA=disable.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// PodAnnotations are added to the database pod.
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// PodLabels are added to the database pod.
	// +optional
	PodLabels map[string]string `json:"podLabels,omitempty"`

	// NodeSelector for the database pod.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations for the database pod.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity for the database pod.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// PriorityClassName for the database pod.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// SereneDBStatus defines the observed state of SereneDB.
type SereneDBStatus struct {
	// ObservedGeneration is the spec generation the status reflects.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions hold Ready and Progressing.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ReadyReplicas is the number of serving database pods, 0 or 1.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Version is the image tag of the last completed rollout.
	// +optional
	Version string `json:"version,omitempty"`

	// SecretName holds the superuser password Secret in use.
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// Host is the in-cluster DNS name of the client Service.
	// +optional
	Host string `json:"host,omitempty"`

	// Port is the pg-wire port of the client Service.
	// +optional
	Port int32 `json:"port,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sdb
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.status.host`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SereneDB is a single-node SereneDB server with a persistent data volume.
type SereneDB struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SereneDBSpec   `json:"spec,omitempty"`
	Status SereneDBStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SereneDBList contains a list of SereneDB.
type SereneDBList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SereneDB `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SereneDB{}, &SereneDBList{})
}
