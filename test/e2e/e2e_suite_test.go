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

// Package e2e runs the operator against a kind cluster and connects to a real SereneDB pod.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/4thel00z/serenedb-operator/test/utils"
)

const projectImage = "ghcr.io/4thel00z/serenedb-operator:e2e"

// prebuiltImage skips building and loading the operator image when E2E_PREBUILT_IMAGE is set,
// for pipelines that build the image in an earlier job and load it into the cluster themselves.
var prebuiltImage = os.Getenv("E2E_PREBUILT_IMAGE") != ""

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting serenedb-operator e2e suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	if prebuiltImage {
		return
	}
	By("building the operator image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to build the operator image")

	By("loading the operator image into kind")
	Expect(utils.LoadImageToKindClusterWithName(projectImage)).To(Succeed(), "Failed to load the operator image into kind")
})
