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
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
)

// StatefulSet builds the single-replica StatefulSet running serened.
func StatefulSet(db *databasev1alpha1.SereneDB, configHash string) *appsv1.StatefulSet {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name(db),
			Namespace: db.Namespace,
			Labels:    Labels(db),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:       ptr.To[int32](1),
			ServiceName:    HeadlessServiceName(db),
			Selector:       &metav1.LabelSelector{MatchLabels: SelectorLabels(db)},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType},
			Template:       PodTemplate(db, configHash),
		},
	}
	if db.Spec.Persistence.ExistingClaim != "" {
		return sts
	}
	sts.Spec.PersistentVolumeClaimRetentionPolicy = RetentionPolicy(db)
	sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{VolumeClaimTemplate(db)}
	return sts
}

// RetentionPolicy translates spec.persistence.retentionPolicy into the StatefulSet field.
func RetentionPolicy(db *databasev1alpha1.SereneDB) *appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy {
	policy := db.Spec.Persistence.RetentionPolicy
	return &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenDeleted: retentionType(policy.WhenDeleted),
		WhenScaled:  retentionType(policy.WhenScaled),
	}
}

// VolumeClaimTemplate builds the data volume claim.
func VolumeClaimTemplate(db *databasev1alpha1.SereneDB) corev1.PersistentVolumeClaim {
	p := db.Spec.Persistence
	size := resource.MustParse("20Gi")
	if p.Size != nil {
		size = *p.Size
	}
	modes := p.AccessModes
	if len(modes) == 0 {
		modes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}
	claim := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: dataVolume},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      modes,
			StorageClassName: p.StorageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: size},
			},
		},
	}
	if db.Spec.Bootstrap == nil {
		return claim
	}
	claim.Spec.DataSource = &corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(VolumeSnapshotGVK.Group),
		Kind:     VolumeSnapshotGVK.Kind,
		Name:     db.Spec.Bootstrap.VolumeSnapshotName,
	}
	return claim
}

// PodTemplate builds the database pod, hardened the same way as the upstream Helm chart.
func PodTemplate(db *databasev1alpha1.SereneDB, configHash string) corev1.PodTemplateSpec {
	labels := SelectorLabels(db)
	maps.Copy(labels, db.Spec.PodLabels)
	annotations := map[string]string{ConfigHashAnnotation: configHash}
	maps.Copy(annotations, db.Spec.PodAnnotations)

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: annotations},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: ptr.To(terminationGracePeriod(db)),
			AutomountServiceAccountToken:  ptr.To(false),
			ImagePullSecrets:              db.Spec.ImagePullSecrets,
			SecurityContext:               &corev1.PodSecurityContext{FSGroup: ptr.To[int64](0)},
			PriorityClassName:             db.Spec.PriorityClassName,
			Containers:                    []corev1.Container{Container(db)},
			Volumes:                       Volumes(db),
			NodeSelector:                  db.Spec.NodeSelector,
			Tolerations:                   db.Spec.Tolerations,
			Affinity:                      db.Spec.Affinity,
		},
	}
}

// Container builds the serened container.
func Container(db *databasev1alpha1.SereneDB) corev1.Container {
	probe := pgIsReady(db)
	c := corev1.Container{
		Name:            containerName,
		Image:           Image(db),
		ImagePullPolicy: pullPolicy(db),
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:                ptr.To(databasev1alpha1.ImageUID),
			RunAsNonRoot:             ptr.To(true),
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Args: []string{containerName, databasev1alpha1.DefaultDataMountPath},
		Env: append([]corev1.EnvVar{{
			Name: "POSTGRES_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: SecretName(db)},
				Key:                  PasswordKey(db),
			}},
		}}, db.Spec.Env...),
		Ports: ContainerPorts(db),
		StartupProbe: &corev1.Probe{
			ProbeHandler:     probe,
			PeriodSeconds:    5,
			FailureThreshold: 60,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler:        probe,
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
			TimeoutSeconds:      3,
			FailureThreshold:    3,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler:        probe,
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    6,
		},
		Resources:    db.Spec.Resources,
		VolumeMounts: VolumeMounts(db),
	}
	return c
}

// ContainerPorts lists the container ports, pg-wire first and HTTP when enabled.
func ContainerPorts(db *databasev1alpha1.SereneDB) []corev1.ContainerPort {
	ports := []corev1.ContainerPort{{Name: postgresPort, ContainerPort: PostgresPort(db), Protocol: corev1.ProtocolTCP}}
	if !db.Spec.Listeners.HTTP.Enabled {
		return ports
	}
	return append(ports, corev1.ContainerPort{Name: httpPort, ContainerPort: HTTPPort(db), Protocol: corev1.ProtocolTCP})
}

// VolumeMounts mounts the data directory, the flagfile and optionally the TLS material.
func VolumeMounts(db *databasev1alpha1.SereneDB) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{Name: dataVolume, MountPath: databasev1alpha1.DefaultDataMountPath},
		{Name: configVolume, MountPath: configMountPath, ReadOnly: true},
	}
	if !db.Spec.TLS.Enabled {
		return mounts
	}
	return append(mounts, corev1.VolumeMount{Name: tlsVolume, MountPath: tlsMountPath, ReadOnly: true})
}

// Volumes lists the pod volumes. The data volume comes from the claim template unless an existing claim is named.
func Volumes(db *databasev1alpha1.SereneDB) []corev1.Volume {
	volumes := []corev1.Volume{{
		Name: configVolume,
		VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: Name(db)},
		}},
	}}
	if db.Spec.TLS.Enabled {
		volumes = append(volumes, corev1.Volume{
			Name:         tlsVolume,
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: db.Spec.TLS.SecretName}},
		})
	}
	if db.Spec.Persistence.ExistingClaim != "" {
		volumes = append(volumes, corev1.Volume{
			Name: dataVolume,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: db.Spec.Persistence.ExistingClaim,
			}},
		})
	}
	return volumes
}

func pgIsReady(db *databasev1alpha1.SereneDB) corev1.ProbeHandler {
	return corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{
		"pg_isready", "-h", "127.0.0.1", "-p", fmt.Sprint(PostgresPort(db)), "-U", "postgres",
	}}}
}

func pullPolicy(db *databasev1alpha1.SereneDB) corev1.PullPolicy {
	if db.Spec.Image.PullPolicy != "" {
		return db.Spec.Image.PullPolicy
	}
	return corev1.PullIfNotPresent
}

func terminationGracePeriod(db *databasev1alpha1.SereneDB) int64 {
	if db.Spec.TerminationGracePeriodSeconds != nil {
		return *db.Spec.TerminationGracePeriodSeconds
	}
	return databasev1alpha1.DefaultTerminationGracePeriodSeconds
}

func retentionType(value string) appsv1.PersistentVolumeClaimRetentionPolicyType {
	if value == "Delete" {
		return appsv1.DeletePersistentVolumeClaimRetentionPolicyType
	}
	return appsv1.RetainPersistentVolumeClaimRetentionPolicyType
}
