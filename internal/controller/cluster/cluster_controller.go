// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1beta2 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/controlplane/v1alpha1"
	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
)

// ClusterController mocks the cluster-api Cluster controller.
// This _ONLY_ works with the Gardener provider, as no dynamic watching is being done here.
type ClusterController struct {
	Manager mcmanager.Manager
}

// Reconcile reconciles the Cluster resource.
func (r *ClusterController) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	log := runtimelog.FromContext(ctx).WithValues("cluster-object", req.NamespacedName, "cluster", req.ClusterName)

	cl, err := r.Manager.GetCluster(ctx, req.ClusterName)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get cluster: %w", err)
	}
	c := cl.GetClient()

	log.Info("Getting Cluster")
	cluster := clusterv1beta2.Cluster{}
	if err := c.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource no longer exists")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	patch := client.MergeFrom(cluster.DeepCopy())
	if controllerutil.AddFinalizer(&cluster, clusterv1beta2.ClusterFinalizer) {
		if err := c.Patch(ctx, &cluster, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if the cluster is being deleted
	if !cluster.DeletionTimestamp.IsZero() {
		log.Info("Cluster is being deleted")
		if cluster.Status.Phase != string(clusterv1beta2.ClusterPhaseDeleting) {
			cluster.Status.Phase = string(clusterv1beta2.ClusterPhaseDeleting)
			if err := c.Status().Update(ctx, &cluster); err != nil {
				log.Error(err, "unable to update cluster status")
				return ctrl.Result{}, err
			}
		}
		// Check whether the gscp and gsc are still present
		gscp := &controlplanev1alpha1.GardenerShootControlPlane{}
		gscpErr := c.Get(ctx, client.ObjectKey{
			Name:      cluster.Spec.ControlPlaneRef.Name,
			Namespace: cluster.Namespace,
		}, gscp)
		if gscpErr == nil {
			if err := c.Delete(ctx, gscp); err != nil {
				log.Error(err, "unable to delete gscp")
				return ctrl.Result{}, err
			}
		}

		infraCluster := &infrastructurev1alpha1.GardenerShootCluster{}
		infrErr := c.Get(ctx, client.ObjectKey{
			Name:      cluster.Spec.InfrastructureRef.Name,
			Namespace: cluster.Namespace,
		}, infraCluster)
		if infrErr == nil {
			if err := c.Delete(ctx, infraCluster); err != nil {
				log.Error(err, "unable to delete gsc")
				return ctrl.Result{}, err
			}
		}

		if apierrors.IsNotFound(gscpErr) && apierrors.IsNotFound(infrErr) {
			log.Info("Cluster deletion complete")
			controllerutil.RemoveFinalizer(&cluster, clusterv1beta2.ClusterFinalizer)
			if err := c.Update(ctx, &cluster); err != nil {
				log.Error(err, "unable to remove finalizer")
				return ctrl.Result{}, err
			}
		}
	}

	// Mocking setting the Owner reference for GardenerShootControlPlanes
	gscp := &controlplanev1alpha1.GardenerShootControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{
		Name:      cluster.Spec.ControlPlaneRef.Name,
		Namespace: cluster.Namespace,
	}, gscp); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("could not find respective GSCP. Requeueing.")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	if err := ensureOwnerRef(ctx, c, gscp, &cluster); err != nil {
		log.Error(err, "unable to ensure OwnerRef on GSC")
		return ctrl.Result{}, err
	}

	// Mocking setting the Owner reference for GardenerShootClusters
	infraCluster := &infrastructurev1alpha1.GardenerShootCluster{}
	if err := c.Get(ctx, client.ObjectKey{
		Name:      cluster.Spec.InfrastructureRef.Name,
		Namespace: cluster.Namespace,
	}, infraCluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("could not find respective GSC. Requeueing.")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	if err := ensureOwnerRef(ctx, c, infraCluster, &cluster); err != nil {
		log.Error(err, "unable to ensure OwnerRef on GSC")
		return ctrl.Result{}, err
	}

	cluster.Status = clusterv1beta2.ClusterStatus{
		Phase: string(clusterv1beta2.ClusterPhaseProvisioned),
		Conditions: []metav1.Condition{
			{Type: clusterv1beta2.ClusterInfrastructureReadyCondition, Status: metav1.ConditionTrue, Reason: clusterv1beta2.ReadyReason},
			{Type: clusterv1beta2.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionTrue, Reason: clusterv1beta2.ClusterControlPlaneInitializedReason},
		},
		ObservedGeneration: cluster.Generation,
	}
	if !gscp.Status.Initialized || !infraCluster.Status.Ready {
		cluster.Status.Phase = string(clusterv1beta2.ClusterPhaseProvisioning)
	}
	if err := c.Status().Update(ctx, &cluster); err != nil {
		log.Error(err, "unable to update cluster status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func ensureOwnerRef(ctx context.Context, c client.Client, obj metav1.Object, cluster *clusterv1beta2.Cluster) error {
	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: cluster.APIVersion,
		Kind:       cluster.Kind,
		Name:       cluster.Name,
		UID:        cluster.UID,
		Controller: ptr.To(true),
	}

	if util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) &&
		obj.GetLabels()[clusterv1beta2.ClusterNameLabel] == cluster.Name {
		return nil
	}

	if err := controllerutil.SetControllerReference(cluster, obj, c.Scheme()); err != nil {
		return err
	}

	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[clusterv1beta2.ClusterNameLabel] = cluster.Name
	obj.SetLabels(labels)

	// Update the object in the cluster
	if uObj, ok := obj.(client.Object); ok {
		return c.Update(ctx, uObj)
	}
	return fmt.Errorf("object does not implement client.Object")
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterController) SetupWithManager(mgr mcmanager.Manager) error {
	if r.Manager != nil {
		r.Manager = mgr
	}
	return mcbuilder.ControllerManagedBy(mgr).
		For(&clusterv1beta2.Cluster{}).
		Named("cluster").
		Owns(&controlplanev1alpha1.GardenerShootControlPlane{}).
		Owns(&infrastructurev1alpha1.GardenerShootCluster{}).
		Complete(r)
}
