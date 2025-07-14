// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure

import (
	"context"
	"fmt"
	"strings"

	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerRuntimeCluster "sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"

	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
	providerutil "github.com/gardener/cluster-api-provider-gardener/internal/util"
)

// GardenerWorkerPoolReconciler reconciles a GardenerWorkerPool object
type GardenerWorkerPoolReconciler struct {
	Manager         mcmanager.Manager
	GardenerClient  client.Client
	IsKCP           bool
	Scheme          *runtime.Scheme
	PrioritizeShoot bool
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gardenerworkerpools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gardenerworkerpools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gardenerworkerpools/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools,verbs=get;update;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools/status,verbs=get;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GardenerWorkerPool object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *GardenerWorkerPoolReconciler) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	log := runtimelog.FromContext(ctx).WithValues("gardenerworkerpool", req.NamespacedName, "cluster", req.ClusterName)
	log.Info("Reconciling GardenerWorkerPool")

	cl, err := r.Manager.GetCluster(ctx, req.ClusterName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get cluster: %w", err)
	}
	c := cl.GetClient()

	workerPool := &infrastructurev1alpha1.GardenerWorkerPool{}
	if err := c.Get(ctx, req.NamespacedName, workerPool); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("GardenerWorkerPool not found or already deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get GardenerWorkerPool")
		return ctrl.Result{}, err
	}

	log.Info("Getting owning MachinePool")
	machinePool, err := providerutil.GetMachinePoolForWorkerPool(ctx, c, workerPool)
	if err != nil {
		log.Error(err, "Failed to get MachinePool for GardenerWorkerPool")
		return ctrl.Result{}, err
	}
	if machinePool == nil {
		log.Info("MachinePool not found")
		return ctrl.Result{Requeue: true}, nil
	}

	log.Info("Getting own cluster")
	cluster, err := util.GetOwnerCluster(ctx, c, machinePool.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{Requeue: true}, nil
	}

	if annotations.IsPaused(cluster, workerPool) {
		log.Info("GardenerWorkerPool or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	if !workerPool.DeletionTimestamp.IsZero() {
		return r.reconcileDelete()
	}

	return r.reconcile(ctx, c, workerPool, machinePool, cluster)
}

func (r *GardenerWorkerPoolReconciler) syncSpecs(ctx context.Context, c client.Client, workerPool *infrastructurev1alpha1.GardenerWorkerPool, cluster *clusterv1beta1.Cluster) error {
	log := runtimelog.FromContext(ctx).WithValues("gardenerworkerpool", client.ObjectKeyFromObject(workerPool), "operation", "syncSpecs")

	shoot, err := providerutil.ShootFromCluster(ctx, r.GardenerClient, c, cluster)
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
	originalWorkerPool := workerPool.DeepCopy()

	// Sync the specs between Shoot and GardenerWorkerPool
	if r.PrioritizeShoot {
		providerutil.SyncWorkerPoolFromShootSpec(originalShoot, workerPool)

		// Check if GardenerWorkerPool spec has changed before patching
		if !providerutil.IsWorkerPoolSpecEqual(originalWorkerPool, workerPool) {
			log.Info("Syncing GardenerWorkerPool spec <<< Shoot spec")
			patchWorkerPool := client.MergeFrom(originalWorkerPool)
			// patch, _ := patchWorkerPool.Data(workerPool)
			// log.Info("Calculated patch for GardenerWorkerPool spec", "patch", string(patch))
			if err := c.Patch(ctx, workerPool, patchWorkerPool); err != nil {
				log.Error(err, "Error while syncing Gardener Shoot to GardenerWorkerPool")
				return err
			}
		} else {
			log.Info("No changes detected in GardenerWorkerPool spec, skipping patch")
		}
	} else {
		providerutil.SyncShootSpecFromWorkerPool(shoot, originalWorkerPool)

		// Check if Shoot spec has changed before patching
		if !providerutil.IsShootSpecEqual(originalShoot, shoot) {
			log.Info("Syncing GardenerWorkerPool spec >>> Shoot spec")
			patchShoot := client.StrategicMergeFrom(originalShoot)
			// patch, _ := patchShoot.Data(shoot)
			// log.Info("Calculated patch for Shoot (GardenerWorkerPool) spec", "patch", string(patch))
			if err := r.GardenerClient.Patch(ctx, shoot, patchShoot); err != nil {
				log.Error(err, "Error while syncing GardenerWorkerPool to Gardener Shoot")
				return err
			}
		} else {
			log.Info("No changes detected in Shoot spec, skipping patch")
		}
	}

	return nil
}

func (r *GardenerWorkerPoolReconciler) reconcileDelete() (ctrl.Result, error) {
	// TODO(tobschli): On deletion, the nodes that are deleted should be removed from the providerID list
	return ctrl.Result{}, nil
}

// createClientFromKubeconfig creates a client.Client from a kubeconfig string.
func createClientFromKubeconfig(kubeconfig []byte, scheme *runtime.Scheme) (client.Client, error) {
	// Load the kubeconfig from the provided string
	restConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Convert the kubeconfig to a rest.Config
	config, err := restConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create rest.Config: %w", err)
	}

	// Create a new client.Client using the rest.Config
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return k8sClient, nil
}

func (r *GardenerWorkerPoolReconciler) updateStatus(ctx context.Context, c client.Client, workerPool *infrastructurev1alpha1.GardenerWorkerPool, machinePool *v1beta1.MachinePool, cluster *clusterv1beta1.Cluster) error {
	log := runtimelog.FromContext(ctx).WithValues("operation", "updateStatus")

	// Get the secret for the shoot cluster to get the nodes
	secret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      fmt.Sprintf("%s-kubeconfig", cluster.Name),
		},
	}
	log.Info("Retrieving Shoot Access Secret", "secret", client.ObjectKeyFromObject(secret))
	if err := c.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Shoot Access Secret not found or already deleted")
			return nil
		}
		log.Error(err, "Failed to get Shoot Access Secret")
		return err
	}

	kubeconfig, ok := secret.Data["value"]
	if !ok {
		err := fmt.Errorf("could not find kubeconfig in secret")
		log.Error(err, "Failed to get kubeconfig from secret")
		return err
	}

	shootClient, err := createClientFromKubeconfig(kubeconfig, r.Scheme)
	if err != nil {
		log.Error(err, "Failed to create client from kubeconfig")
		return err
	}

	// Reset it here everytime so we don't write the same ids over and over
	workerPool.Spec.ProviderIDList = []string{}
	nodes := &corev1.NodeList{}
	if err := shootClient.List(ctx, nodes); err != nil {
		log.Error(err, "Failed to list nodes")
		return err
	}
	for _, node := range nodes.Items {
		// TODO remove debug log
		log.Info("Found node", "name", node.Name, "labels", node.Labels)
		if node.Labels[v1beta1constants.LabelWorkerPool] == workerPool.Name {
			// TODO remove debug log
			log.Info("Adding node to providerIDList", "name", node.Name, "providerID", node.Spec.ProviderID)
			workerPool.Spec.ProviderIDList = append(workerPool.Spec.ProviderIDList, node.Spec.ProviderID)
		}
	}

	if err := c.Update(ctx, workerPool); err != nil {
		log.Error(err, "Failed to update GardenerWorkerPool provider IDs")
		return err
	}

	workerPool.Status.Ready = len(workerPool.Spec.ProviderIDList) >= int(workerPool.Spec.Minimum)
	// Update the status
	if err := c.Status().Update(ctx, workerPool); err != nil {
		log.Error(err, "Failed to update GardenerWorkerPool readiness status")
		return err
	}

	machinePool.Status.Replicas = int32(len(workerPool.Spec.ProviderIDList)) // #nosec G115
	if err := c.Status().Update(ctx, machinePool); err != nil {
		log.Error(err, "Failed to update MachinePool replicas")
		return err
	}

	return nil
}

func (r *GardenerWorkerPoolReconciler) reconcile(ctx context.Context, c client.Client, workerPool *infrastructurev1alpha1.GardenerWorkerPool, machinePool *v1beta1.MachinePool, cluster *clusterv1beta1.Cluster) (ctrl.Result, error) {
	log := runtimelog.FromContext(ctx).WithValues("operation", "reconcile")
	if err := r.syncSpecs(ctx, c, workerPool, cluster); err != nil {
		log.Error(err, "Failed to sync GardenerWorkerPool spec")
		return ctrl.Result{}, err
	}

	if r.PrioritizeShoot {
		if err := r.updateStatus(ctx, c, workerPool, machinePool, cluster); err != nil {
			log.Error(err, "Failed to update GardenerWorkerPool status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GardenerWorkerPoolReconciler) SetupWithManager(mgr mcmanager.Manager, targetCluster controllerRuntimeCluster.Cluster) error {
	name := "gardenerworkerpool"
	controller := mcbuilder.ControllerManagedBy(mgr)
	if r.PrioritizeShoot {
		controller.
			Named(name + "-prioritized-shoot").
			WatchesRawSource(
				source.TypedKind[client.Object, mcreconcile.Request](
					targetCluster.GetCache(),
					&gardenercorev1beta1.Shoot{},
					handler.TypedEnqueueRequestsFromMapFunc[client.Object, mcreconcile.Request](r.MapShootToGardenerWorkerPoolObject),
				),
			)
	} else {
		controller.
			Named(name).
			For(&infrastructurev1alpha1.GardenerWorkerPool{})
	}
	return controller.Complete(r)
}

// MapShootToGardenerWorkerPoolObject maps a Shoot object to a list of GardenerWorkerPool reconcile requests.
func (r *GardenerWorkerPoolReconciler) MapShootToGardenerWorkerPoolObject(ctx context.Context, obj client.Object) []mcreconcile.Request {
	var (
		log         = runtimelog.FromContext(ctx).WithValues("shoot", client.ObjectKeyFromObject(obj))
		clusterName string
	)
	shoot, ok := obj.(*gardenercorev1beta1.Shoot)
	if !ok {
		log.Error(fmt.Errorf("could not assert object to Shoot"), "")
		return nil
	}

	namespace, ok := shoot.GetLabels()[infrastructurev1alpha1.GSWReferenceNamespaceKey]
	if !ok {
		log.Info("Could not find gsw namespace on label")
		return nil
	}

	if r.IsKCP {
		clusterName, ok = shoot.GetLabels()[infrastructurev1alpha1.GSWReferenceClusterNameKey]
		if !ok {
			log.Info("Could not find gsw cluster on label")
		}
	}

	requests := []mcreconcile.Request{}
	for key, value := range shoot.GetLabels() {
		if !strings.HasPrefix(key, infrastructurev1alpha1.GSWReferenceNamePrefix) {
			continue
		}
		if value != infrastructurev1alpha1.GSWTrue {
			continue
		}

		name := strings.TrimPrefix(key, infrastructurev1alpha1.GSWReferenceNamePrefix)
		if name == "" {
			log.Info("Could not extract gsw name from label")
			continue
		}

		requests = append(requests, mcreconcile.Request{Request: reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      name,
				Namespace: namespace,
			},
		},
			ClusterName: clusterName,
		})
	}
	return requests
}
