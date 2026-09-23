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

package controller

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
	"github.com/4thel00z/serenedb-operator/internal/resources"
)

const (
	eventuallyTimeout = 20 * time.Second
	pollInterval      = 200 * time.Millisecond
)

var namespaceCounter int

func newNamespace() string {
	namespaceCounter++
	name := fmt.Sprintf("sdb-test-%d", namespaceCounter)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	return name
}

func newDatabase(namespace, name string) *databasev1alpha1.SereneDB {
	return &databasev1alpha1.SereneDB{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

func key(db *databasev1alpha1.SereneDB, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: db.Namespace, Name: name}
}

func fetch(db *databasev1alpha1.SereneDB) *databasev1alpha1.SereneDB {
	current := &databasev1alpha1.SereneDB{}
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(db), current)).To(Succeed())
	return current
}

func condition(db *databasev1alpha1.SereneDB, conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(fetch(db).Status.Conditions, conditionType)
}

func conditionReason(db *databasev1alpha1.SereneDB, conditionType string) string {
	c := condition(db, conditionType)
	if c == nil {
		return ""
	}
	return c.Reason
}

func updateSpec(db *databasev1alpha1.SereneDB, mutate func(*databasev1alpha1.SereneDB)) {
	Eventually(func() error {
		current := fetch(db)
		mutate(current)
		return k8sClient.Update(ctx, current)
	}, eventuallyTimeout, pollInterval).Should(Succeed())
}

func markStatefulSetReady(db *databasev1alpha1.SereneDB) {
	Eventually(func() error {
		sts := &appsv1.StatefulSet{}
		if err := k8sClient.Get(ctx, key(db, db.Name), sts); err != nil {
			return err
		}
		sts.Status = appsv1.StatefulSetStatus{
			ObservedGeneration: sts.Generation,
			Replicas:           1,
			ReadyReplicas:      1,
			CurrentReplicas:    1,
			UpdatedReplicas:    1,
			AvailableReplicas:  1,
			CurrentRevision:    fmt.Sprintf("rev-%d", sts.Generation),
			UpdateRevision:     fmt.Sprintf("rev-%d", sts.Generation),
		}
		return k8sClient.Status().Update(ctx, sts)
	}, eventuallyTimeout, pollInterval).Should(Succeed())
}

var _ = Describe("SereneDB controller", func() {
	It("creates every owned object for a minimal spec and applies CRD defaults", func() {
		db := newDatabase(newNamespace(), "mydb")
		Expect(k8sClient.Create(ctx, db)).To(Succeed())

		By("applying CRD defaults")
		defaulted := fetch(db)
		Expect(defaulted.Spec.Image.Tag).To(Equal(databasev1alpha1.DefaultImageTag))
		Expect(defaulted.Spec.Listeners.Postgres.Port).To(Equal(databasev1alpha1.DefaultPostgresPort))
		Expect(defaulted.Spec.Persistence.Size.Cmp(resource.MustParse("20Gi"))).To(BeZero())
		Expect(defaulted.Spec.Persistence.RetentionPolicy.WhenDeleted).To(Equal("Retain"))
		Expect(*defaulted.Spec.TerminationGracePeriodSeconds).To(Equal(int64(120)))

		By("creating the StatefulSet")
		sts := &appsv1.StatefulSet{}
		Eventually(func() error { return k8sClient.Get(ctx, key(db, "mydb"), sts) }, eventuallyTimeout, pollInterval).Should(Succeed())
		Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		Expect(sts.Spec.ServiceName).To(Equal("mydb-hl"))
		Expect(sts.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"serened", "/var/lib/serenedb"}))
		Expect(sts.Spec.Template.Annotations).To(HaveKey(resources.ConfigHashAnnotation))
		Expect(metav1.IsControlledBy(sts, defaulted)).To(BeTrue())

		By("creating the ConfigMap with the flagfile")
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key(db, "mydb"), cm)).To(Succeed())
		Expect(cm.Data["serened.conf"]).To(ContainSubstring("--listen=postgres://0.0.0.0:7890\n"))
		Expect(cm.Data["serened.conf"]).To(ContainSubstring("--log_level=info\n"))

		By("creating both Services")
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, key(db, "mydb"), svc)).To(Succeed())
		Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
		Expect(svc.Spec.ClusterIP).NotTo(BeEmpty())
		headless := &corev1.Service{}
		Expect(k8sClient.Get(ctx, key(db, "mydb-hl"), headless)).To(Succeed())
		Expect(headless.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))

		By("generating a password Secret that outlives the SereneDB while the volume is retained")
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, key(db, "mydb"), secret)).To(Succeed())
		Expect(secret.Data["postgres-password"]).To(HaveLen(24))
		Expect(metav1.IsControlledBy(secret, defaulted)).To(BeFalse())

		By("not creating a NetworkPolicy by default")
		Expect(k8sClient.Get(ctx, key(db, "mydb"), &networkingv1.NetworkPolicy{})).NotTo(Succeed())

		By("reporting a not-ready status with connection details")
		Eventually(func() string { return conditionReason(db, databasev1alpha1.ConditionReady) }, eventuallyTimeout, pollInterval).Should(Equal(reasonPodNotReady))
		status := fetch(db).Status
		Expect(status.Host).To(Equal("mydb." + db.Namespace + ".svc"))
		Expect(status.Port).To(Equal(int32(7890)))
		Expect(status.SecretName).To(Equal("mydb"))
		Expect(status.Version).To(BeEmpty())
		Expect(status.ObservedGeneration).To(Equal(fetch(db).Generation))
		Expect(conditionReason(db, databasev1alpha1.ConditionProgressing)).To(Equal(reasonRollingOut))

		By("becoming Ready once the StatefulSet reports a complete rollout")
		markStatefulSetReady(db)
		Eventually(func() string { return conditionReason(db, databasev1alpha1.ConditionReady) }, eventuallyTimeout, pollInterval).Should(Equal(reasonPodReady))
		Expect(fetch(db).Status.ReadyReplicas).To(Equal(int32(1)))
		Expect(fetch(db).Status.Version).To(Equal(databasev1alpha1.DefaultImageTag))
		Expect(conditionReason(db, databasev1alpha1.ConditionProgressing)).To(Equal(reasonStable))
	})

	It("rolls the pod template when the flagfile changes and keeps the password stable", func() {
		db := newDatabase(newNamespace(), "cfg")
		Expect(k8sClient.Create(ctx, db)).To(Succeed())
		sts := &appsv1.StatefulSet{}
		Eventually(func() error { return k8sClient.Get(ctx, key(db, "cfg"), sts) }, eventuallyTimeout, pollInterval).Should(Succeed())
		before := sts.Spec.Template.Annotations[resources.ConfigHashAnnotation]
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, key(db, "cfg"), secret)).To(Succeed())
		password := string(secret.Data["postgres-password"])

		updateSpec(db, func(d *databasev1alpha1.SereneDB) { d.Spec.Config.LogLevel = "debug" })

		Eventually(func() string {
			Expect(k8sClient.Get(ctx, key(db, "cfg"), sts)).To(Succeed())
			return sts.Spec.Template.Annotations[resources.ConfigHashAnnotation]
		}, eventuallyTimeout, pollInterval).ShouldNot(Equal(before))
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key(db, "cfg"), cm)).To(Succeed())
		Expect(cm.Data["serened.conf"]).To(ContainSubstring("--log_level=debug\n"))

		Expect(k8sClient.Get(ctx, key(db, "cfg"), secret)).To(Succeed())
		Expect(string(secret.Data["postgres-password"])).To(Equal(password))
	})

	It("ties the generated Secret to the SereneDB only while the volume is deleted with it", func() {
		db := newDatabase(newNamespace(), "own")
		db.Spec.Persistence.RetentionPolicy.WhenDeleted = "Delete"
		Expect(k8sClient.Create(ctx, db)).To(Succeed())

		secret := &corev1.Secret{}
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, key(db, "own"), secret); err != nil {
				return false
			}
			return metav1.IsControlledBy(secret, fetch(db))
		}, eventuallyTimeout, pollInterval).Should(BeTrue())

		updateSpec(db, func(d *databasev1alpha1.SereneDB) { d.Spec.Persistence.RetentionPolicy.WhenDeleted = "Retain" })
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx, key(db, "own"), secret)).To(Succeed())
			return metav1.IsControlledBy(secret, fetch(db))
		}, eventuallyTimeout, pollInterval).Should(BeFalse())
	})

	It("waits for a user-provided Secret and its key before creating the StatefulSet", func() {
		db := newDatabase(newNamespace(), "ext")
		db.Spec.Auth.ExistingSecret = "creds"
		Expect(k8sClient.Create(ctx, db)).To(Succeed())

		Eventually(func() string { return conditionReason(db, databasev1alpha1.ConditionReady) }, eventuallyTimeout, pollInterval).Should(Equal(reasonSecretMissing))
		Expect(k8sClient.Get(ctx, key(db, "ext"), &appsv1.StatefulSet{})).NotTo(Succeed())

		creds := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: db.Namespace}, StringData: map[string]string{"other": "x"}}
		Expect(k8sClient.Create(ctx, creds)).To(Succeed())
		updateSpec(db, func(d *databasev1alpha1.SereneDB) { d.Spec.Config.LogLevel = "warning" })
		Eventually(func() string { return conditionReason(db, databasev1alpha1.ConditionReady) }, eventuallyTimeout, pollInterval).Should(Equal(reasonSecretKeyMissing))

		Expect(k8sClient.Get(ctx, key(db, "creds"), creds)).To(Succeed())
		creds.StringData = map[string]string{"postgres-password": "hunter22"}
		Expect(k8sClient.Update(ctx, creds)).To(Succeed())
		updateSpec(db, func(d *databasev1alpha1.SereneDB) { d.Spec.Config.LogLevel = "error" })

		sts := &appsv1.StatefulSet{}
		Eventually(func() error { return k8sClient.Get(ctx, key(db, "ext"), sts) }, eventuallyTimeout, pollInterval).Should(Succeed())
		Expect(sts.Spec.Template.Spec.Containers[0].Env[0].ValueFrom.SecretKeyRef.Name).To(Equal("creds"))
		Expect(k8sClient.Get(ctx, key(db, "ext"), &corev1.Secret{})).NotTo(Succeed())
	})

	It("requires a TLS secret name and its Secret when TLS is enabled", func() {
		db := newDatabase(newNamespace(), "tls")
		db.Spec.TLS.Enabled = true
		Expect(k8sClient.Create(ctx, db)).To(Succeed())
		Eventually(func() string { return conditionReason(db, databasev1alpha1.ConditionReady) }, eventuallyTimeout, pollInterval).Should(Equal(reasonTLSSecretRequired))

		updateSpec(db, func(d *databasev1alpha1.SereneDB) { d.Spec.TLS.SecretName = "tls-material" })
		Eventually(func() string { return conditionReason(db, databasev1alpha1.ConditionReady) }, eventuallyTimeout, pollInterval).Should(Equal(reasonTLSSecretMissing))

		tlsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "tls-material", Namespace: db.Namespace},
			Type:       corev1.SecretTypeTLS,
			StringData: map[string]string{"tls.crt": "c", "tls.key": "k"},
		}
		Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())
		updateSpec(db, func(d *databasev1alpha1.SereneDB) { d.Spec.Listeners.HTTP.Enabled = true })

		sts := &appsv1.StatefulSet{}
		Eventually(func() error { return k8sClient.Get(ctx, key(db, "tls"), sts) }, eventuallyTimeout, pollInterval).Should(Succeed())
		Expect(sts.Spec.Template.Spec.Volumes).To(ContainElement(HaveField("Secret.SecretName", "tls-material")))
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key(db, "tls"), cm)).To(Succeed())
		Expect(cm.Data["serened.conf"]).To(ContainSubstring("https://0.0.0.0:9200?api=es"))
		Expect(cm.Data["serened.conf"]).To(ContainSubstring("--tls_cert=/etc/serenedb-tls/tls.crt"))
	})

	It("creates and removes the NetworkPolicy with the toggle", func() {
		db := newDatabase(newNamespace(), "np")
		db.Spec.NetworkPolicy.Enabled = true
		Expect(k8sClient.Create(ctx, db)).To(Succeed())

		np := &networkingv1.NetworkPolicy{}
		Eventually(func() error { return k8sClient.Get(ctx, key(db, "np"), np) }, eventuallyTimeout, pollInterval).Should(Succeed())
		Expect(np.Spec.Ingress[0].Ports[0].Port.IntValue()).To(Equal(7890))

		updateSpec(db, func(d *databasev1alpha1.SereneDB) { d.Spec.NetworkPolicy.Enabled = false })
		Eventually(func() error { return k8sClient.Get(ctx, key(db, "np"), np) }, eventuallyTimeout, pollInterval).ShouldNot(Succeed())

		By("leaving a same-named NetworkPolicy alone when the operator does not own it")
		foreign := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "np", Namespace: db.Namespace},
			Spec:       networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{}},
		}
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())
		updateSpec(db, func(d *databasev1alpha1.SereneDB) { d.Spec.Config.LogLevel = "debug" })
		Eventually(func() string {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, key(db, "np"), cm)).To(Succeed())
			return cm.Data["serened.conf"]
		}, eventuallyTimeout, pollInterval).Should(ContainSubstring("--log_level=debug"))
		Expect(k8sClient.Get(ctx, key(db, "np"), np)).To(Succeed())
		Expect(np.OwnerReferences).To(BeEmpty())
	})

	It("keeps a NodePort assigned by the API server across reconciles", func() {
		db := newDatabase(newNamespace(), "nodeport")
		db.Spec.Service.Type = corev1.ServiceTypeNodePort
		Expect(k8sClient.Create(ctx, db)).To(Succeed())

		svc := &corev1.Service{}
		Eventually(func() int32 {
			if err := k8sClient.Get(ctx, key(db, "nodeport"), svc); err != nil {
				return 0
			}
			return svc.Spec.Ports[0].NodePort
		}, eventuallyTimeout, pollInterval).ShouldNot(BeZero())
		assigned := svc.Spec.Ports[0].NodePort

		updateSpec(db, func(d *databasev1alpha1.SereneDB) { d.Spec.Service.Annotations = map[string]string{"touched": "yes"} })
		Eventually(func() string {
			Expect(k8sClient.Get(ctx, key(db, "nodeport"), svc)).To(Succeed())
			return svc.Annotations["touched"]
		}, eventuallyTimeout, pollInterval).Should(Equal("yes"))
		Expect(svc.Spec.Ports[0].NodePort).To(Equal(assigned))
	})

	It("rejects changes to the immutable volume claim fields", func() {
		db := newDatabase(newNamespace(), "immutable")
		Expect(k8sClient.Create(ctx, db)).To(Succeed())

		current := fetch(db)
		current.Spec.Persistence.Size = ptr.To(resource.MustParse("50Gi"))
		err := k8sClient.Update(ctx, current)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("persistence.size is immutable"))

		current = fetch(db)
		current.Spec.Persistence.ExistingClaim = "elsewhere"
		err = k8sClient.Update(ctx, current)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("persistence.existingClaim is immutable"))
	})
})
