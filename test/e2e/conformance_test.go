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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opendatahub-io/ray-module-operator/test/utils"
)

func applicationsNamespace() string {
	if ns := os.Getenv("APPLICATIONS_NAMESPACE"); ns != "" {
		return ns
	}
	return "opendatahub"
}

func kubectlGetJsonpath(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	return utils.Run(cmd)
}

var _ = Describe("Module CR Validation", func() {
	It("should reject a Ray CR with non-default name", func() {
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(`
apiVersion: components.platform.opendatahub.io/v1alpha1
kind: Ray
metadata:
  name: bad-name
spec:
  managementState: Managed
`)
		output, err := cmd.CombinedOutput()
		Expect(err).To(HaveOccurred(), "creating a Ray CR with non-default name should fail")
		Expect(string(output)).To(ContainSubstring("Ray name must be default-ray"))
	})
})

var _ = Describe("Module Contract Conformance", Ordered, func() {
	ns := applicationsNamespace()

	BeforeAll(func() {
		By("creating the Ray module CR explicitly")
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(fmt.Sprintf(`
apiVersion: components.platform.opendatahub.io/v1alpha1
kind: Ray
metadata:
  name: default-ray
spec:
  managementState: Managed
  applicationsNamespace: %s
`, ns))
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "failed to create Ray CR")

		By("waiting for the Ray module CR to be Ready")
		Eventually(func(g Gomega) {
			out, err := kubectlGetJsonpath("get", "ray", "default-ray",
				"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
			g.Expect(err).NotTo(HaveOccurred(), "failed to get Ray CR")

			failureMessage := "Ray CR not Ready"
			if strings.TrimSpace(out) != "True" {
				conditions, conditionErr := kubectlGetJsonpath("get", "ray", "default-ray",
					"-o", "jsonpath={range .status.conditions[*]}{.type}={.status} reason={.reason} message={.message}{\"\\n\"}{end}")
				if conditionErr == nil {
					failureMessage = fmt.Sprintf("Ray CR not Ready; conditions:\n%s", strings.TrimSpace(conditions))
				}
			}

			g.Expect(strings.TrimSpace(out)).To(Equal("True"), failureMessage)
		}, 5*time.Minute, 5*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		By("deleting the Ray module CR")
		cmd := exec.Command("kubectl", "delete", "ray", "default-ray", "--ignore-not-found", "--timeout=120s")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "failed to delete Ray CR")

		By("waiting for the Ray module CR finalizer to complete")
		Eventually(func(g Gomega) {
			out, err := kubectlGetJsonpath("get", "ray", "default-ray", "--ignore-not-found", "-o", "name")
			g.Expect(err).NotTo(HaveOccurred(), "failed to check Ray CR deletion")
			g.Expect(strings.TrimSpace(out)).To(BeEmpty(), "Ray CR should be deleted")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying module operands were removed")
		Eventually(func(g Gomega) {
			out, err := kubectlGetJsonpath("get", "deployment", "kuberay-operator", "-n", ns,
				"--ignore-not-found", "-o", "name")
			g.Expect(err).NotTo(HaveOccurred(), "failed to check KubeRay deployment cleanup")
			g.Expect(strings.TrimSpace(out)).To(BeEmpty(), "KubeRay deployment should be removed")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			out, err := kubectlGetJsonpath("get", "mutatingwebhookconfigurations",
				"-l", "platform.opendatahub.io/part-of=ray", "-o", "name")
			g.Expect(err).NotTo(HaveOccurred(), "failed to check webhook cleanup")
			g.Expect(strings.TrimSpace(out)).To(BeEmpty(), "Ray webhooks should be removed")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
	})

	Context("Module CR", func() {
		It("should accept the singleton instance name", func() {
			cmd := exec.Command("kubectl", "get", "ray", "default-ray", "-o", "name")
			out, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(out)).To(Equal("ray.components.platform.opendatahub.io/default-ray"))
		})
	})

	Context("Status Contract", func() {
		It("should populate observedGeneration matching generation", func() {
			Eventually(func(g Gomega) {
				genOut, err := kubectlGetJsonpath("get", "ray", "default-ray",
					"-o", "jsonpath={.metadata.generation}")
				g.Expect(err).NotTo(HaveOccurred())

				obsOut, err := kubectlGetJsonpath("get", "ray", "default-ray",
					"-o", "jsonpath={.status.observedGeneration}")
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(strings.TrimSpace(obsOut)).To(Equal(strings.TrimSpace(genOut)),
					"observedGeneration should match generation")
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})

		It("should have Ready condition", func() {
			out, err := kubectlGetJsonpath("get", "ray", "default-ray",
				"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(out)).To(Equal("True"))
		})

		It("should have Degraded condition set to False", func() {
			out, err := kubectlGetJsonpath("get", "ray", "default-ray",
				"-o", "jsonpath={.status.conditions[?(@.type==\"Degraded\")].status}")
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(out)).To(Equal("False"))

			reason, err := kubectlGetJsonpath("get", "ray", "default-ray",
				"-o", "jsonpath={.status.conditions[?(@.type==\"Degraded\")].reason}")
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(reason)).To(Equal("AsExpected"))
		})

		It("should have ProvisioningSucceeded condition", func() {
			out, err := kubectlGetJsonpath("get", "ray", "default-ray",
				"-o", "jsonpath={.status.conditions[?(@.type==\"ProvisioningSucceeded\")].status}")
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(out)).NotTo(BeEmpty(), "ProvisioningSucceeded condition should exist")
		})

		It("should populate status.releases", func() {
			out, err := kubectlGetJsonpath("get", "ray", "default-ray",
				"-o", "jsonpath={.status.releases}")
			Expect(err).NotTo(HaveOccurred())
			raw := strings.TrimSpace(out)
			Expect(raw).NotTo(BeEmpty(), "status.releases should not be empty")

			type release struct {
				Name    string `json:"name"`
				Version string `json:"version"`
				RepoURL string `json:"repoURL"`
			}
			var releases []release
			Expect(json.Unmarshal([]byte(raw), &releases)).To(Succeed())
			Expect(releases).NotTo(BeEmpty(), "should have at least one release entry")

			found := false
			for _, r := range releases {
				if r.Name == "KubeRay" {
					found = true
					Expect(r.Version).NotTo(BeEmpty(), "KubeRay release version should not be empty")
					Expect(r.RepoURL).To(Equal("https://github.com/opendatahub-io/kuberay"))
					break
				}
			}
			Expect(found).To(BeTrue(), "should have a release entry with name=KubeRay")
		})
	})

	Context("Finalizer", func() {
		It("should have the platform finalizer on the Ray CR", func() {
			out, err := kubectlGetJsonpath("get", "ray", "default-ray",
				"-o", "jsonpath={.metadata.finalizers}")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("platform.opendatahub.io/finalizer"))
		})
	})

	Context("Operand Health", func() {
		It("should have kuberay-operator deployment Available", func() {
			cmd := exec.Command("kubectl", "wait", "--for=condition=Available",
				"deployment/kuberay-operator", "-n", ns, "--timeout=120s")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "kuberay-operator deployment should be Available")
		})

		It("should have kuberay webhook configuration registered", func() {
			cmd := exec.Command("kubectl", "get", "mutatingwebhookconfigurations",
				"-l", "platform.opendatahub.io/part-of=ray", "-o", "name")
			out, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(out)).NotTo(BeEmpty(),
				"at least one MutatingWebhookConfiguration should be registered for Ray")
		})
	})

	Context("Status Transitions", func() {
		It("should set Ready=False when a critical operand fails", func() {
			By("scaling kuberay-operator to 0 replicas")
			cmd := exec.Command("kubectl", "scale", "deployment/kuberay-operator",
				"-n", ns, "--replicas=0")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Ready or DeploymentsAvailable to become False")
			Eventually(func(g Gomega) {
				ready, err := kubectlGetJsonpath("get", "ray", "default-ray",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
				g.Expect(err).NotTo(HaveOccurred())

				deplAvail, err := kubectlGetJsonpath("get", "ray", "default-ray",
					"-o", "jsonpath={.status.conditions[?(@.type==\"DeploymentsAvailable\")].status}")
				g.Expect(err).NotTo(HaveOccurred())

				readyVal := strings.TrimSpace(ready)
				deplVal := strings.TrimSpace(deplAvail)
				g.Expect(readyVal == "False" || deplVal == "False").To(BeTrue(),
					fmt.Sprintf("expected Ready=False or DeploymentsAvailable=False, got Ready=%s DeploymentsAvailable=%s",
						readyVal, deplVal))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("should recover Ready=True after operand is restored", func() {
			By("scaling kuberay-operator back to 1 replica")
			cmd := exec.Command("kubectl", "scale", "deployment/kuberay-operator",
				"-n", ns, "--replicas=1")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Ready=True")
			Eventually(func(g Gomega) {
				out, err := kubectlGetJsonpath("get", "ray", "default-ray",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(out)).To(Equal("True"))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())
		})
	})

	Context("Management State", func() {
		It("should remove operands when managementState is Removed", func() {
			By("setting managementState to Removed")
			cmd := exec.Command("kubectl", "patch", "ray", "default-ray", "--type=merge",
				"-p", `{"spec":{"managementState":"Removed"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the module to report RemovedComponent")
			Eventually(func(g Gomega) {
				status, err := kubectlGetJsonpath("get", "ray", "default-ray",
					"-o", "jsonpath={.status.conditions[?(@.type==\"DeploymentsAvailable\")].status}")
				g.Expect(err).NotTo(HaveOccurred())
				reason, err := kubectlGetJsonpath("get", "ray", "default-ray",
					"-o", "jsonpath={.status.conditions[?(@.type==\"DeploymentsAvailable\")].reason}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(status)).To(Equal("True"))
				g.Expect(strings.TrimSpace(reason)).To(Equal("RemovedComponent"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for the KubeRay deployment to be removed")
			Eventually(func(g Gomega) {
				out, err := kubectlGetJsonpath("get", "deployment", "kuberay-operator", "-n", ns,
					"--ignore-not-found", "-o", "name")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(out)).To(BeEmpty(), "KubeRay deployment should be removed")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})
	})
})
