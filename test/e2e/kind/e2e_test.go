// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kind

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/cluster-api-provider-gardener/api"
	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/controlplane/v1alpha1"
	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
	"github.com/gardener/cluster-api-provider-gardener/test/utils"
)

const (
	// namespace where the project is deployed in
	namespace = "cluster-api-provider-gardener-system"
)

var _ = Describe("Manager", Ordered, Label("kind"), func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should create a CAPI Cluster resulting in a hibernated Shoot", func(ctx SpecContext) {
			By("create client")
			clusterClient, err := kubernetes.NewClientFromFile("", os.Getenv("KUBECONFIG"),
				kubernetes.WithClientOptions(client.Options{Scheme: api.Scheme}),
				kubernetes.WithClientConnectionOptions(
					componentbaseconfigv1alpha1.ClientConnectionConfiguration{QPS: 100, Burst: 130}),
				kubernetes.WithAllowedUserFields([]string{kubernetes.AuthTokenFile}),
				kubernetes.WithDisabledCachedClient(),
			)
			Expect(err).ToNot(HaveOccurred())

			namePrefix := "e2e-kind-"

			controlPlaneSpec := &controlplanev1alpha1.GardenerShootControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: namePrefix,
					Namespace:    "default",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "cluster-api-provider-gardener",
						"app.kubernetes.io/managed-by": "kustomize",
					},
				},
				Spec: controlplanev1alpha1.GardenerShootControlPlaneSpec{
					ProjectNamespace: "garden-local",
					Provider:         controlplanev1alpha1.ProviderGSCP{Type: "local"},
					Kubernetes:       gardenercorev1beta1.Kubernetes{Version: "1.32"},
					CloudProfile:     &gardenercorev1beta1.CloudProfileReference{Name: "local"},
					Workerless:       true,
				},
			}

			By("create control plane")
			Eventually(func() error {
				return clusterClient.Client().Create(ctx, controlPlaneSpec)
			}).Should(Succeed())

			infraClusterSpec := &infrastructurev1alpha1.GardenerShootCluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: namePrefix,
					Namespace:    "default",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "cluster-api-provider-gardener",
						"app.kubernetes.io/managed-by": "kustomize",
					},
				},
				Spec: infrastructurev1alpha1.GardenerShootClusterSpec{
					Region:      "local",
					Hibernation: &gardenercorev1beta1.Hibernation{Enabled: ptr.To(true)},
				},
			}

			By("create infra cluster")
			Eventually(func() error {
				return clusterClient.Client().Create(ctx, infraClusterSpec)
			}).Should(Succeed())

			clusterSpec := &v1beta1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: controlPlaneSpec.Name, Namespace: "default"},
				Spec: v1beta1.ClusterSpec{
					ControlPlaneRef: &v1.ObjectReference{
						APIVersion: "controlplane.cluster.x-k8s.io/v1alpha1",
						Kind:       "GardenerShootControlPlane",
						Name:       controlPlaneSpec.Name,
						Namespace:  "default",
					},
					InfrastructureRef: &v1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
						Kind:       "GardenerShootCluster",
						Name:       infraClusterSpec.Name,
						Namespace:  "default",
					},
				},
			}

			By("create cluster")
			Expect(clusterClient.Client().Create(ctx, clusterSpec)).To(Succeed())

			shoot := &gardenercorev1beta1.Shoot{
				ObjectMeta: metav1.ObjectMeta{Name: controlPlaneSpec.Name, Namespace: "garden-local"},
			}

			By(fmt.Sprintf("Ensure control plane is reconciled and shoot is created (Name: %s)", controlPlaneSpec.Name))
			Eventually(func(g Gomega) {
				g.Expect(clusterClient.Client().Get(ctx, client.ObjectKeyFromObject(shoot), shoot)).To(Succeed())
				g.Expect(shoot.Status.LastOperation).ToNot(BeNil())
				g.Expect(shoot.Status.LastOperation.Progress).To(BeEquivalentTo(100))
				g.Expect(shoot.Status.LastOperation.State).To(Equal(gardenercorev1beta1.LastOperationStateSucceeded))
			}).WithTimeout(10 * time.Minute).Should(Succeed())

			By("Ensure control plane spec is updated from shoot")
			Expect(clusterClient.Client().Get(ctx, client.ObjectKeyFromObject(controlPlaneSpec), controlPlaneSpec)).To(Succeed())

			By("Deleting cluster")
			Expect(clusterClient.Client().Delete(ctx, clusterSpec)).To(Succeed())

			By("Ensure shoot receives delete request")
			Eventually(func(g Gomega) {
				g.Expect(clusterClient.Client().Get(ctx, client.ObjectKeyFromObject(shoot), shoot)).To(Succeed())
				g.Expect(shoot.DeletionTimestamp).ToNot(BeNil())
			}).Should(Succeed())

			By("Ensure control plane is deleted")
			Eventually(func() error {
				return clusterClient.Client().Get(ctx, client.ObjectKeyFromObject(controlPlaneSpec), controlPlaneSpec)
			}).WithTimeout(10 * time.Minute).Should(matchers.BeNotFoundError())

			By("Ensure cluster object is deleted")
			Eventually(func() error {
				return clusterClient.Client().Get(ctx, client.ObjectKeyFromObject(controlPlaneSpec), controlPlaneSpec)
			}).Should(matchers.BeNotFoundError())
		})

		It("should provisioned cert-manager", func() {
			By("validating that cert-manager has the certificate Secret")
			verifyCertManager := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secrets", "webhook-server-cert", "-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyCertManager).Should(Succeed())
		})

		It("should have CA injection for validating webhooks", func() {
			By("checking CA injection for validating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"validatingwebhookconfigurations.admissionregistration.k8s.io",
					"cluster-api-provider-gardener-validating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				vwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should have CA injection for mutating webhooks", func() {
			By("checking CA injection for mutating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"mutatingwebhookconfigurations.admissionregistration.k8s.io",
					"cluster-api-provider-gardener-mutating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				mwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(mwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// TODO: Customize the e2e test suite with scenarios specific to your project.
		// Consider applying sample/CR(s) and check their status and/or verifying
		// the reconciliation by using the metrics, i.e.:
		// metricsOutput := getMetricsOutput()
		// Expect(metricsOutput).To(ContainSubstring(
		//    fmt.Sprintf(`controller_runtime_reconcile_total{controller="%s",result="success"} 1`,
		//    strings.ToLower(<Kind>),
		// ))
	})
})
