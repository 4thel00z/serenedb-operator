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

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/4thel00z/serenedb-operator/test/utils"
)

const (
	operatorNamespace = "serenedb-operator-system"
	databaseNamespace = "serenedb-e2e"
	databaseName      = "e2e"
	databaseImage     = "serenedb/serenedb:26.09.2"
)

func kubectl(args ...string) (string, error) {
	return utils.Run(exec.Command("kubectl", args...))
}

func databaseManifest() string {
	return fmt.Sprintf(`apiVersion: database.serenedb.com/v1alpha1
kind: SereneDB
metadata:
  name: %s
  namespace: %s
spec:
  persistence:
    size: 1Gi
    retentionPolicy:
      whenDeleted: Delete
  config:
    cpuThreads: 1
    ioThreads: 1
    backgroundThreads: 1
`, databaseName, databaseNamespace)
}

func applyManifest(manifest string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func queryPodManifest(name, sql string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
spec:
  restartPolicy: Never
  automountServiceAccountToken: false
  containers:
    - name: psql
      image: %s
      env:
        - name: PGPASSWORD
          valueFrom:
            secretKeyRef:
              name: %s
              key: postgres-password
      command: ["psql", "-h", "%s", "-p", "7890", "-U", "postgres", "-d", "postgres", "-tAc", %q]
`, name, databaseNamespace, databaseImage, databaseName, databaseName, sql)
}

func runQuery(name, sql string) string {
	applyManifest(queryPodManifest(name, sql))
	Eventually(func() string {
		phase, _ := kubectl("get", "pod", name, "-n", databaseNamespace, "-o", "jsonpath={.status.phase}")
		return phase
	}, 3*time.Minute, 2*time.Second).Should(Equal("Succeeded"))
	out, err := kubectl("logs", name, "-n", databaseNamespace)
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(out)
}

var _ = Describe("SereneDB operator", Ordered, func() {
	BeforeAll(func() {
		By("installing the CRDs")
		_, err := utils.Run(exec.Command("make", "install"))
		Expect(err).NotTo(HaveOccurred())

		By("deploying the operator")
		_, err = utils.Run(exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage)))
		Expect(err).NotTo(HaveOccurred())

		By("creating the database namespace")
		_, err = kubectl("create", "ns", databaseNamespace)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		_, _ = kubectl("delete", "ns", databaseNamespace, "--wait=false")
		_, _ = utils.Run(exec.Command("make", "undeploy", "ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("make", "uninstall", "ignore-not-found=true"))
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		logs, _ := kubectl("logs", "-n", operatorNamespace, "-l", "control-plane=controller-manager", "--tail=200")
		_, _ = fmt.Fprintf(GinkgoWriter, "operator logs:\n%s\n", logs)
		describe, _ := kubectl("describe", "serenedb", databaseName, "-n", databaseNamespace)
		_, _ = fmt.Fprintf(GinkgoWriter, "serenedb:\n%s\n", describe)
		pods, _ := kubectl("get", "pods", "-n", databaseNamespace, "-o", "wide")
		_, _ = fmt.Fprintf(GinkgoWriter, "pods:\n%s\n", pods)
		events, _ := kubectl("get", "events", "-n", databaseNamespace, "--sort-by=.lastTimestamp")
		_, _ = fmt.Fprintf(GinkgoWriter, "events:\n%s\n", events)
	})

	It("runs the operator pod", func() {
		Eventually(func() string {
			out, _ := kubectl("get", "pods", "-n", operatorNamespace, "-l", "control-plane=controller-manager",
				"-o", "jsonpath={.items[0].status.phase}")
			return out
		}, 2*time.Minute, 2*time.Second).Should(Equal("Running"))
	})

	It("brings a SereneDB to Ready", func() {
		applyManifest(databaseManifest())
		Eventually(func() string {
			out, _ := kubectl("get", "serenedb", databaseName, "-n", databaseNamespace,
				"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
			return out
		}, 6*time.Minute, 5*time.Second).Should(Equal("True"))

		version, err := kubectl("get", "serenedb", databaseName, "-n", databaseNamespace, "-o", "jsonpath={.status.version}")
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal("26.09.2"))
	})

	It("accepts authenticated SQL connections through the Service", func() {
		Expect(runQuery("query-select", "SELECT 1")).To(Equal("1"))
	})

	It("rejects the wrong password", func() {
		manifest := strings.Replace(queryPodManifest("query-badpw", "SELECT 1"),
			"valueFrom:\n            secretKeyRef:\n              name: e2e\n              key: postgres-password",
			"value: not-the-password", 1)
		applyManifest(manifest)
		Eventually(func() string {
			phase, _ := kubectl("get", "pod", "query-badpw", "-n", databaseNamespace, "-o", "jsonpath={.status.phase}")
			return phase
		}, 3*time.Minute, 2*time.Second).Should(Equal("Failed"))
	})

	It("rolls the pod and keeps data when the config changes", func() {
		created := runQuery("query-create", "CREATE TABLE e2e_marker(v INT); INSERT INTO e2e_marker VALUES (42)")
		Expect(created).To(ContainSubstring("INSERT 0 1"))

		before, err := kubectl("get", "pod", databaseName+"-0", "-n", databaseNamespace, "-o", "jsonpath={.metadata.uid}")
		Expect(err).NotTo(HaveOccurred())

		_, err = kubectl("patch", "serenedb", databaseName, "-n", databaseNamespace, "--type=merge",
			"-p", `{"spec":{"config":{"logLevel":"debug"}}}`)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() string {
			uid, _ := kubectl("get", "pod", databaseName+"-0", "-n", databaseNamespace, "-o", "jsonpath={.metadata.uid}")
			return uid
		}, 4*time.Minute, 5*time.Second).ShouldNot(Or(Equal(before), BeEmpty()))
		Eventually(func() string {
			out, _ := kubectl("get", "serenedb", databaseName, "-n", databaseNamespace,
				"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
			return out
		}, 6*time.Minute, 5*time.Second).Should(Equal("True"))

		Expect(runQuery("query-after-roll", "SELECT v FROM e2e_marker")).To(Equal("42"))
	})

	It("garbage collects owned objects and the data volume when the SereneDB is deleted", func() {
		_, err := kubectl("delete", "serenedb", databaseName, "-n", databaseNamespace, "--wait=true")
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() string {
			out, _ := kubectl("get", "statefulset,service,configmap,secret,pvc", "-n", databaseNamespace,
				"-l", "app.kubernetes.io/instance="+databaseName, "-o", "name")
			return strings.TrimSpace(out)
		}, 3*time.Minute, 3*time.Second).Should(BeEmpty())
	})
})
