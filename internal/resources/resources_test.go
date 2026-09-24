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
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
)

const testName = "mydb"

func minimal() *databasev1alpha1.SereneDB {
	return &databasev1alpha1.SereneDB{ObjectMeta: metav1.ObjectMeta{Name: testName, Namespace: "team"}}
}

func full() *databasev1alpha1.SereneDB {
	db := minimal()
	db.Spec.Image = databasev1alpha1.ImageSpec{Repository: "registry.local/serenedb", Tag: "26.09.1", PullPolicy: corev1.PullAlways}
	db.Spec.Listeners.Postgres = databasev1alpha1.PostgresListenerSpec{Port: 5432, SSLMode: "require"}
	db.Spec.Listeners.HTTP = databasev1alpha1.HTTPListenerSpec{Enabled: true, Port: 9201, CORSOrigins: "https://app.example"}
	db.Spec.TLS = databasev1alpha1.TLSSpec{Enabled: true, SecretName: "mydb-tls", MinVersion: "1.3"}
	db.Spec.Config = databasev1alpha1.ConfigSpec{
		LogLevel:           "debug",
		MaxConnections:     100,
		CPUThreads:         4,
		IOThreads:          2,
		BackgroundThreads:  3,
		AuthTimeout:        "10s",
		IdleSessionTimeout: "5m",
		PGMaxMessageBytes:  1024,
		HBA:                "host all all 0.0.0.0/0 scram-sha-256\n",
		ExtraFlags:         []string{"--foo=bar"},
	}
	db.Spec.Persistence.RetentionPolicy = databasev1alpha1.RetentionPolicySpec{WhenDeleted: "Delete", WhenScaled: "Retain"}
	db.Spec.Service = databasev1alpha1.ServiceSpec{Type: corev1.ServiceTypeNodePort, NodePort: 30789, Annotations: map[string]string{"a": "b"}}
	return db
}

func assertContains(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			t.Errorf("expected %q to contain %q", haystack, n)
		}
	}
}

func assertNotContains(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			t.Errorf("expected %q not to contain %q", haystack, n)
		}
	}
}

func TestFlagfileDefaults(t *testing.T) {
	got := Flagfile(minimal())
	assertContains(t, got,
		"--listen=postgres://0.0.0.0:7890\n",
		"--log_storage=stdout\n",
		"--log_level=info\n",
		"--auth_timeout=30s\n",
		"--idle_session_timeout=75s\n",
	)
	assertNotContains(t, got, "--max_connections", "--cpu_threads", "--io_threads", "--background_threads",
		"--pg_max_message_bytes", "--tls_", "--hba_config", "--http_cors_origins")
}

func TestFlagfileFull(t *testing.T) {
	got := Flagfile(full())
	assertContains(t, got,
		"--listen=postgres://0.0.0.0:5432?sslmode=require,https://0.0.0.0:9201?api=es\n",
		"--log_level=debug\n",
		"--max_connections=100\n",
		"--cpu_threads=4\n",
		"--io_threads=2\n",
		"--background_threads=3\n",
		"--auth_timeout=10s\n",
		"--idle_session_timeout=5m\n",
		"--pg_max_message_bytes=1024\n",
		"--http_cors_origins=https://app.example\n",
		"--tls_cert=/etc/serenedb-tls/tls.crt\n",
		"--tls_key=/etc/serenedb-tls/tls.key\n",
		"--tls_min_version=1.3\n",
		"--hba_config=/etc/serenedb/pg_hba.conf\n",
		"--foo=bar\n",
	)
}

func TestListenValueHTTPWithoutTLS(t *testing.T) {
	db := minimal()
	db.Spec.Listeners.HTTP.Enabled = true
	if got := ListenValue(db); got != "postgres://0.0.0.0:7890,http://0.0.0.0:9200?api=es" {
		t.Fatalf("unexpected listen value %q", got)
	}
}

func TestConfigMapCarriesHBAOnlyWhenSet(t *testing.T) {
	if _, ok := ConfigMap(minimal()).Data["pg_hba.conf"]; ok {
		t.Fatal("pg_hba.conf must be absent by default")
	}
	cm := ConfigMap(full())
	if cm.Data["pg_hba.conf"] != full().Spec.Config.HBA {
		t.Fatal("pg_hba.conf must carry spec.config.hba verbatim")
	}
}

func TestConfigHashTracksData(t *testing.T) {
	a := ConfigHash(ConfigMap(minimal()))
	b := ConfigHash(ConfigMap(minimal()))
	c := ConfigHash(ConfigMap(full()))
	if a != b {
		t.Fatal("hash must be deterministic")
	}
	if a == c {
		t.Fatal("hash must change with the data")
	}
}

func TestServiceDefaults(t *testing.T) {
	svc := Service(minimal())
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("type %q", svc.Spec.Type)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 7890 || svc.Spec.Ports[0].TargetPort.String() != "postgres" {
		t.Fatalf("ports %+v", svc.Spec.Ports)
	}
	if svc.Spec.Selector["app.kubernetes.io/instance"] != testName {
		t.Fatalf("selector %+v", svc.Spec.Selector)
	}
}

func TestServiceNodePortPinned(t *testing.T) {
	svc := Service(full())
	if svc.Spec.Type != corev1.ServiceTypeNodePort || svc.Spec.Ports[0].NodePort != 30789 {
		t.Fatalf("ports %+v", svc.Spec.Ports)
	}
	if len(svc.Spec.Ports) != 2 || svc.Spec.Ports[1].Name != "http" || svc.Spec.Ports[1].Port != 9201 {
		t.Fatalf("ports %+v", svc.Spec.Ports)
	}
	if svc.Annotations["a"] != "b" {
		t.Fatal("annotations must pass through")
	}
}

func TestHeadlessService(t *testing.T) {
	svc := HeadlessService(full())
	if svc.Name != testName+"-hl" || svc.Spec.ClusterIP != corev1.ClusterIPNone || !svc.Spec.PublishNotReadyAddresses {
		t.Fatalf("headless %+v", svc.Spec)
	}
	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("headless service exposes only pg-wire, got %+v", svc.Spec.Ports)
	}
}

func TestStatefulSetDefaults(t *testing.T) {
	sts := StatefulSet(minimal(), "abc")
	if *sts.Spec.Replicas != 1 || sts.Spec.ServiceName != "mydb-hl" {
		t.Fatalf("spec %+v", sts.Spec)
	}
	if sts.Spec.Template.Annotations[ConfigHashAnnotation] != "abc" {
		t.Fatal("config hash annotation missing")
	}
	c := sts.Spec.Template.Spec.Containers[0]
	if c.Image != "serenedb/serenedb:"+databasev1alpha1.DefaultImageTag {
		t.Fatalf("image %q", c.Image)
	}
	if strings.Join(c.Args, " ") != "serened /var/lib/serenedb" {
		t.Fatalf("args %v", c.Args)
	}
	if c.Env[0].Name != "POSTGRES_PASSWORD" || c.Env[0].ValueFrom.SecretKeyRef.Name != testName || c.Env[0].ValueFrom.SecretKeyRef.Key != "postgres-password" {
		t.Fatalf("env %+v", c.Env)
	}
	if c.StartupProbe.FailureThreshold != 60 || c.StartupProbe.PeriodSeconds != 5 {
		t.Fatalf("startup probe %+v", c.StartupProbe)
	}
	if strings.Join(c.StartupProbe.Exec.Command, " ") != "pg_isready -h 127.0.0.1 -p 7890 -U postgres" {
		t.Fatalf("probe command %v", c.StartupProbe.Exec.Command)
	}
	if *sts.Spec.Template.Spec.TerminationGracePeriodSeconds != 120 {
		t.Fatal("termination grace period default")
	}
	if *sts.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Fatal("service account token must not be mounted")
	}
	if *sts.Spec.Template.Spec.SecurityContext.FSGroup != 0 {
		t.Fatal("fsGroup must be 0")
	}
	if !*c.SecurityContext.RunAsNonRoot || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Fatal("container hardening")
	}
	if *c.SecurityContext.RunAsUser != 999 {
		t.Fatal("runAsUser must pin the image uid so runAsNonRoot can be verified")
	}
	vct := sts.Spec.VolumeClaimTemplates
	if len(vct) != 1 || vct[0].Name != "data" || vct[0].Spec.Resources.Requests.Storage().Cmp(resource.MustParse("20Gi")) != 0 {
		t.Fatalf("volume claim templates %+v", vct)
	}
	if vct[0].Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Fatal("default access mode")
	}
	if sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted != appsv1.RetainPersistentVolumeClaimRetentionPolicyType {
		t.Fatal("default retention must be Retain")
	}
	if len(sts.Spec.Template.Spec.Volumes) != 1 || sts.Spec.Template.Spec.Volumes[0].ConfigMap.Name != testName {
		t.Fatalf("volumes %+v", sts.Spec.Template.Spec.Volumes)
	}
}

func TestStatefulSetFull(t *testing.T) {
	db := full()
	db.Spec.Env = []corev1.EnvVar{{Name: "NUMA", Value: "disable"}}
	db.Spec.PodLabels = map[string]string{"team": "search"}
	db.Spec.PodAnnotations = map[string]string{"note": "x"}
	sts := StatefulSet(db, "h")
	c := sts.Spec.Template.Spec.Containers[0]
	if c.Image != "registry.local/serenedb:26.09.1" || c.ImagePullPolicy != corev1.PullAlways {
		t.Fatalf("image %q %q", c.Image, c.ImagePullPolicy)
	}
	if len(c.Ports) != 2 || c.Ports[1].ContainerPort != 9201 {
		t.Fatalf("ports %+v", c.Ports)
	}
	if c.Env[1].Name != "NUMA" {
		t.Fatalf("env %+v", c.Env)
	}
	if len(c.VolumeMounts) != 3 || c.VolumeMounts[2].MountPath != "/etc/serenedb-tls" {
		t.Fatalf("mounts %+v", c.VolumeMounts)
	}
	if len(sts.Spec.Template.Spec.Volumes) != 2 || sts.Spec.Template.Spec.Volumes[1].Secret.SecretName != "mydb-tls" {
		t.Fatalf("volumes %+v", sts.Spec.Template.Spec.Volumes)
	}
	if sts.Spec.Template.Labels["team"] != "search" || sts.Spec.Template.Annotations["note"] != "x" {
		t.Fatal("pod labels and annotations must pass through")
	}
	if sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted != appsv1.DeletePersistentVolumeClaimRetentionPolicyType {
		t.Fatal("retention whenDeleted must be Delete")
	}
	if strings.Join(c.StartupProbe.Exec.Command, " ") != "pg_isready -h 127.0.0.1 -p 5432 -U postgres" {
		t.Fatalf("probe command %v", c.StartupProbe.Exec.Command)
	}
}

func TestStatefulSetExistingClaim(t *testing.T) {
	db := minimal()
	db.Spec.Persistence.ExistingClaim = "prewarmed"
	sts := StatefulSet(db, "h")
	if len(sts.Spec.VolumeClaimTemplates) != 0 || sts.Spec.PersistentVolumeClaimRetentionPolicy != nil {
		t.Fatal("existing claim must disable the claim template")
	}
	vols := sts.Spec.Template.Spec.Volumes
	if len(vols) != 2 || vols[1].Name != "data" || vols[1].PersistentVolumeClaim.ClaimName != "prewarmed" {
		t.Fatalf("volumes %+v", vols)
	}
}

func TestBootstrapSeedsClaimFromSnapshot(t *testing.T) {
	db := minimal()
	db.Spec.Bootstrap = &databasev1alpha1.BootstrapSpec{VolumeSnapshotName: "nightly-1"}
	claim := VolumeClaimTemplate(db)
	if claim.Spec.DataSource == nil || claim.Spec.DataSource.Kind != "VolumeSnapshot" || claim.Spec.DataSource.Name != "nightly-1" {
		t.Fatalf("data source %+v", claim.Spec.DataSource)
	}
	if *claim.Spec.DataSource.APIGroup != "snapshot.storage.k8s.io" {
		t.Fatal("api group")
	}
	if VolumeClaimTemplate(minimal()).Spec.DataSource != nil {
		t.Fatal("no bootstrap means no data source")
	}
}

func TestDataClaimName(t *testing.T) {
	if DataClaimName(minimal()) != "data-mydb-0" {
		t.Fatal(DataClaimName(minimal()))
	}
	db := minimal()
	db.Spec.Persistence.ExistingClaim = "pre"
	if DataClaimName(db) != "pre" {
		t.Fatal("existing claim")
	}
}

func TestVolumeSnapshot(t *testing.T) {
	b := &databasev1alpha1.Backup{ObjectMeta: metav1.ObjectMeta{Name: "b1", Namespace: "team"}}
	b.Spec.Cluster.Name = "mydb"
	b.Spec.VolumeSnapshotClassName = ptr.To("csi-fast")
	u := VolumeSnapshot(b, "data-mydb-0")
	if u.GetKind() != "VolumeSnapshot" || u.GetName() != "b1" || u.GetNamespace() != "team" {
		t.Fatalf("meta %v", u.Object["metadata"])
	}
	spec := u.Object["spec"].(map[string]any)
	if spec["volumeSnapshotClassName"] != "csi-fast" || spec["source"].(map[string]any)["persistentVolumeClaimName"] != "data-mydb-0" {
		t.Fatalf("spec %v", spec)
	}
	ready, failure := SnapshotOutcome(u)
	if ready || failure != "" {
		t.Fatal("fresh snapshot is neither ready nor failed")
	}
}

func TestSecretNaming(t *testing.T) {
	db := minimal()
	if SecretName(db) != testName || PasswordKey(db) != "postgres-password" {
		t.Fatal("defaults")
	}
	db.Spec.Auth = databasev1alpha1.AuthSpec{ExistingSecret: "creds", PasswordKey: "pw"}
	if SecretName(db) != "creds" || PasswordKey(db) != "pw" {
		t.Fatal("overrides")
	}
}

func TestSecretOwnedByDatabaseFollowsRetention(t *testing.T) {
	if SecretOwnedByDatabase(minimal()) {
		t.Fatal("secret must outlive the database when the volume is retained")
	}
	if !SecretOwnedByDatabase(full()) {
		t.Fatal("secret follows the database when the volume is deleted")
	}
}

func TestGeneratedSecret(t *testing.T) {
	a, err := GeneratedSecret(minimal())
	if err != nil {
		t.Fatal(err)
	}
	b, err := GeneratedSecret(minimal())
	if err != nil {
		t.Fatal(err)
	}
	if len(a.StringData["postgres-password"]) != 24 {
		t.Fatalf("password %q", a.StringData["postgres-password"])
	}
	if a.StringData["postgres-password"] == b.StringData["postgres-password"] {
		t.Fatal("passwords must be random")
	}
}

func TestNetworkPolicyPorts(t *testing.T) {
	np := NetworkPolicy(full())
	ports := np.Spec.Ingress[0].Ports
	if len(ports) != 2 || ports[0].Port.IntValue() != 5432 || ports[1].Port.IntValue() != 9201 {
		t.Fatalf("ports %+v", ports)
	}
	if np.Spec.PodSelector.MatchLabels["app.kubernetes.io/instance"] != testName {
		t.Fatal("selector")
	}
}

func TestLabels(t *testing.T) {
	labels := Labels(full())
	if labels["app.kubernetes.io/version"] != "26.09.1" || labels["app.kubernetes.io/managed-by"] != ManagedBy {
		t.Fatalf("labels %+v", labels)
	}
	if Host(minimal()) != testName+".team.svc" {
		t.Fatal("host")
	}
}
