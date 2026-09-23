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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
)

// Service builds the client-facing Service.
func Service(db *databasev1alpha1.SereneDB) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        Name(db),
			Namespace:   db.Namespace,
			Labels:      Labels(db),
			Annotations: db.Spec.Service.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     serviceType(db),
			Ports:    ServicePorts(db),
			Selector: SelectorLabels(db),
		},
	}
	if svc.Spec.Type == corev1.ServiceTypeNodePort && db.Spec.Service.NodePort != 0 {
		svc.Spec.Ports[0].NodePort = db.Spec.Service.NodePort
	}
	return svc
}

// HeadlessService builds the StatefulSet governing Service with stable per-pod DNS.
func HeadlessService(db *databasev1alpha1.SereneDB) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      HeadlessServiceName(db),
			Namespace: db.Namespace,
			Labels:    Labels(db),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Ports:                    ServicePorts(db)[:1],
			Selector:                 SelectorLabels(db),
		},
	}
}

// ServicePorts lists the serving ports, pg-wire first and HTTP when enabled.
func ServicePorts(db *databasev1alpha1.SereneDB) []corev1.ServicePort {
	ports := []corev1.ServicePort{{
		Name:       postgresPort,
		Port:       PostgresPort(db),
		TargetPort: intstr.FromString(postgresPort),
		Protocol:   corev1.ProtocolTCP,
	}}
	if !db.Spec.Listeners.HTTP.Enabled {
		return ports
	}
	return append(ports, corev1.ServicePort{
		Name:       httpPort,
		Port:       HTTPPort(db),
		TargetPort: intstr.FromString(httpPort),
		Protocol:   corev1.ProtocolTCP,
	})
}

func serviceType(db *databasev1alpha1.SereneDB) corev1.ServiceType {
	if db.Spec.Service.Type != "" {
		return db.Spec.Service.Type
	}
	return corev1.ServiceTypeClusterIP
}
