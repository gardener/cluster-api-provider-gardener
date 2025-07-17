// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kind_kcp

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/gardener/cluster-api-provider-gardener/test/utils"
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the the purposed to be used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter)))
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting cluster-api-provider-gardener e2e test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	Expect(os.Setenv("KCP_PORT", "6444")).To(Succeed())
	Expect(os.Setenv("KUBECONFIG", kubeconfigKcp)).To(Succeed())

	By("running kcp")
	go func() {
		defer GinkgoRecover()

		cmd := exec.Command("make", "kcp-up")
		_, err := utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to run kcp")
	}()

	By("ensuring provider workspace")
	Eventually(func() error {
		return utils.EnsureAndSwitchWorkspace("gardener")
	}).To(Succeed())

	By("duplicate kubeconfig")
	cmd := exec.Command("cp", kubeconfigKcp, kubeconfigKcpWorkload)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to duplicate kubeconfig")
})
