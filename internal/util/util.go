// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardenerv1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	mcsingle "sigs.k8s.io/multicluster-runtime/providers/single"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/controlplane/v1alpha1"
	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
)

// ShootNameFromCAPIResources generates a NamespacedName for the Shoot resource based on the provided CAPI resources.
func ShootNameFromCAPIResources(cluster clusterv1beta1.Cluster, controlPlane controlplanev1alpha1.GardenerShootControlPlane) types.NamespacedName {
	return types.NamespacedName{
		Name:      cluster.Name,
		Namespace: controlPlane.Spec.ProjectNamespace,
	}
}

// ShootFromCAPIResources creates a new Shoot resource based on the provided CAPI resources.
func ShootFromCAPIResources(
	capiCluster clusterv1beta1.Cluster,
	controlPlane controlplanev1alpha1.GardenerShootControlPlane,
	infraCluster infrastructurev1alpha1.GardenerShootCluster,
	workerPools []infrastructurev1alpha1.GardenerWorkerPool,
) *gardenercorev1beta1.Shoot {
	namespacedName := ShootNameFromCAPIResources(capiCluster, controlPlane)

	workerConfigs := make([]gardenercorev1beta1.Worker, 0, len(workerPools))
	for _, pool := range workerPools {
		workerConfigs = append(workerConfigs, *WorkerConfigFromWorkerPool(&pool))
	}

	providerConfig := gardenercorev1beta1.Provider{
		Type:                 controlPlane.Spec.Provider.Type,
		ControlPlaneConfig:   controlPlane.Spec.Provider.ControlPlaneConfig,
		InfrastructureConfig: controlPlane.Spec.Provider.InfrastructureConfig,
		Workers:              workerConfigs,
		WorkersSettings:      controlPlane.Spec.Provider.WorkersSettings,
	}

	return &gardenercorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: gardenercorev1beta1.ShootSpec{
			Addons:                 controlPlane.Spec.Addons,
			DNS:                    controlPlane.Spec.DNS,
			Extensions:             controlPlane.Spec.Extensions,
			Hibernation:            infraCluster.Spec.Hibernation,
			Kubernetes:             controlPlane.Spec.Kubernetes,
			Networking:             controlPlane.Spec.Networking,
			Maintenance:            infraCluster.Spec.Maintenance,
			Monitoring:             controlPlane.Spec.Monitoring,
			Provider:               providerConfig,
			Purpose:                controlPlane.Spec.Purpose,
			Region:                 infraCluster.Spec.Region,
			SecretBindingName:      controlPlane.Spec.SecretBindingName,
			SeedName:               infraCluster.Spec.SeedName,
			SeedSelector:           infraCluster.Spec.SeedSelector,
			Resources:              controlPlane.Spec.Resources,
			Tolerations:            controlPlane.Spec.Tolerations,
			ExposureClassName:      controlPlane.Spec.ExposureClassName,
			SystemComponents:       controlPlane.Spec.SystemComponents,
			ControlPlane:           controlPlane.Spec.ControlPlane,
			SchedulerName:          controlPlane.Spec.SchedulerName,
			CloudProfile:           controlPlane.Spec.CloudProfile,
			CredentialsBindingName: controlPlane.Spec.CredentialsBindingName,
			AccessRestrictions:     controlPlane.Spec.AccessRestrictions,
		},
	}
}

var (
	// AnnotationAllowList defines the list of annotations that are allowed to be synced between GardenerShootControlPlane and Shoot.
	// This is used to ensure that only specific annotations are propagated to the Shoot resource.
	AnnotationAllowList = []string{
		gardenerv1beta1constants.GardenerOperation,
	}
)

func syncAnnotations(source, target map[string]string, allowedKeys []string) map[string]string {
	for _, key := range allowedKeys {
		// Add or update annotations from source to target if they are in the allowed list
		if sourceValue, existsInSource := source[key]; existsInSource {
			target[key] = sourceValue
			continue
		}

		// Remove annotations from target that are not in the source and are in the allowed list
		_, existsInTarget := target[key]
		if existsInTarget {
			delete(target, key)
		}
	}

	return target
}

// SyncShootSpecFromGSCP syncs the Shoot spec from the GardenerShootControlPlane spec.
func SyncShootSpecFromGSCP(shoot *gardenercorev1beta1.Shoot, controlPlane *controlplanev1alpha1.GardenerShootControlPlane) {
	shoot.Annotations = syncAnnotations(controlPlane.Annotations, shoot.Annotations, AnnotationAllowList)

	shoot.Spec.Addons = controlPlane.Spec.Addons
	shoot.Spec.DNS = controlPlane.Spec.DNS
	shoot.Spec.Extensions = controlPlane.Spec.Extensions
	shoot.Spec.Kubernetes = controlPlane.Spec.Kubernetes
	shoot.Spec.Networking = controlPlane.Spec.Networking
	shoot.Spec.Monitoring = controlPlane.Spec.Monitoring
	SyncShootProviderFromGSCP(shoot, controlPlane)
	shoot.Spec.Purpose = controlPlane.Spec.Purpose
	shoot.Spec.SecretBindingName = controlPlane.Spec.SecretBindingName
	// Let's not allow updates on SeedName as this causes the reconciler to not be able to update anything
	// shoot.Spec.SeedName = controlPlane.Spec.SeedName
	shoot.Spec.Resources = controlPlane.Spec.Resources
	shoot.Spec.Tolerations = controlPlane.Spec.Tolerations
	shoot.Spec.ExposureClassName = controlPlane.Spec.ExposureClassName
	shoot.Spec.SystemComponents = controlPlane.Spec.SystemComponents
	shoot.Spec.ControlPlane = controlPlane.Spec.ControlPlane
	shoot.Spec.SchedulerName = controlPlane.Spec.SchedulerName
}

// SyncGSCPSpecFromShoot syncs the GardenerShootControlPlane spec from the Shoot spec.
func SyncGSCPSpecFromShoot(shoot *gardenercorev1beta1.Shoot, controlPlane *controlplanev1alpha1.GardenerShootControlPlane) {
	controlPlane.Annotations = syncAnnotations(shoot.Annotations, controlPlane.Annotations, AnnotationAllowList)

	controlPlane.Spec.Addons = shoot.Spec.Addons
	controlPlane.Spec.DNS = shoot.Spec.DNS
	controlPlane.Spec.Extensions = shoot.Spec.Extensions
	controlPlane.Spec.Kubernetes = shoot.Spec.Kubernetes
	controlPlane.Spec.Networking = shoot.Spec.Networking
	controlPlane.Spec.Monitoring = shoot.Spec.Monitoring
	SyncGSCPProviderFromShoot(shoot, controlPlane)
	controlPlane.Spec.Purpose = shoot.Spec.Purpose
	controlPlane.Spec.SecretBindingName = shoot.Spec.SecretBindingName
	controlPlane.Spec.Resources = shoot.Spec.Resources
	controlPlane.Spec.Tolerations = shoot.Spec.Tolerations
	controlPlane.Spec.ExposureClassName = shoot.Spec.ExposureClassName
	controlPlane.Spec.SystemComponents = shoot.Spec.SystemComponents
	controlPlane.Spec.ControlPlane = shoot.Spec.ControlPlane
	controlPlane.Spec.SchedulerName = shoot.Spec.SchedulerName
	controlPlane.Spec.CredentialsBindingName = shoot.Spec.CredentialsBindingName
	controlPlane.Spec.AccessRestrictions = shoot.Spec.AccessRestrictions
}

// SyncShootSpecFromCluster syncs the Shoot spec from the GardenerShootCluster spec.
func SyncShootSpecFromCluster(shoot *gardenercorev1beta1.Shoot, infraCluster *infrastructurev1alpha1.GardenerShootCluster) {
	shoot.Spec.Hibernation = infraCluster.Spec.Hibernation
	shoot.Spec.Maintenance = infraCluster.Spec.Maintenance
	shoot.Spec.Region = infraCluster.Spec.Region
	shoot.Spec.SeedName = infraCluster.Spec.SeedName
	shoot.Spec.SeedSelector = infraCluster.Spec.SeedSelector
}

// SyncClusterSpecFromShoot syncs the GardenerShootCluster spec from the Shoot spec.
func SyncClusterSpecFromShoot(shoot *gardenercorev1beta1.Shoot, infraCluster *infrastructurev1alpha1.GardenerShootCluster) {
	infraCluster.Spec.Hibernation = shoot.Spec.Hibernation
	infraCluster.Spec.Maintenance = shoot.Spec.Maintenance
	infraCluster.Spec.Region = shoot.Spec.Region
	infraCluster.Spec.SeedName = shoot.Spec.SeedName
	infraCluster.Spec.SeedSelector = shoot.Spec.SeedSelector
}

// SyncShootProviderFromGSCP syncs the Shoot provider configuration from the GardenerShootControlPlane provider configuration.
func SyncShootProviderFromGSCP(shoot *gardenercorev1beta1.Shoot, controlPlane *controlplanev1alpha1.GardenerShootControlPlane) {
	shoot.Spec.Provider.Type = controlPlane.Spec.Provider.Type
	shoot.Spec.Provider.ControlPlaneConfig = controlPlane.Spec.Provider.ControlPlaneConfig
	shoot.Spec.Provider.InfrastructureConfig = controlPlane.Spec.Provider.InfrastructureConfig
	shoot.Spec.Provider.WorkersSettings = controlPlane.Spec.Provider.WorkersSettings
}

// SyncGSCPProviderFromShoot syncs the GardenerShootControlPlane provider configuration from the Shoot provider configuration.
func SyncGSCPProviderFromShoot(shoot *gardenercorev1beta1.Shoot, controlPlane *controlplanev1alpha1.GardenerShootControlPlane) {
	controlPlane.Spec.Provider.Type = shoot.Spec.Provider.Type
	controlPlane.Spec.Provider.ControlPlaneConfig = shoot.Spec.Provider.ControlPlaneConfig
	controlPlane.Spec.Provider.InfrastructureConfig = shoot.Spec.Provider.InfrastructureConfig
	controlPlane.Spec.Provider.WorkersSettings = shoot.Spec.Provider.WorkersSettings
}

// WorkerConfigFromWorkerPool converts a GardenerWorkerPool to a GardenerWorker configuration.
func WorkerConfigFromWorkerPool(workerPool *infrastructurev1alpha1.GardenerWorkerPool) *gardenercorev1beta1.Worker {
	return &gardenercorev1beta1.Worker{
		Name: workerPool.Name,

		Annotations:                      workerPool.Spec.Annotations,
		CABundle:                         workerPool.Spec.CABundle,
		CRI:                              workerPool.Spec.CRI,
		Kubernetes:                       workerPool.Spec.Kubernetes,
		Labels:                           workerPool.Spec.Labels,
		Machine:                          workerPool.Spec.Machine,
		Maximum:                          workerPool.Spec.Maximum,
		Minimum:                          workerPool.Spec.Minimum,
		MaxSurge:                         workerPool.Spec.MaxSurge,
		MaxUnavailable:                   workerPool.Spec.MaxUnavailable,
		ProviderConfig:                   workerPool.Spec.ProviderConfig,
		Taints:                           workerPool.Spec.Taints,
		Volume:                           workerPool.Spec.Volume,
		DataVolumes:                      workerPool.Spec.DataVolumes,
		KubeletDataVolumeName:            workerPool.Spec.KubeletDataVolumeName,
		Zones:                            workerPool.Spec.Zones,
		SystemComponents:                 workerPool.Spec.SystemComponents,
		MachineControllerManagerSettings: workerPool.Spec.MachineControllerManagerSettings,
		Sysctls:                          workerPool.Spec.Sysctls,
		ClusterAutoscaler:                workerPool.Spec.ClusterAutoscaler,
		Priority:                         workerPool.Spec.Priority,
		UpdateStrategy:                   workerPool.Spec.UpdateStrategy,
		ControlPlane:                     workerPool.Spec.ControlPlane,
	}
}

// SyncShootSpecFromWorkerPool syncs the Shoot spec from the GardenerWorkerPool spec.
func SyncShootSpecFromWorkerPool(shoot *gardenercorev1beta1.Shoot, workerPool *infrastructurev1alpha1.GardenerWorkerPool) {
	workers := shoot.Spec.Provider.Workers
	for i, worker := range workers {
		if worker.Name != workerPool.Name {
			continue
		}
		workers[i] = *WorkerConfigFromWorkerPool(workerPool)
	}
}

// SyncWorkerPoolFromShootSpec syncs the GardenerWorkerPool spec from the Shoot spec.
func SyncWorkerPoolFromShootSpec(shoot *gardenercorev1beta1.Shoot, workerPool *infrastructurev1alpha1.GardenerWorkerPool) {
	workers := shoot.Spec.Provider.Workers
	for _, worker := range workers {
		if worker.Name != workerPool.Name {
			continue
		}
		workerPool.Spec.Annotations = worker.Annotations
		workerPool.Spec.CABundle = worker.CABundle
		workerPool.Spec.CRI = worker.CRI
		workerPool.Spec.Kubernetes = worker.Kubernetes
		workerPool.Spec.Labels = worker.Labels
		workerPool.Spec.Machine = worker.Machine
		workerPool.Spec.Maximum = worker.Maximum
		workerPool.Spec.Minimum = worker.Minimum
		workerPool.Spec.MaxSurge = worker.MaxSurge
		workerPool.Spec.MaxUnavailable = worker.MaxUnavailable
		workerPool.Spec.ProviderConfig = worker.ProviderConfig
		workerPool.Spec.Taints = worker.Taints
		workerPool.Spec.Volume = worker.Volume
		workerPool.Spec.DataVolumes = worker.DataVolumes
		workerPool.Spec.KubeletDataVolumeName = worker.KubeletDataVolumeName
		workerPool.Spec.Zones = worker.Zones
		workerPool.Spec.SystemComponents = worker.SystemComponents
		workerPool.Spec.MachineControllerManagerSettings = worker.MachineControllerManagerSettings
		workerPool.Spec.Sysctls = worker.Sysctls
		workerPool.Spec.ClusterAutoscaler = worker.ClusterAutoscaler
		workerPool.Spec.Priority = worker.Priority
		workerPool.Spec.UpdateStrategy = worker.UpdateStrategy
		workerPool.Spec.ControlPlane = worker.ControlPlane
	}
}

// ShootFromCluster retrieves the Shoot resource from the Gardener API based on the provided Cluster and ControlPlane references.
func ShootFromCluster(ctx context.Context, gardenerClient client.Client, client client.Client, cluster *clusterv1beta1.Cluster) (*gardenercorev1beta1.Shoot, error) {
	log := runtimelog.FromContext(ctx).WithValues("operation", "shootFromCluster")

	if cluster.Spec.ControlPlaneRef == nil {
		log.Info("ControlPlaneRef is nil, do nothing")
		return nil, nil
	}
	controlPlane := &controlplanev1alpha1.GardenerShootControlPlane{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: cluster.Spec.ControlPlaneRef.Namespace, Name: cluster.Spec.ControlPlaneRef.Name}, controlPlane); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ControlPlane not found")
			return nil, nil
		}
		log.Error(err, "Failed to get ControlPlane")
		return nil, err
	}

	shoot := &gardenercorev1beta1.Shoot{}
	if err := gardenerClient.Get(ctx, ShootNameFromCAPIResources(*cluster, *controlPlane), shoot); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return shoot, nil
}

// GetMachinePoolForWorkerPool retrieves the MachinePool that owns the given GardenerWorkerPool.
func GetMachinePoolForWorkerPool(ctx context.Context, c client.Client, workerPool *infrastructurev1alpha1.GardenerWorkerPool) (*expclusterv1.MachinePool, error) {
	log := runtimelog.FromContext(ctx).WithValues("operation", "GetMachinePoolForWorkerPool")
	machinePool := &expclusterv1.MachinePool{}
	for _, owner := range workerPool.OwnerReferences {
		if owner.Kind == "MachinePool" && owner.APIVersion == expclusterv1.GroupVersion.String() {
			if err := c.Get(ctx, client.ObjectKey{Namespace: workerPool.Namespace, Name: owner.Name}, machinePool); err != nil {
				if apierrors.IsNotFound(err) {
					log.Info("MachinePool not found or already deleted")
					return nil, nil
				}
				log.Error(err, "Failed to get MachinePool")
				return nil, err
			}
			log.Info("Found owning MachinePool", "machinepool", machinePool.Name)
			break
		}
	}
	return machinePool, nil
}

// IsShootSpecEqual checks if the original and updated GardenerShoot specs and annotations are equal.
func IsShootSpecEqual(original, updated *gardenercorev1beta1.Shoot) bool {
	return apiequality.Semantic.DeepEqual(original.Spec, updated.Spec) && apiequality.Semantic.DeepEqual(original.Annotations, updated.Annotations)
}

// IsClusterSpecEqual checks if the original and updated GardenerShootCluster specs are equal.
func IsClusterSpecEqual(original, updated *infrastructurev1alpha1.GardenerShootCluster) bool {
	return apiequality.Semantic.DeepEqual(original.Spec, updated.Spec)
}

// IsControlPlaneSpecEqual checks if the original and updated GardenerShootControlPlane specs and annotations are equal.
func IsControlPlaneSpecEqual(original, updated *controlplanev1alpha1.GardenerShootControlPlane) bool {
	return apiequality.Semantic.DeepEqual(original.Spec, updated.Spec) && apiequality.Semantic.DeepEqual(original.Annotations, updated.Annotations)
}

// IsWorkerPoolSpecEqual checks if the original and updated GardenerWorkerPool specs are equal.
func IsWorkerPoolSpecEqual(original, updated *infrastructurev1alpha1.GardenerWorkerPool) bool {
	return apiequality.Semantic.DeepEqual(original.Spec, updated.Spec)
}

// ProviderWithRun is an interface that extends the multicluster.Provider interface that expects to be runnable.
type ProviderWithRun interface {
	multicluster.Provider
	Run(context.Context, mcmanager.Manager) error
}

// SingleClusterProviderWithRun wraps a multicluster.Provider to run on a single cluster.
type SingleClusterProviderWithRun struct {
	multicluster.Provider
	Cluster cluster.Cluster
}

// NewSingleClusterProviderWithRun creates a Single Cluster provider instance for the given cluster.
func NewSingleClusterProviderWithRun(cluster cluster.Cluster) *SingleClusterProviderWithRun {
	return &SingleClusterProviderWithRun{
		Provider: mcsingle.New(mcmanager.LocalCluster, cluster),
		Cluster:  cluster,
	}
}

// Run starts the single cluster provider with the specified manager.
func (s *SingleClusterProviderWithRun) Run(ctx context.Context, mgr mcmanager.Manager) error {
	if err := mgr.Engage(ctx, mcmanager.LocalCluster, s.Cluster); err != nil {
		return err
	}
	return s.Provider.(ProviderWithRun).Run(ctx, mgr)
}
