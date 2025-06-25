// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/controlplane/v1alpha1"
	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
)

// ClusterController mocks the cluster-api Cluster controller.
// This _ONLY_ works with the Gardener provider, as no dynamic watching is being done here.
type ClusterController struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// Reconcile reconciles the Cluster resource.
func (r *ClusterController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := runtimelog.FromContext(ctx).WithValues("cluster-object", req.NamespacedName, "cluster", req.ClusterName)

	log.Info("Getting Cluster")
	cluster := v1beta1.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource no longer exists")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	patch := client.MergeFrom(cluster.DeepCopy())
	if controllerutil.AddFinalizer(&cluster, v1beta1.ClusterFinalizer) {
		if err := r.Client.Patch(ctx, &cluster, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if the cluster is being deleted
	if !cluster.DeletionTimestamp.IsZero() {
		log.Info("Cluster is being deleted")
		if cluster.Status.Phase != string(v1beta1.ClusterPhaseDeleting) {
			cluster.Status.Phase = string(v1beta1.ClusterPhaseDeleting)
			if err := r.Client.Status().Update(ctx, &cluster); err != nil {
				log.Error(err, "unable to update cluster status")
				return ctrl.Result{}, err
			}
		}
		// Check whether the gscp and gsc are still present
		gscp := &controlplanev1alpha1.GardenerShootControlPlane{}
		gscpErr := r.Client.Get(ctx, client.ObjectKey{
			Name:      cluster.Spec.ControlPlaneRef.Name,
			Namespace: cluster.Namespace,
		}, gscp)
		if gscpErr == nil {
			if err := r.Client.Delete(ctx, gscp); err != nil {
				log.Error(err, "unable to delete gscp")
				return ctrl.Result{}, err
			}
		}

		infraCluster := &infrastructurev1alpha1.GardenerShootCluster{}
		infrErr := r.Client.Get(ctx, client.ObjectKey{
			Name:      cluster.Spec.InfrastructureRef.Name,
			Namespace: cluster.Namespace,
		}, infraCluster)
		if infrErr == nil {
			if err := r.Client.Delete(ctx, infraCluster); err != nil {
				log.Error(err, "unable to delete gsc")
				return ctrl.Result{}, err
			}
		}

		if apierrors.IsNotFound(gscpErr) && apierrors.IsNotFound(infrErr) {
			log.Info("Cluster deletion complete")
			controllerutil.RemoveFinalizer(&cluster, v1beta1.ClusterFinalizer)
			if err := r.Client.Update(ctx, &cluster); err != nil {
				log.Error(err, "unable to remove finalizer")
				return ctrl.Result{}, err
			}
		}
	}

	// Mocking setting the Owner reference for GardenerShootControlPlanes
	gscp := &controlplanev1alpha1.GardenerShootControlPlane{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      cluster.Spec.ControlPlaneRef.Name,
		Namespace: cluster.Namespace,
	}, gscp); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("could not find respective GSCP. Requeueing.")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	if err := ensureOwnerRef(ctx, r.Client, gscp, &cluster); err != nil {
		log.Error(err, "unable to ensure OwnerRef on GSC")
		return ctrl.Result{}, err
	}

	// Mocking setting the Owner reference for GardenerShootClusters
	infraCluster := &infrastructurev1alpha1.GardenerShootCluster{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      cluster.Spec.InfrastructureRef.Name,
		Namespace: cluster.Namespace,
	}, infraCluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("could not find respective GSC. Requeueing.")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	if err := ensureOwnerRef(ctx, r.Client, infraCluster, &cluster); err != nil {
		log.Error(err, "unable to ensure OwnerRef on GSC")
		return ctrl.Result{}, err
	}

	cluster.Status = v1beta1.ClusterStatus{
		Phase:               string(v1beta1.ClusterPhaseProvisioned),
		InfrastructureReady: infraCluster.Status.Ready,
		ControlPlaneReady:   gscp.Status.Initialized,
		ObservedGeneration:  cluster.Generation,
	}
	if !gscp.Status.Initialized || !infraCluster.Status.Ready {
		cluster.Status.Phase = string(v1beta1.ClusterPhaseProvisioning)
	}
	if err := r.Client.Status().Update(ctx, &cluster); err != nil {
		log.Error(err, "unable to update cluster status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func ensureOwnerRef(ctx context.Context, c client.Client, obj metav1.Object, cluster *v1beta1.Cluster) error {
	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: cluster.APIVersion,
		Kind:       cluster.Kind,
		Name:       cluster.Name,
		UID:        cluster.UID,
		Controller: ptr.To(true),
	}

	if util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) &&
		obj.GetLabels()[v1beta1.ClusterNameLabel] == cluster.Name {
		return nil
	}

	if err := controllerutil.SetControllerReference(cluster, obj, c.Scheme()); err != nil {
		return err
	}

	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[v1beta1.ClusterNameLabel] = cluster.Name
	obj.SetLabels(labels)

	// Update the object in the cluster
	if uObj, ok := obj.(client.Object); ok {
		return c.Update(ctx, uObj)
	}
	return fmt.Errorf("object does not implement client.Object")
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.Cluster{}).
		Named("cluster").
		Owns(&controlplanev1alpha1.GardenerShootControlPlane{}).
		Owns(&infrastructurev1alpha1.GardenerShootCluster{}).
		Complete(kcp.WithClusterInContext(r))
}
