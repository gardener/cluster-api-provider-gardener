// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kind_kcp

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

// namespace where the project is deployed in
const namespace = "gardener"

var (
	kubeconfigKcp         = ".kcp/admin.kubeconfig"
	kubeconfigKcpWorkload = ".kcp/workload.kubeconfig"
	kubeconfigGardener    = "./bin/gardener/example/provider-local/seed-kind/base/kubeconfig"
)

var _ = Describe("Manager", Ordered, Label("kind-kcp"), func() {
	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("installing APIResourceSchemas, APIExports and APIBindings in the provider workspace")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "apply", "-f", "schemas/gardener")
			output, err := utils.Run(cmd)
			_, _ = fmt.Fprintf(GinkgoWriter, "Output:\n %s", output)
			g.Expect(err).NotTo(HaveOccurred(), "Failed to install APIResourceSchemas, APIExports")
		}).Should(Succeed())

		cmd := exec.Command("kubectl", "apply", "-f", "schemas/binding.yaml")
		output, err := utils.Run(cmd)
		_, _ = fmt.Fprintf(GinkgoWriter, "Output:\n %s", output)
		Expect(err).NotTo(HaveOccurred(), "Failed to install APIBindings")

		go func() {
			defer GinkgoRecover()

			By("running the controller")
			Expect(os.Setenv("ENABLE_WEBHOOKS", "false")).To(Succeed())
			cmd = exec.Command("go", "run", "cmd/main.go", "--kubeconfig", kubeconfigKcp, "-gardener-kubeconfig", kubeconfigGardener)
			controllerOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to run the controller")
			_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerOutput)
		}()
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching Kubernetes events")
			cmd := exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		BeforeAll(func() {
			Expect(os.Setenv("KUBECONFIG", kubeconfigKcpWorkload)).To(Succeed())
			By("Switch to kcp consuming workload workspace")
			Eventually(func() error {
				return utils.EnsureAndSwitchWorkspace("gardener", "test")
			}).To(Succeed())

			By("Create APIBindings in the consuming workspace")
			cmd := exec.Command("kubectl", "apply", "-f", "schemas/binding.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create APIBindings in the consuming workspace")
		})
		It("should create a GardenerShootControlPlane with a hibernated Shoot", func(ctx SpecContext) {
			By("create client")

			kcpClient, err := kubernetes.NewClientFromFile("", kubeconfigKcpWorkload,
				kubernetes.WithClientOptions(client.Options{Scheme: api.Scheme}),
				kubernetes.WithClientConnectionOptions(
					componentbaseconfigv1alpha1.ClientConnectionConfiguration{QPS: 100, Burst: 130}),
				kubernetes.WithAllowedUserFields([]string{kubernetes.AuthTokenFile}),
				kubernetes.WithDisabledCachedClient(),
			)
			Expect(err).ToNot(HaveOccurred())
			gardenerClient, err := kubernetes.NewClientFromFile("", kubeconfigGardener,
				kubernetes.WithClientOptions(client.Options{Scheme: api.Scheme}),
				kubernetes.WithClientConnectionOptions(
					componentbaseconfigv1alpha1.ClientConnectionConfiguration{QPS: 100, Burst: 130}),
				kubernetes.WithAllowedUserFields([]string{kubernetes.AuthTokenFile}),
				kubernetes.WithDisabledCachedClient(),
			)
			Expect(err).ToNot(HaveOccurred())

			namePrefix := "e2e-kcp-"

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
				return kcpClient.Client().Create(ctx, controlPlaneSpec)
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
				return kcpClient.Client().Create(ctx, infraClusterSpec)
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
			Expect(kcpClient.Client().Create(ctx, clusterSpec)).To(Succeed())

			shoot := &gardenercorev1beta1.Shoot{
				ObjectMeta: metav1.ObjectMeta{Name: controlPlaneSpec.Name, Namespace: "garden-local"},
			}

			By(fmt.Sprintf("Ensure shoot is synced to gardener cluster (Name: %s)", controlPlaneSpec.Name))
			Eventually(func(g Gomega) {
				g.Expect(gardenerClient.Client().Get(ctx, client.ObjectKeyFromObject(shoot), shoot)).To(Succeed())
				g.Expect(shoot.Status.LastOperation).ToNot(BeNil())
			}).WithTimeout(30 * time.Second).Should(Succeed())

			By(fmt.Sprintf("Ensure control plane is reconciled and shoot is created (Name: %s)", controlPlaneSpec.Name))
			Eventually(func(g Gomega) {
				g.Expect(gardenerClient.Client().Get(ctx, client.ObjectKeyFromObject(shoot), shoot)).To(Succeed())
				g.Expect(shoot.Status.LastOperation).ToNot(BeNil())
				g.Expect(shoot.Status.LastOperation.Progress).To(BeEquivalentTo(100))
				g.Expect(shoot.Status.LastOperation.State).To(Equal(gardenercorev1beta1.LastOperationStateSucceeded))
			}).WithTimeout(10 * time.Minute).Should(Succeed())

			By("Ensure control plane spec is updated from shoot")
			Expect(kcpClient.Client().Get(ctx, client.ObjectKeyFromObject(controlPlaneSpec), controlPlaneSpec)).To(Succeed())

			By("Deleting cluster")
			Expect(kcpClient.Client().Delete(ctx, clusterSpec)).To(Succeed())

			By("Ensure shoot receives delete request")
			Eventually(func(g Gomega) {
				g.Expect(gardenerClient.Client().Get(ctx, client.ObjectKeyFromObject(shoot), shoot)).To(Succeed())
				g.Expect(shoot.DeletionTimestamp).ToNot(BeNil())
			}).Should(Succeed())

			By("Ensure control plane is deleted")
			Eventually(func() error {
				return kcpClient.Client().Get(ctx, client.ObjectKeyFromObject(controlPlaneSpec), controlPlaneSpec)
			}).WithTimeout(10 * time.Minute).Should(matchers.BeNotFoundError())

			By("Ensure cluster object is deleted")
			Eventually(func() error {
				return kcpClient.Client().Get(ctx, client.ObjectKeyFromObject(controlPlaneSpec), controlPlaneSpec)
			}).Should(matchers.BeNotFoundError())
		})
	})
})
