// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
)

// MachinePoolController is a controller for managing MachinePool resources.
type MachinePoolController struct {
	Manager mcmanager.Manager
}

// Reconcile reconciles the MachinePool resource.
func (r *MachinePoolController) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	log := runtimelog.FromContext(ctx).WithValues("machinepool-object", req.NamespacedName, "cluster", req.ClusterName)

	cl, err := r.Manager.GetCluster(ctx, req.ClusterName)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get cluster: %w", err)
	}
	c := cl.GetClient()

	log.Info("Getting MachinePool")

	machinePool := clusterv1beta2.MachinePool{}
	if err := c.Get(ctx, req.NamespacedName, &machinePool); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource no longer exists")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	clusterName := machinePool.Spec.ClusterName
	if clusterName == "" {
		log.Info("ClusterName is empty")
		return ctrl.Result{Requeue: true}, nil
	}

	// Get CLuster and ensure the cluster owner ref on the machinePool
	cluster := &clusterv1beta2.Cluster{}
	if err := c.Get(ctx, client.ObjectKey{
		Name:      clusterName,
		Namespace: machinePool.Namespace,
	}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource no longer exists")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := ensureOwnerRef(ctx, c, &machinePool, cluster); err != nil {
		log.Error(err, "unable to set OwnerRef on MachinePool")
		return ctrl.Result{}, err
	}

	if infraRefGroupKind := machinePool.Spec.Template.Spec.InfrastructureRef.GroupKind(); infraRefGroupKind != infrastructurev1alpha1.GroupVersion.WithKind("GardenerWorkerPool").GroupKind() {
		log.Info(fmt.Sprintf("%s is not a GardenerWorkerPool", infraRefGroupKind))
		return ctrl.Result{}, nil
	}

	gsw := &infrastructurev1alpha1.GardenerWorkerPool{}
	if err := c.Get(ctx, client.ObjectKey{
		Name:      machinePool.Spec.Template.Spec.InfrastructureRef.Name,
		Namespace: machinePool.Namespace,
	}, gsw); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource no longer exists")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := ensureMachinePoolOwnerRef(ctx, c, gsw, &machinePool); err != nil {
		log.Error(err, "unable to set OwnerRef on MachinePool")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func ensureMachinePoolOwnerRef(ctx context.Context, c client.Client, obj metav1.Object, machinePool *clusterv1beta2.MachinePool) error {
	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: machinePool.APIVersion,
		Kind:       machinePool.Kind,
		Name:       machinePool.Name,
		UID:        machinePool.UID,
	}

	if util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) {
		return nil
	}

	if err := controllerutil.SetControllerReference(machinePool, obj, c.Scheme()); err != nil {
		return err
	}

	// Update the object in the cluster
	if uObj, ok := obj.(client.Object); ok {
		return c.Update(ctx, uObj)
	}
	return fmt.Errorf("object does not implement client.Object")
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachinePoolController) SetupWithManager(mgr mcmanager.Manager) error {
	return mcbuilder.ControllerManagedBy(mgr).
		For(&clusterv1beta2.MachinePool{}).
		Complete(r)
}
