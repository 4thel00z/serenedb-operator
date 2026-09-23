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

// Package resources builds the Kubernetes objects that make up one SereneDB server.
package resources

import (
	"fmt"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
)

const (
	// ManagedBy is the app.kubernetes.io/managed-by label value.
	ManagedBy = "serenedb-operator"
	// AppName is the app.kubernetes.io/name label value.
	AppName = "serenedb"
	// ConfigHashAnnotation on the pod template rolls the pod when the flagfile changes.
	ConfigHashAnnotation = "database.serenedb.com/config-hash"

	configMountPath = "/etc/serenedb"
	tlsMountPath    = "/etc/serenedb-tls"
	containerName   = "serened"
	dataVolume      = "data"
	configVolume    = "config"
	tlsVolume       = "tls"
	postgresPort    = "postgres"
	httpPort        = "http"
	flagfileKey     = "serened.conf"
	hbaKey          = "pg_hba.conf"
)

// Name returns the shared name for the StatefulSet, ConfigMap, client Service and generated Secret.
func Name(db *databasev1alpha1.SereneDB) string {
	return db.Name
}

// HeadlessServiceName returns the name of the StatefulSet governing Service.
func HeadlessServiceName(db *databasev1alpha1.SereneDB) string {
	return db.Name + "-hl"
}

// SecretName returns the Secret the server reads its superuser password from.
func SecretName(db *databasev1alpha1.SereneDB) string {
	if db.Spec.Auth.ExistingSecret != "" {
		return db.Spec.Auth.ExistingSecret
	}
	return Name(db)
}

// PasswordKey returns the Secret key holding the superuser password.
func PasswordKey(db *databasev1alpha1.SereneDB) string {
	if db.Spec.Auth.PasswordKey != "" {
		return db.Spec.Auth.PasswordKey
	}
	return databasev1alpha1.DefaultPasswordKey
}

// Image returns the fully qualified server image reference.
func Image(db *databasev1alpha1.SereneDB) string {
	return fmt.Sprintf("%s:%s", ImageRepository(db), ImageTag(db))
}

// ImageRepository returns the server image repository with the default applied.
func ImageRepository(db *databasev1alpha1.SereneDB) string {
	if db.Spec.Image.Repository != "" {
		return db.Spec.Image.Repository
	}
	return databasev1alpha1.DefaultImageRepository
}

// ImageTag returns the server image tag with the default applied.
func ImageTag(db *databasev1alpha1.SereneDB) string {
	if db.Spec.Image.Tag != "" {
		return db.Spec.Image.Tag
	}
	return databasev1alpha1.DefaultImageTag
}

// PostgresPort returns the pg-wire port with the default applied.
func PostgresPort(db *databasev1alpha1.SereneDB) int32 {
	if db.Spec.Listeners.Postgres.Port != 0 {
		return db.Spec.Listeners.Postgres.Port
	}
	return databasev1alpha1.DefaultPostgresPort
}

// HTTPPort returns the HTTP listener port with the default applied.
func HTTPPort(db *databasev1alpha1.SereneDB) int32 {
	if db.Spec.Listeners.HTTP.Port != 0 {
		return db.Spec.Listeners.HTTP.Port
	}
	return databasev1alpha1.DefaultHTTPPort
}

// Host returns the in-cluster DNS name of the client Service.
func Host(db *databasev1alpha1.SereneDB) string {
	return fmt.Sprintf("%s.%s.svc", Name(db), db.Namespace)
}

// Labels returns the labels stamped on every owned object.
func Labels(db *databasev1alpha1.SereneDB) map[string]string {
	labels := SelectorLabels(db)
	labels["app.kubernetes.io/managed-by"] = ManagedBy
	labels["app.kubernetes.io/version"] = ImageTag(db)
	return labels
}

// SelectorLabels returns the immutable subset of labels used by selectors.
func SelectorLabels(db *databasev1alpha1.SereneDB) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     AppName,
		"app.kubernetes.io/instance": db.Name,
	}
}
