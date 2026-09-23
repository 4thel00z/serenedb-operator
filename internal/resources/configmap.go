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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
)

// ConfigMap builds the flagfile the image entrypoint passes to serened via --flagfile.
func ConfigMap(db *databasev1alpha1.SereneDB) *corev1.ConfigMap {
	data := map[string]string{flagfileKey: Flagfile(db)}
	if db.Spec.Config.HBA != "" {
		data[hbaKey] = db.Spec.Config.HBA
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name(db),
			Namespace: db.Namespace,
			Labels:    Labels(db),
		},
		Data: data,
	}
}

// Flagfile renders the serened flagfile, one --flag=value per line.
func Flagfile(db *databasev1alpha1.SereneDB) string {
	cfg := db.Spec.Config
	lines := []string{
		"--listen=" + ListenValue(db),
		"--log_storage=stdout",
		"--log_level=" + logLevel(cfg),
	}
	lines = appendNonZero(lines, "--max_connections", int64(cfg.MaxConnections))
	lines = appendNonZero(lines, "--cpu_threads", int64(cfg.CPUThreads))
	lines = appendNonZero(lines, "--io_threads", int64(cfg.IOThreads))
	lines = appendNonZero(lines, "--background_threads", int64(cfg.BackgroundThreads))
	lines = append(lines, "--auth_timeout="+orDefault(cfg.AuthTimeout, "30s"))
	lines = append(lines, "--idle_session_timeout="+orDefault(cfg.IdleSessionTimeout, "75s"))
	lines = appendNonZero(lines, "--pg_max_message_bytes", cfg.PGMaxMessageBytes)
	if db.Spec.Listeners.HTTP.Enabled && db.Spec.Listeners.HTTP.CORSOrigins != "" {
		lines = append(lines, "--http_cors_origins="+db.Spec.Listeners.HTTP.CORSOrigins)
	}
	if db.Spec.TLS.Enabled {
		lines = append(lines,
			"--tls_cert="+tlsMountPath+"/tls.crt",
			"--tls_key="+tlsMountPath+"/tls.key",
			"--tls_min_version="+orDefault(db.Spec.TLS.MinVersion, "1.2"),
		)
	}
	if cfg.HBA != "" {
		lines = append(lines, "--hba_config="+configMountPath+"/"+hbaKey)
	}
	lines = append(lines, cfg.ExtraFlags...)
	return strings.Join(lines, "\n") + "\n"
}

// ListenValue joins all listeners into the single comma-separated --listen value serened expects.
func ListenValue(db *databasev1alpha1.SereneDB) string {
	pg := fmt.Sprintf("postgres://0.0.0.0:%d", PostgresPort(db))
	if db.Spec.Listeners.Postgres.SSLMode != "" {
		pg += "?sslmode=" + db.Spec.Listeners.Postgres.SSLMode
	}
	if !db.Spec.Listeners.HTTP.Enabled {
		return pg
	}
	scheme := "http"
	if db.Spec.TLS.Enabled {
		scheme = "https"
	}
	return pg + "," + fmt.Sprintf("%s://0.0.0.0:%d?api=es", scheme, HTTPPort(db))
}

// ConfigHash digests the ConfigMap data so the pod template changes together with the flagfile.
func ConfigHash(cm *corev1.ConfigMap) string {
	keys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(cm.Data[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func logLevel(cfg databasev1alpha1.ConfigSpec) string {
	return orDefault(cfg.LogLevel, "info")
}

func orDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func appendNonZero(lines []string, flag string, value int64) []string {
	if value == 0 {
		return lines
	}
	return append(lines, fmt.Sprintf("%s=%d", flag, value))
}
