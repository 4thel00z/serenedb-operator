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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
	"github.com/4thel00z/serenedb-operator/internal/statements"
)

const clusterName = "srv"

func readyCluster(namespace string) {
	db := newDatabase(namespace, clusterName)
	Expect(k8sClient.Create(ctx, db)).To(Succeed())
	Eventually(func() string { return conditionReason(db, databasev1alpha1.ConditionReady) }, eventuallyTimeout, pollInterval).Should(Equal(reasonPodNotReady))
	markStatefulSetReady(db)
	Eventually(func() string { return conditionReason(db, databasev1alpha1.ConditionReady) }, eventuallyTimeout, pollInterval).Should(Equal(reasonPodReady))
}

func readyReason(obj client.Object, conditions func() []metav1.Condition) func() string {
	return func() string {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			return ""
		}
		c := meta.FindStatusCondition(conditions(), databasev1alpha1.ConditionReady)
		if c == nil {
			return ""
		}
		return c.Reason
	}
}

func must(statement string, err error) string {
	Expect(err).NotTo(HaveOccurred())
	return statement
}

var _ = Describe("Database controller", func() {
	It("waits for the cluster, creates the database once, and drops it on delete when asked", func() {
		ns := newNamespace()
		database := &databasev1alpha1.Database{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns},
			Spec:       databasev1alpha1.DatabaseSpec{Cluster: databasev1alpha1.ClusterRef{Name: clusterName}, ReclaimPolicy: databasev1alpha1.ReclaimDelete},
		}
		Expect(k8sClient.Create(ctx, database)).To(Succeed())
		reason := readyReason(database, func() []metav1.Condition { return database.Status.Conditions })
		Eventually(reason, eventuallyTimeout, pollInterval).Should(Equal(reasonClusterNotFound))

		readyCluster(ns)
		Eventually(reason, eventuallyTimeout, pollInterval).Should(Equal(reasonApplied))
		Expect(fakeSQL.Executed(must(statements.DatabaseExists("app")))).To(BeTrue())
		Expect(fakeSQL.Executed(statements.CreateDatabase("app"))).To(BeTrue())
		Expect(database.Finalizers).To(ContainElement(databasev1alpha1.DependentFinalizer))

		fakeSQL.Answer(must(statements.DatabaseExists("app")), [][]string{{"1"}})
		Expect(k8sClient.Delete(ctx, database)).To(Succeed())
		Eventually(func() bool {
			return fakeSQL.Executed(statements.DropDatabase("app"))
		}, eventuallyTimeout, pollInterval).Should(BeTrue())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(database), &databasev1alpha1.Database{})
		}, eventuallyTimeout, pollInterval).ShouldNot(Succeed())
	})

	It("keeps the database on delete with the default policy", func() {
		ns := newNamespace()
		readyCluster(ns)
		database := &databasev1alpha1.Database{
			ObjectMeta: metav1.ObjectMeta{Name: "kept", Namespace: ns},
			Spec:       databasev1alpha1.DatabaseSpec{Cluster: databasev1alpha1.ClusterRef{Name: clusterName}, Name: "kept_db"},
		}
		Expect(k8sClient.Create(ctx, database)).To(Succeed())
		reason := readyReason(database, func() []metav1.Condition { return database.Status.Conditions })
		Eventually(reason, eventuallyTimeout, pollInterval).Should(Equal(reasonApplied))
		Expect(fakeSQL.Executed(statements.CreateDatabase("kept_db"))).To(BeTrue())

		Expect(k8sClient.Delete(ctx, database)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(database), &databasev1alpha1.Database{})
		}, eventuallyTimeout, pollInterval).ShouldNot(Succeed())
		Expect(fakeSQL.Executed(statements.DropDatabase("kept_db"))).To(BeFalse())
	})
})

var _ = Describe("DatabaseRole controller", func() {
	It("creates the role with its password, converges memberships, and re-applies the password when the Secret changes", func() {
		ns := newNamespace()
		readyCluster(ns)
		pw := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "app-pw", Namespace: ns}, StringData: map[string]string{"password": "first"}}
		Expect(k8sClient.Create(ctx, pw)).To(Succeed())
		fakeSQL.Answer(must(statements.RoleMemberships("app")), [][]string{{"stale"}})

		role := &databasev1alpha1.DatabaseRole{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns},
			Spec: databasev1alpha1.DatabaseRoleSpec{
				Cluster:        databasev1alpha1.ClusterRef{Name: clusterName},
				Login:          true,
				CreateDB:       true,
				PasswordSecret: &databasev1alpha1.PasswordSecretRef{Name: "app-pw"},
				InRoles:        []string{"readers"},
				ReclaimPolicy:  databasev1alpha1.ReclaimDelete,
			},
		}
		Expect(k8sClient.Create(ctx, role)).To(Succeed())
		reason := readyReason(role, func() []metav1.Condition { return role.Status.Conditions })
		Eventually(reason, eventuallyTimeout, pollInterval).Should(Equal(reasonApplied))

		created := must(statements.CreateRole("app", statements.RoleAttributes{Login: true, CreateDB: true, Inherit: true, Password: ptr.To("first")}))
		Expect(fakeSQL.Executed(created)).To(BeTrue())
		Expect(fakeSQL.Executed(statements.GrantMembership("readers", "app"))).To(BeTrue())
		Expect(fakeSQL.Executed(statements.RevokeMembership("stale", "app"))).To(BeTrue())
		Expect(role.Status.PasswordSecretVersion).To(Equal(pw.ResourceVersion))

		By("rotating the password through the Secret")
		fakeSQL.Answer(must(statements.RoleExists("app")), [][]string{{"1"}})
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pw), pw)).To(Succeed())
		pw.StringData = map[string]string{"password": "second"}
		Expect(k8sClient.Update(ctx, pw)).To(Succeed())
		rotated := must(statements.AlterRole("app", statements.RoleAttributes{Login: true, CreateDB: true, Inherit: true, Password: ptr.To("second")}))
		Eventually(func() bool { return fakeSQL.Executed(rotated) }, eventuallyTimeout, pollInterval).Should(BeTrue())

		By("dropping the role on delete")
		Expect(k8sClient.Delete(ctx, role)).To(Succeed())
		Eventually(func() bool { return fakeSQL.Executed(statements.DropRole("app")) }, eventuallyTimeout, pollInterval).Should(BeTrue())
	})

	It("waits for a missing password Secret", func() {
		ns := newNamespace()
		readyCluster(ns)
		role := &databasev1alpha1.DatabaseRole{
			ObjectMeta: metav1.ObjectMeta{Name: "waiting", Namespace: ns},
			Spec: databasev1alpha1.DatabaseRoleSpec{
				Cluster:        databasev1alpha1.ClusterRef{Name: clusterName},
				PasswordSecret: &databasev1alpha1.PasswordSecretRef{Name: "later"},
			},
		}
		Expect(k8sClient.Create(ctx, role)).To(Succeed())
		reason := readyReason(role, func() []metav1.Condition { return role.Status.Conditions })
		Eventually(reason, eventuallyTimeout, pollInterval).Should(Equal(reasonPasswordSecretMissing))
		Expect(fakeSQL.Executed(must(statements.CreateRole("waiting", statements.RoleAttributes{Inherit: true})))).To(BeFalse())
	})
})

var _ = Describe("ServerSecret controller", func() {
	It("merges inline options with the Kubernetes Secret, applies with create-or-replace, and drops on delete", func() {
		ns := newNamespace()
		readyCluster(ns)
		values := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s3-creds", Namespace: ns}, StringData: map[string]string{"KEY_ID": "id", "SECRET": "sec"}}
		Expect(k8sClient.Create(ctx, values)).To(Succeed())

		secret := &databasev1alpha1.ServerSecret{
			ObjectMeta: metav1.ObjectMeta{Name: "lake", Namespace: ns},
			Spec: databasev1alpha1.ServerSecretSpec{
				Cluster:    databasev1alpha1.ClusterRef{Name: clusterName},
				Type:       "s3",
				Scope:      "s3://lake/",
				Options:    map[string]string{"REGION": "eu-central-1"},
				ValuesFrom: &corev1.LocalObjectReference{Name: "s3-creds"},
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		reason := readyReason(secret, func() []metav1.Condition { return secret.Status.Conditions })
		Eventually(reason, eventuallyTimeout, pollInterval).Should(Equal(reasonApplied))
		want := must(statements.CreateSecret("lake", "s3", "s3://lake/", map[string]string{"REGION": "eu-central-1", "KEY_ID": "id", "SECRET": "sec"}))
		Expect(fakeSQL.Executed(want)).To(BeTrue())
		Expect(secret.Status.ValuesSecretVersion).To(Equal(values.ResourceVersion))

		fakeSQL.Answer(must(statements.SecretExists("lake")), [][]string{{"1"}})
		Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		Eventually(func() bool { return fakeSQL.Executed(statements.DropSecret("lake")) }, eventuallyTimeout, pollInterval).Should(BeTrue())
	})

	It("reports a missing values Secret", func() {
		ns := newNamespace()
		readyCluster(ns)
		secret := &databasev1alpha1.ServerSecret{
			ObjectMeta: metav1.ObjectMeta{Name: "nocreds", Namespace: ns},
			Spec: databasev1alpha1.ServerSecretSpec{
				Cluster:    databasev1alpha1.ClusterRef{Name: clusterName},
				Type:       "gcs",
				ValuesFrom: &corev1.LocalObjectReference{Name: "absent"},
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		reason := readyReason(secret, func() []metav1.Condition { return secret.Status.Conditions })
		Eventually(reason, eventuallyTimeout, pollInterval).Should(Equal(reasonValuesSecretMissing))
	})
})
