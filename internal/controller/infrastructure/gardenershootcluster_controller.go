// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure

import (
	"context"
	"fmt"

	"github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/apis/core/helper"
	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerRuntimeCluster "sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
	providerutil "github.com/gardener/cluster-api-provider-gardener/internal/util"
)

// GardenerShootClusterReconciler reconciles a GardenerShootCluster object.
type GardenerShootClusterReconciler struct {
	Client         client.Client
	GardenerClient client.Client
	Scheme         *runtime.Scheme
	IsKCP          bool

	PrioritizeShoot bool
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gardenershootclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gardenershootclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gardenershootclusters/finalizers,verbs=update

// Reconcile reconciles and syncs the GardenerShootCluster resource with the corresponding Shoot resource.
func (r *GardenerShootClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := runtimelog.FromContext(ctx).WithValues("gardenershootcluster", req.NamespacedName, "cluster", req.ClusterName)

	log.Info("Reconciling GardenerShootCluster")
	infraCluster := &infrastructurev1alpha1.GardenerShootCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, infraCluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("GardenerShootCluster not found or already deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get GardenerShootCluster")
		return ctrl.Result{}, err
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, infraCluster.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner Cluster")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	if annotations.IsPaused(cluster, infraCluster) {
		log.Info("GardenerShootCluster or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	if !infraCluster.DeletionTimestamp.IsZero() {
		log.Info("GardenerShootCluster is being deleted")
		return r.reconcileDelete(ctx, infraCluster)
	}

	return r.reconcile(ctx, infraCluster, cluster)
}

func (r *GardenerShootClusterReconciler) reconcileDelete(ctx context.Context, infraCluster *infrastructurev1alpha1.GardenerShootCluster) (ctrl.Result, error) {
	log := runtimelog.FromContext(ctx).WithValues("operation", "delete")

	patch := client.MergeFrom(infraCluster.DeepCopy())
	if controllerutil.RemoveFinalizer(infraCluster, v1beta1.ClusterFinalizer) {
		if err := r.Client.Patch(ctx, infraCluster, patch); err != nil {
			log.Error(err, "Failed to patch GardenerShootCluster finalizer")
			return ctrl.Result{}, err
		}
	}

	log.Info("GardenerShootCluster deleted successfully")
	return ctrl.Result{}, nil
}

func (r *GardenerShootClusterReconciler) reconcile(ctx context.Context, infraCluster *infrastructurev1alpha1.GardenerShootCluster, cluster *v1beta1.Cluster) (ctrl.Result, error) {
	log := runtimelog.FromContext(ctx).WithValues("operation", "reconcile")

	log.Info("Adding finalizer to GardenerShootCluster")
	patch := client.MergeFrom(infraCluster.DeepCopy())
	// TODO(tobschli): This clashes with the finalizer that CAPI uses. Maybe we do not need a finalizer at all?
	if controllerutil.AddFinalizer(infraCluster, v1beta1.ClusterFinalizer) {
		if err := r.Client.Patch(ctx, infraCluster, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.syncSpecs(ctx, infraCluster, cluster); err != nil {
		log.Error(err, "Failed to sync GardenerShootCluster spec")
		return ctrl.Result{}, err
	}

	if r.PrioritizeShoot {
		if err := r.updateStatus(ctx, infraCluster, cluster); err != nil {
			log.Error(err, "Failed to update GardenerShootCluster status")
			return ctrl.Result{}, err
		}
	}

	log.Info("GardenerShootCluster reconciled successfully")
	return ctrl.Result{}, nil
}

func (r *GardenerShootClusterReconciler) updateStatus(ctx context.Context, infraCluster *infrastructurev1alpha1.GardenerShootCluster, cluster *v1beta1.Cluster) error {
	log := runtimelog.FromContext(ctx).WithValues("operation", "updateStatus")

	shoot, err := providerutil.ShootFromCluster(ctx, r.GardenerClient, r.Client, cluster)
	if err != nil {
		log.Error(err, "Failed to get Shoot from Cluster")
		return err
	}
	if shoot == nil {
		log.Info("Shoot not found, do nothing")
		return nil
	}

	if shoot.Spec.SeedName == nil {
		log.Info("Shoot does not have a SeedName yet, do nothing")
		return nil
	}

	seed := &gardenercorev1beta1.Seed{}
	if err := r.GardenerClient.Get(ctx, types.NamespacedName{Name: *shoot.Spec.SeedName}, seed); err != nil {
		log.Error(err, "Failed to get Seed")
		return err
	}

	coreSeed := core.Seed{}
	if err := gardenercorev1beta1.Convert_v1beta1_Seed_To_core_Seed(seed, &coreSeed, nil); err != nil {
		log.Error(err, "Failed to convert Seed from v1beta1 to core")
		return err
	}

	patch := client.MergeFrom(infraCluster.DeepCopy())

	gardenletReadyCondition := helper.GetCondition(coreSeed.Status.Conditions, core.SeedGardenletReady)
	backupBucketCondition := helper.GetCondition(coreSeed.Status.Conditions, core.SeedBackupBucketsReady)
	extensionsReadyCondition := helper.GetCondition(coreSeed.Status.Conditions, core.SeedExtensionsReady)
	seedSystemComponentsHealthyCondition := helper.GetCondition(coreSeed.Status.Conditions, core.SeedSystemComponentsHealthy)

	if gardenletReadyCondition != nil && gardenletReadyCondition.Status == core.ConditionUnknown ||
		(gardenletReadyCondition == nil || gardenletReadyCondition.Status != core.ConditionTrue) ||
		(backupBucketCondition != nil && backupBucketCondition.Status != core.ConditionTrue) ||
		(extensionsReadyCondition == nil || extensionsReadyCondition.Status == core.ConditionFalse || extensionsReadyCondition.Status == core.ConditionUnknown) ||
		(seedSystemComponentsHealthyCondition != nil && (seedSystemComponentsHealthyCondition.Status == core.ConditionFalse || seedSystemComponentsHealthyCondition.Status == core.ConditionUnknown)) {
		infraCluster.Status.Ready = false
	} else {
		infraCluster.Status.Ready = true
	}

	if err := r.Client.Status().Patch(ctx, infraCluster, patch); err != nil {
		log.Error(err, "Failed to patch GardenerShootCluster status")
		return err
	}

	log.Info("GardenerShootCluster status updated successfully")
	return nil
}

func (r *GardenerShootClusterReconciler) syncSpecs(ctx context.Context, infraCluster *infrastructurev1alpha1.GardenerShootCluster, cluster *v1beta1.Cluster) error {
	log := runtimelog.FromContext(ctx).WithValues("operation", "syncSpecs")

	shoot, err := providerutil.ShootFromCluster(ctx, r.GardenerClient, r.Client, cluster)
	if err != nil {
		log.Error(err, "Failed to get Shoot from Cluster")
		return err
	}
	if shoot == nil {
		log.Info("Shoot not found, do nothing")
		return nil
	}

	// Deep copy the original objects for patching
	originalShoot := shoot.DeepCopy()
	originalInfraCluster := infraCluster.DeepCopy()

	if r.PrioritizeShoot {
		providerutil.SyncClusterSpecFromShoot(originalShoot, infraCluster)

		// Check if GardenerShootCluster spec has changed before patching
		if !providerutil.IsClusterSpecEqual(originalInfraCluster, infraCluster) {
			log.Info("Syncing GardenerShootCluster spec <<< Shoot spec")
			patchInfraCluster := client.MergeFrom(originalInfraCluster)
			// patch, _ := patchInfraCluster.Data(infraCluster)
			// log.Info("Calculated patch for GSC (infraCluster) spec", "patch", string(patch))
			if err := r.Client.Patch(ctx, infraCluster, patchInfraCluster); err != nil {
				log.Error(err, "Error while syncing Gardener Shoot to GardenerShootCluster")
				return err
			}
		} else {
			log.Info("No changes detected in GardenerShootCluster spec, skipping patch")
		}
	} else {
		providerutil.SyncShootSpecFromCluster(shoot, originalInfraCluster)

		// Check if Shoot spec has changed before patching
		if !providerutil.IsShootSpecEqual(originalShoot, shoot) {
			log.Info("Syncing GardenerShootCluster spec >>> Shoot spec")
			patchShoot := client.StrategicMergeFrom(originalShoot)
			// patch, _ := patchShoot.Data(shoot)
			// log.Info("Calculated patch for GSC (Shoot) spec", "patch", string(patch))
			if err := r.GardenerClient.Patch(ctx, shoot, patchShoot); err != nil {
				log.Error(err, "Error while syncing GardenerShootCluster to Gardener Shoot")
				return err
			}
		} else {
			log.Info("No changes detected in Shoot spec, skipping patch")
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GardenerShootClusterReconciler) SetupWithManager(mgr ctrl.Manager, targetCluster controllerRuntimeCluster.Cluster) error {
	name := "gardenershootcluster"
	controller := ctrl.NewControllerManagedBy(mgr)
	if r.PrioritizeShoot {
		controller.
			Named(name + "-prioritized-shoot").
			WatchesRawSource(
				source.Kind[client.Object](targetCluster.GetCache(),
					&gardenercorev1beta1.Shoot{},
					handler.EnqueueRequestsFromMapFunc(r.MapShootToGardenerShootClusterObject),
				),
			)
	} else {
		controller.
			Named(name).
			For(&infrastructurev1alpha1.GardenerShootCluster{})
	}
	return controller.Complete(kcp.WithClusterInContext(r))
}

// MapShootToGardenerShootClusterObject maps a Shoot object to a GardenerShootCluster object for reconciliation.
func (r *GardenerShootClusterReconciler) MapShootToGardenerShootClusterObject(ctx context.Context, obj client.Object) []reconcile.Request {
	var (
		log          = runtimelog.FromContext(ctx).WithValues("shoot", client.ObjectKeyFromObject(obj))
		clusterName  string
		infraCluster *infrastructurev1alpha1.GardenerShootCluster
	)
	shoot, ok := obj.(*gardenercorev1beta1.Shoot)
	if !ok {
		log.Error(fmt.Errorf("could not assert object to Shoot"), "")
		return nil
	}

	namespace, ok := shoot.GetLabels()[infrastructurev1alpha1.GSCReferenceNamespaceKey]
	if !ok {
		log.Info("Could not find gsc namespace on label")
		return nil
	}

	name, ok := shoot.GetLabels()[infrastructurev1alpha1.GSCReferenceNameKey]
	if !ok {
		log.Info("Could not find gsc name on label")
	}
	if r.IsKCP {
		clusterName, ok = shoot.GetLabels()[infrastructurev1alpha1.GSCReferecenceClusterNameKey]
		if !ok {
			log.Info("Could not find gsc cluster on label")
		}
	}

	infraCluster = &infrastructurev1alpha1.GardenerShootCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(infraCluster), ClusterName: clusterName}}
}
