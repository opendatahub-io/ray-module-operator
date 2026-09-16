//go:build e2e
// +build e2e

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
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opendatahub-io/ray-module-operator/test/utils"
)

var (
	// managerImage is the manager image to be built and loaded for testing.
	managerImage = "example.com/ray-module-operator:v0.0.1"
	// shouldCleanupCertManager tracks whether CertManager was installed by this suite.
	shouldCleanupCertManager = false
)

const (
	installMethodEnv       = "E2E_INSTALL_METHOD"
	installMethodHelm      = "helm"
	installMethodKustomize = "kustomize"
)

func ensureNamespace(name string) error {
	cmd := exec.Command("kubectl", "get", "namespace", name)
	if _, err := utils.Run(cmd); err == nil {
		return nil
	}

	cmd = exec.Command("kubectl", "create", "namespace", name)
	_, err := utils.Run(cmd)
	return err
}

// TestE2E runs the e2e test suite to validate the solution in an isolated environment.
// The default setup installs the module operator with Helm. Set
// E2E_INSTALL_METHOD=kustomize to exercise the Kustomize installation path.
//
// To enable kubectl kuberc (use custom kubectl configurations), set: KUBECTL_KUBERC=true
// By default, kuberc is disabled to ensure consistent test behavior across different environments.
// To skip CertManager installation, set: CERT_MANAGER_INSTALL_SKIP=true
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting ray-module-operator e2e test suite\n")
	RunSpecs(t, "e2e suite")
}

func setupModuleOperator() {
	By("ensuring manager namespace exists")
	ExpectWithOffset(1, ensureNamespace(namespace)).NotTo(HaveOccurred(),
		"Failed to ensure manager namespace exists")

	By("labeling the namespace to enforce the restricted security policy")
	cmd := exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
		"pod-security.kubernetes.io/enforce=restricted")
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

	By("ensuring applications namespace exists")
	ExpectWithOffset(1, ensureNamespace(applicationsNamespace())).NotTo(HaveOccurred(),
		"Failed to ensure applications namespace exists")

	switch moduleOperatorInstallMethod() {
	case installMethodHelm:
		By("installing the standalone module operator with Helm")
		cmd = exec.Command("helm", "upgrade", "--install", "ray-module-operator",
			"./charts/ray-module-operator",
			"--namespace", namespace,
			"--set", "image.repository=example.com/ray-module-operator",
			"--set", "image.tag=v0.0.1",
			"--set", fmt.Sprintf("applicationsNamespace=%s", applicationsNamespace()),
			"--set", "relatedImages.kuberayOperator=quay.io/opendatahub/kuberay-operator:v1.6.2",
			"--set", "scc.enabled=false",
		)
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to install the module operator with Helm")
	case installMethodKustomize:
		By("installing CRDs with Kustomize")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to install CRDs with Kustomize")

		By("deploying the standalone module operator with Kustomize")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to deploy the module operator with Kustomize")
	default:
		Fail(fmt.Sprintf("unsupported %s value %q", installMethodEnv, moduleOperatorInstallMethod()))
	}
}

func teardownModuleOperator() {
	var cmd *exec.Cmd
	switch moduleOperatorInstallMethod() {
	case installMethodHelm:
		By("uninstalling the standalone module operator with Helm")
		cmd = exec.Command("helm", "uninstall", "ray-module-operator", "--namespace", namespace)
		_, _ = utils.Run(cmd)
	case installMethodKustomize:
		By("undeploying the standalone module operator with Kustomize")
		cmd = exec.Command("make", "undeploy", "ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs with Kustomize")
		cmd = exec.Command("make", "uninstall", "ignore-not-found=true")
		_, _ = utils.Run(cmd)
	}

	By("removing manager namespace")
	cmd = exec.Command("kubectl", "delete", "ns", namespace)
	_, _ = utils.Run(cmd)
}

func moduleOperatorInstallMethod() string {
	if method := os.Getenv(installMethodEnv); method != "" {
		return method
	}

	return installMethodHelm
}

var _ = BeforeSuite(func() {
	By("building the manager image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", managerImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager image")

	// TODO(user): If you want to change the e2e test vendor from Kind,
	// ensure the image is built and available, then remove the following block.
	By("loading the manager image on Kind")
	err = utils.LoadImageToKindClusterWithName(managerImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager image into Kind")

	configureKubectlKubeRC()
	setupCertManager()
	setupModuleOperator()
})

var _ = AfterSuite(func() {
	teardownModuleOperator()
	teardownCertManager()
})

// Disable kubectl kuberc by default for test isolation.
// This prevents local kubectl configurations from affecting test behavior.
// To enable kuberc, set: KUBECTL_KUBERC=true
func configureKubectlKubeRC() {
	if os.Getenv("KUBECTL_KUBERC") != "true" {
		By("disabling kubectl kuberc for test isolation")
		err := os.Setenv("KUBECTL_KUBERC", "false")
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to disable kubectl kuberc")
		_, _ = fmt.Fprintf(GinkgoWriter,
			"kubectl kuberc disabled for consistent test behavior (override with KUBECTL_KUBERC=true)\n")
	} else {
		_, _ = fmt.Fprintf(GinkgoWriter, "kubectl kuberc enabled (KUBECTL_KUBERC=true)\n")
	}
}

// setupCertManager installs CertManager if needed for webhook tests.
// Skips installation if CERT_MANAGER_INSTALL_SKIP=true or if already present.
func setupCertManager() {
	if os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true" {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager installation (CERT_MANAGER_INSTALL_SKIP=true)\n")
		return
	}

	By("checking if CertManager is already installed")
	if utils.IsCertManagerCRDsInstalled() {
		_, _ = fmt.Fprintf(GinkgoWriter, "CertManager is already installed. Skipping installation.\n")
		return
	}

	// Mark for cleanup before installation to handle interruptions and partial installs.
	shouldCleanupCertManager = true

	By("installing CertManager")
	Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
}

// teardownCertManager uninstalls CertManager if it was installed by setupCertManager.
// This ensures we only remove what we installed.
func teardownCertManager() {
	if !shouldCleanupCertManager {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager cleanup (not installed by this suite)\n")
		return
	}

	By("uninstalling CertManager")
	utils.UninstallCertManager()
}
