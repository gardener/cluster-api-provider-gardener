// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"

	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
)

var _ = Describe("GardenerWorkerPool Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		gardenerworkerpool := &infrastructurev1alpha1.GardenerWorkerPool{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind GardenerWorkerPool")
			err := k8sClient.Get(ctx, typeNamespacedName, gardenerworkerpool)
			if err != nil && errors.IsNotFound(err) {
				resource := &infrastructurev1alpha1.GardenerWorkerPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
			Eventually(ctx, func() error { return k8sClient.Get(ctx, typeNamespacedName, gardenerworkerpool) }).Should(Succeed())
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &infrastructurev1alpha1.GardenerWorkerPool{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance GardenerWorkerPool")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &GardenerWorkerPoolReconciler{
				Manager:        mgr,
				GardenerClient: k8sClient,
				IsKCP:          false,
				Scheme:         k8sClient.Scheme(),
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
