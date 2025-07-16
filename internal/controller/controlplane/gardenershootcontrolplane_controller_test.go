// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/controlplane/v1alpha1"
)

var _ = Describe("GardenerShootControlPlane Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		gardenershootcontrolplane := &controlplanev1alpha1.GardenerShootControlPlane{}

		BeforeEach(func() {
			By("Ensuring the custom resource for the Kind GardenerShootControlPlane")
			var err error
			Eventually(func() error {
				err = k8sClient.Get(ctx, typeNamespacedName, gardenershootcontrolplane)
				return client.IgnoreNotFound(err)
			}).WithTimeout(10 * time.Second).Should(Succeed())
			if err != nil && errors.IsNotFound(err) {
				By("Creating the custom resource for the Kind GardenerShootControlPlane")
				resource := &controlplanev1alpha1.GardenerShootControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
			Eventually(ctx, func() error { return k8sClient.Get(ctx, typeNamespacedName, gardenershootcontrolplane) }).WithTimeout(10 * time.Second).Should(Succeed())
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &controlplanev1alpha1.GardenerShootControlPlane{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance GardenerShootControlPlane")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &GardenerShootControlPlaneReconciler{
				Manager: mgr,
			}

			_, err := controllerReconciler.Reconcile(ctx, mcreconcile.Request{
				Request: reconcile.Request{
					NamespacedName: typeNamespacedName,
				},
				ClusterName: mcmanager.LocalCluster,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
