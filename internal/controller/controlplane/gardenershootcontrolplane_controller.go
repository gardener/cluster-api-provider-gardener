// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	gardenerauthenticationv1alpha1 "github.com/gardener/gardener/pkg/apis/authentication/v1alpha1"
	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/utils/gardener"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/controlplane/v1alpha1"
	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
	providerutil "github.com/gardener/cluster-api-provider-gardener/internal/util"
)

const (
	// KubeConfigValiditySeconds defines the validity of the kubeconfig in seconds.
	KubeConfigValiditySeconds = 6000
)

// GardenerShootControlPlaneReconciler reconciles a GardenerShootControlPlane object
type GardenerShootControlPlaneReconciler struct {
	Client         client.Client
	GardenerClient client.Client
	Scheme         *runtime.Scheme
	IsKCP          bool

	PrioritizeShoot bool
}

// ControlPlaneContext holds the context for the GardenerShootControlPlane reconciler.
type ControlPlaneContext struct {
	ctx context.Context

	cluster           *clusterv1beta1.Cluster
	shootControlPlane *controlplanev1alpha1.GardenerShootControlPlane
	shoot             *gardenercorev1beta1.Shoot
	clusterName       string
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=gardenershootcontrolplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=gardenershootcontrolplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=gardenershootcontrolplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.gardener.cloud,resources=shoots/adminkubeconfig,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=core.gardener.cloud,resources=shoots;shoots/status,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *GardenerShootControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := runtimelog.FromContext(ctx).WithValues("gardenershootcontrolplane", req.NamespacedName, "cluster", req.ClusterName)

	cpc := ControlPlaneContext{
		ctx:         ctx,
		clusterName: req.ClusterName,
	}

	log.Info("Reconciling GardenerShootControlPlane")
	cpc.shootControlPlane = &controlplanev1alpha1.GardenerShootControlPlane{}
	if err := r.Client.Get(cpc.ctx, req.NamespacedName, cpc.shootControlPlane); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("GardenerShootControlPlane not found or already deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get GardenerShootControlPlane")
		return ctrl.Result{}, err
	}

	log.Info("Getting own cluster")
	var err error
	cpc.cluster, err = util.GetOwnerCluster(cpc.ctx, r.Client, cpc.shootControlPlane.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cpc.cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{Requeue: true}, nil
	}

	if annotations.IsPaused(cpc.cluster, cpc.shootControlPlane) {
		log.Info("GardenerShootControlPlane or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	// Setting the name and namespace of the shoot object here.
	// This is needed to be able to delete the shoot, as well as fetch into this resource.
	shootID := providerutil.ShootNameFromCAPIResources(*cpc.cluster, *cpc.shootControlPlane)
	cpc.shoot = &gardenercorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shootID.Name,
			Namespace: shootID.Namespace,
		},
	}
	// Handle deleted clusters
	if !cpc.shootControlPlane.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(cpc)
	}

	// Handle non-deleted clusters
	return r.reconcile(cpc)
}

var errIncompleteSpecifications = fmt.Errorf("incomplete specifications")

func (r *GardenerShootControlPlaneReconciler) reconcile(cpc ControlPlaneContext) (ctrl.Result, error) {
	log := runtimelog.FromContext(cpc.ctx).WithValues("operation", "reconcile")

	log.Info("Adding finalizer to GardenerShootControlPlane")
	patch := client.MergeFrom(cpc.shootControlPlane.DeepCopy())
	// TODO(tobschli): This clashes with the finalizer that CAPI uses. Maybe we do not need a finalizer at all?
	if controllerutil.AddFinalizer(cpc.shootControlPlane, clusterv1beta1.ClusterFinalizer) {
		if err := r.Client.Patch(cpc.ctx, cpc.shootControlPlane, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	err := r.GardenerClient.Get(cpc.ctx, client.ObjectKeyFromObject(cpc.shoot), cpc.shoot)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("Shoot not found, creating it")
		if err := r.createShoot(cpc); err != nil {
			if errors.Is(err, errIncompleteSpecifications) {
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "Failed to create shoot")
			return ctrl.Result{}, err
		}
		log.Info("Shoot created successfully")
		return ctrl.Result{}, nil
	}

	if r.PrioritizeShoot {
		if err := r.updateStatus(cpc); err != nil {
			log.Error(err, "failed to update status")
			return ctrl.Result{}, err
		}
	}

	if err := r.syncControlPlaneSpecs(cpc); err != nil {
		log.Error(err, "failed to sync control plane spec")
		return ctrl.Result{}, err
	}

	if !cpc.shootControlPlane.Status.Initialized {
		// Wait until the shoot is initialized.
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	log.Info("Reconcile Shoot Access for ClusterAPI")
	err = r.reconcileShootAccess(cpc)
	if err != nil {
		log.Error(err, "Error reconciling Shoot Access for ClusterAPI")
		return ctrl.Result{}, err
	}

	log.Info("Reconcile shootControlEndpoint")
	err = r.reconcileShootControlPlaneEndpoint(cpc)
	if err != nil {
		log.Error(err, "Error reconciling shootControlEndpoint")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled GardenerShootControlPlane")
	record.Event(cpc.shootControlPlane, "GardenerShootControlPlaneReconcile", "Reconciled")
	if !cpc.shootControlPlane.Status.Ready {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (r *GardenerShootControlPlaneReconciler) createShoot(cpc ControlPlaneContext) error {
	log := runtimelog.FromContext(cpc.ctx).WithValues("operation", "createShoot")
	infraCluster := &infrastructurev1alpha1.GardenerShootCluster{}
	if err := r.Client.Get(cpc.ctx, types.NamespacedName{
		Name:      cpc.cluster.Spec.InfrastructureRef.Name,
		Namespace: cpc.cluster.Namespace,
	}, infraCluster); err != nil {
		log.Error(err, "Failed to get infrastructureCluster")
		return err
	}
	isShootWorkerless := cpc.shootControlPlane.Spec.Workerless

	workers, err := r.getWorkerPoolsForCluster(cpc)
	if err != nil {
		log.Error(err, "Failed to get worker pools")
		return err
	}
	if !isShootWorkerless && len(workers) == 0 {
		// TODO(tobschli): Notify the user that no worker pools were found.
		err := fmt.Errorf("no worker pools found: %w", errIncompleteSpecifications)
		log.Info("No worker pools found")
		// Return no error, as we want to wait for the user to create the worker pools
		return err
	} else if isShootWorkerless {
		// If the shoot is supposed to be workerless, we should dismiss all worker pools that might be configured.
		workers = []infrastructurev1alpha1.GardenerWorkerPool{}
	}

	shoot := providerutil.ShootFromCAPIResources(*cpc.cluster, *cpc.shootControlPlane, *infraCluster, workers)
	injectReferenceLabels(shoot, cpc.shootControlPlane, infraCluster, workers, r.IsKCP, cpc.clusterName)
	return r.GardenerClient.Create(cpc.ctx, shoot)
}

func (r *GardenerShootControlPlaneReconciler) reconcileDelete(cpc ControlPlaneContext) (ctrl.Result, error) {
	log := runtimelog.FromContext(cpc.ctx).WithValues("operation", "delete")
	log.Info("Reconciling Delete GardenerShootControlPlane")

	err := r.Client.Delete(cpc.ctx, newEmptyShootAccessSecret(cpc.cluster))
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("Shoot Access Secret not found")
	}
	err = r.GardenerClient.Get(cpc.ctx, client.ObjectKeyFromObject(cpc.shoot), cpc.shoot)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("Shoot not found")
		cpc.shoot = nil
	}

	if err = r.updateStatus(cpc); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if cpc.shoot != nil {
		// Propagate the deletion to the shoot.
		patch := client.MergeFrom(cpc.shoot.DeepCopy())
		annotations.AddAnnotations(cpc.shoot, map[string]string{constants.ConfirmationDeletion: "true"})
		if err := r.GardenerClient.Patch(cpc.ctx, cpc.shoot, patch); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err := r.GardenerClient.Delete(cpc.ctx, cpc.shoot); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true, RequeueAfter: 1 * time.Minute}, nil
	}

	patch := client.MergeFrom(cpc.shootControlPlane.DeepCopy())
	if controllerutil.RemoveFinalizer(cpc.shootControlPlane, clusterv1beta1.ClusterFinalizer) {
		if err = r.Client.Patch(cpc.ctx, cpc.shootControlPlane, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info("Successfully reconciled deletion of GardenerShootControlPlane")
	record.Event(cpc.shootControlPlane, "GardenerShootControlPlaneReconcile", "Reconciled")
	return ctrl.Result{}, nil
}

func (r *GardenerShootControlPlaneReconciler) getWorkerPoolsForCluster(cpc ControlPlaneContext) ([]infrastructurev1alpha1.GardenerWorkerPool, error) {
	log := runtimelog.FromContext(cpc.ctx).WithValues("operation", "getWorkerPoolsForCluster")
	machinePools := &expclusterv1.MachinePoolList{}
	workers := []infrastructurev1alpha1.GardenerWorkerPool{}
	if err := r.Client.List(cpc.ctx, machinePools, client.InNamespace(cpc.cluster.Namespace)); err != nil {
		log.Error(err, "Failed to list machine pools")
		return nil, err
	}

	log.Info(fmt.Sprintf("MachinePools: %v", len(machinePools.Items)))

	for _, machinePool := range machinePools.Items {
		if machinePool.Spec.ClusterName != cpc.cluster.Name {
			continue
		}
		if machinePool.Spec.Template.Spec.InfrastructureRef.GroupVersionKind() != infrastructurev1alpha1.GroupVersion.WithKind("GardenerWorkerPool") {
			continue
		}
		workerRef := machinePool.Spec.Template.Spec.InfrastructureRef
		workerPool := &infrastructurev1alpha1.GardenerWorkerPool{}
		if err := r.Client.Get(cpc.ctx, client.ObjectKey{Name: workerRef.Name, Namespace: machinePool.Namespace}, workerPool); err != nil {
			if apierrors.IsNotFound(err) {
				// TODO(tobschli): Notify the user that the specified worker pool could not be found.
				log.Info("WorkerPool not found")
				continue
			}
			log.Error(err, "Failed to get worker pool")
			return nil, err
		}
		workers = append(workers, *workerPool)
	}
	log.Info(fmt.Sprintf("Workers: %v", len(workers)))
	return workers, nil
}

func (r *GardenerShootControlPlaneReconciler) reconcileShootControlPlaneEndpoint(cpc ControlPlaneContext) error {
	endpoint := ""
	for _, address := range cpc.shoot.Status.AdvertisedAddresses {
		if address.Name == constants.AdvertisedAddressExternal {
			endpoint = address.URL
			break
		}
	}

	if len(endpoint) == 0 {
		return fmt.Errorf("could not find external advertised address for shoot")
	}

	patch := client.MergeFrom(cpc.shootControlPlane.DeepCopy())
	cpc.shootControlPlane.Spec.ControlPlaneEndpoint = clusterv1beta1.APIEndpoint{
		Host: endpoint,
		Port: 443,
	}
	return r.Client.Patch(cpc.ctx, cpc.shootControlPlane, patch)
}

func (r *GardenerShootControlPlaneReconciler) reconcileShootAccess(cpc ControlPlaneContext) error {
	secret := newEmptyShootAccessSecret(cpc.cluster)
	err := r.Client.Get(cpc.ctx, client.ObjectKeyFromObject(secret), secret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		// Create (empty secret)
		if err = r.Client.Create(cpc.ctx, secret); err != nil {
			return err
		}
	}

	valid, err := isKubeConfigValid(secret.Data)
	if err != nil {
		return fmt.Errorf("could not get validity from secret data: %w", err)
	}
	if valid {
		// The kubeconfig is still valid, no need to update it.
		return nil
	}

	adminKubeconfigRequest := &gardenerauthenticationv1alpha1.AdminKubeconfigRequest{
		Spec: gardenerauthenticationv1alpha1.AdminKubeconfigRequestSpec{
			ExpirationSeconds: ptr.To(int64(KubeConfigValiditySeconds)),
		},
	}
	if err := r.GardenerClient.SubResource("adminkubeconfig").Create(cpc.ctx, cpc.shoot, adminKubeconfigRequest); err != nil {
		return err
	}

	secret.Data = map[string][]byte{
		"value":    adminKubeconfigRequest.Status.Kubeconfig,
		"validity": []byte(strconv.FormatInt(time.Now().Add(KubeConfigValiditySeconds*time.Second).Unix(), 10)),
	}

	return r.Client.Update(cpc.ctx, secret)
}

func isKubeConfigValid(data map[string][]byte) (bool, error) {
	validity, ok := data["validity"]
	if !ok {
		return false, nil
	}
	intVal, err := strconv.Atoi(string(validity))
	if err != nil {
		return false, fmt.Errorf("could not convert validity to int: %w", err)
	}
	validityTimeStamp := time.Unix(int64(intVal), 0)
	return time.Now().Add(5 * time.Minute).Before(validityTimeStamp), nil
}

func newEmptyShootAccessSecret(cluster *clusterv1beta1.Cluster) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-kubeconfig", cluster.Name),
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				"cluster.x-k8s.io/cluster-name": cluster.Name,
			},
		},
		Type: clusterv1beta1.ClusterSecretType,
	}
}

func (r *GardenerShootControlPlaneReconciler) syncControlPlaneSpecs(cpc ControlPlaneContext) error {
	log := runtimelog.FromContext(cpc.ctx).WithValues("operation", "syncSpecs")

	originalShoot := cpc.shoot.DeepCopy()
	originalShootControlPlane := cpc.shootControlPlane.DeepCopy()

	if r.PrioritizeShoot {
		providerutil.SyncGSCPSpecFromShoot(originalShoot, cpc.shootControlPlane)

		// Check if GardenerShootControlPlane spec has changed before patching
		if !providerutil.IsControlPlaneSpecEqual(originalShootControlPlane, cpc.shootControlPlane) {
			log.Info("Syncing GardenerShootControlPlane spec <<< Shoot spec")
			patchShootControlPlane := client.MergeFrom(originalShootControlPlane)
			// patch, _ := patchShootControlPlane.Data(cpc.shootControlPlane)
			// log.Info("Calculated patch for GardenerShootControlPlane spec", "patch", string(patch))
			if err := r.Client.Patch(cpc.ctx, cpc.shootControlPlane, patchShootControlPlane); err != nil {
				log.Error(err, "Error while syncing Gardener Shoot to GardenerShootControlPlane")
				return err
			}
		} else {
			log.Info("No changes detected in GardenerShootControlPlane spec, skipping patch")
		}
	} else {
		providerutil.SyncShootSpecFromGSCP(cpc.shoot, originalShootControlPlane)

		// Check if Shoot spec has changed before patching
		if !providerutil.IsShootSpecEqual(originalShoot, cpc.shoot) {
			log.Info("Syncing GardenerShootControlPlane spec >>> Shoot spec")
			patchShoot := client.StrategicMergeFrom(originalShoot)
			// patch, _ := patchShoot.Data(cpc.shoot)
			// log.Info("Calculated patch for Shoot spec", "patch", string(patch))
			if err := r.GardenerClient.Patch(cpc.ctx, cpc.shoot, patchShoot); err != nil {
				log.Error(err, "Error while syncing GardenerShootControlPlane to Gardener Shoot")
				return err
			}
		} else {
			log.Info("No changes detected in Shoot spec, skipping patch")
		}
	}

	return nil
}

func (r *GardenerShootControlPlaneReconciler) updateStatus(cpc ControlPlaneContext) error {
	formerShootStatus := cpc.shootControlPlane.Status.DeepCopy()
	if cpc.shoot != nil {
		shootStatus := gardener.ComputeShootStatus(cpc.shoot.Status.LastOperation, cpc.shoot.Status.LastErrors, cpc.shoot.Status.Conditions...)
		// TODO(LucaBernstein): Adapt readiness check to assert shoot component conditions.
		cpc.shootControlPlane.Status.Ready = shootStatus == gardener.ShootStatusHealthy
		if !cpc.shootControlPlane.Status.Initialized {
			cpc.shootControlPlane.Status.Initialized = controlPlaneReady(cpc.shoot.Status)
		}
		cpc.shootControlPlane.Status.ShootStatus = cpc.shoot.Status
	}
	if apiequality.Semantic.DeepEqual(cpc.shootControlPlane.Status, formerShootStatus) {
		return nil
	}
	return r.Client.Status().Update(cpc.ctx, cpc.shootControlPlane)
}

func controlPlaneReady(shootStatus gardenercorev1beta1.ShootStatus) bool {
	for _, condition := range shootStatus.Conditions {
		if condition.Type != gardenercorev1beta1.ShootControlPlaneHealthy {
			continue
		}
		if condition.Status == gardenercorev1beta1.ConditionTrue {
			return true
		}
	}
	return false
}

func injectReferenceLabels(
	shoot *gardenercorev1beta1.Shoot,
	shootControlPlane *controlplanev1alpha1.GardenerShootControlPlane,
	infraCluster *infrastructurev1alpha1.GardenerShootCluster,
	workerPools []infrastructurev1alpha1.GardenerWorkerPool,
	isKCP bool,
	kcpClusterName string,
) {
	labels := map[string]string{
		controlplanev1alpha1.GSCPReferenceNameKey:      shootControlPlane.Name,
		controlplanev1alpha1.GSCPReferenceNamespaceKey: shootControlPlane.Namespace,

		infrastructurev1alpha1.GSCReferenceNameKey:      infraCluster.Name,
		infrastructurev1alpha1.GSCReferenceNamespaceKey: infraCluster.Namespace,
	}
	if isKCP {
		labels[controlplanev1alpha1.GSCPReferenceClusterNameKey] = kcpClusterName
		labels[infrastructurev1alpha1.GSCReferecenceClusterNameKey] = kcpClusterName
		labels[infrastructurev1alpha1.GSWReferenceClusterNameKey] = kcpClusterName
	}

	for _, workerPool := range workerPools {
		labels[infrastructurev1alpha1.GSWReferenceNamePrefix+workerPool.Name] = infrastructurev1alpha1.GSWTrue
		labels[infrastructurev1alpha1.GSWReferenceNamespaceKey] = workerPool.Namespace
	}

	if shoot.Labels == nil {
		shoot.Labels = labels
	} else {
		for k, v := range labels {
			shoot.Labels[k] = v
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *GardenerShootControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager, targetCluster cluster.Cluster) error {
	name := "gardenershootcontrolplane"
	controller := ctrl.NewControllerManagedBy(mgr)
	if r.PrioritizeShoot {
		controller.
			Named(name + "-prioritized-shoot").
			WatchesRawSource(
				source.Kind[client.Object](targetCluster.GetCache(),
					&gardenercorev1beta1.Shoot{},
					handler.EnqueueRequestsFromMapFunc(r.MapShootToControlPlaneObject),
				),
			)
	} else {
		controller.
			Named(name).
			For(&controlplanev1alpha1.GardenerShootControlPlane{})
	}
	return controller.Complete(kcp.WithClusterInContext(r))
}

// MapShootToControlPlaneObject maps a Shoot object to a GardenerShootControlPlane object.
func (r *GardenerShootControlPlaneReconciler) MapShootToControlPlaneObject(ctx context.Context, obj client.Object) []reconcile.Request {
	var (
		log          = runtimelog.FromContext(ctx).WithValues("shoot", client.ObjectKeyFromObject(obj))
		clusterName  string
		controlPlane *controlplanev1alpha1.GardenerShootControlPlane
	)
	shoot, ok := obj.(*gardenercorev1beta1.Shoot)
	if !ok {
		log.Error(fmt.Errorf("could not assert object to Shoot"), "")
		return nil
	}

	namespace, ok := shoot.GetLabels()[controlplanev1alpha1.GSCPReferenceNamespaceKey]
	if !ok {
		log.Info("Could not find gscp namespace on label")
		return nil
	}

	name, ok := shoot.GetLabels()[controlplanev1alpha1.GSCPReferenceNameKey]
	if !ok {
		log.Info("Could not find gscp name on label")
	}
	if r.IsKCP {
		clusterName, ok = shoot.GetLabels()[controlplanev1alpha1.GSCPReferenceClusterNameKey]
		if !ok {
			log.Info("Could not find gscp cluster on label")
		}
	}

	controlPlane = &controlplanev1alpha1.GardenerShootControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(controlPlane), ClusterName: clusterName}}
}
