// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// GSWReferenceNamespaceKey is the key for the namespace in which the Gardener worker pool is referenced.
	GSWReferenceNamespaceKey = "infrastructure.cluster.x-k8s.io/gsw_namespace"
	// GSWReferenceNamePrefix is the prefix for the name of the Gardener worker pool.
	GSWReferenceNamePrefix = "infrastructure.cluster.x-k8s.io/gsw_name-"
	// GSWReferenceClusterNameKey is the key for the name of the Gardener worker pool in the ControlPlane object.
	GSWReferenceClusterNameKey = "infrastructure.cluster.x-k8s.io/gsw_cluster"
	// GSWTrue is a string representation of the boolean value true, used for annotations.
	GSWTrue = "true"
)

// GardenerWorkerPoolSpec defines the desired state of GardenerWorkerPool.
type GardenerWorkerPoolSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ProviderIDList is a list of provider IDs for nodes that belong to this worker pool.
	ProviderIDList []string `json:"providerIDList,omitempty"`
	// Annotations is a map of key/value pairs for annotations for all the `Node` objects in this worker pool.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,1,rep,name=annotations"`
	// CABundle is a certificate bundle which will be installed onto every machine of this worker pool.
	// +optional
	CABundle *string `json:"caBundle,omitempty" protobuf:"bytes,2,opt,name=caBundle"`
	// CRI contains configurations of CRI support of every machine in the worker pool.
	// Defaults to a CRI with name `containerd`.
	// +optional
	CRI *gardenercorev1beta1.CRI `json:"cri,omitempty" protobuf:"bytes,3,opt,name=cri"`
	// Kubernetes contains configuration for Kubernetes components related to this worker pool.
	// +optional
	Kubernetes *gardenercorev1beta1.WorkerKubernetes `json:"kubernetes,omitempty" protobuf:"bytes,4,opt,name=kubernetes"`
	// Labels is a map of key/value pairs for labels for all the `Node` objects in this worker pool.
	// +optional
	Labels map[string]string `json:"labels,omitempty" protobuf:"bytes,5,rep,name=labels"`
	// Machine contains information about the machine type and image.
	Machine gardenercorev1beta1.Machine `json:"machine" protobuf:"bytes,7,opt,name=machine"`
	// Maximum is the maximum number of machines to create.
	// This value is divided by the number of configured zones for a fair distribution.
	Maximum int32 `json:"maximum" protobuf:"varint,8,opt,name=maximum"`
	// Minimum is the minimum number of machines to create.
	// This value is divided by the number of configured zones for a fair distribution.
	Minimum int32 `json:"minimum" protobuf:"varint,9,opt,name=minimum"`
	// MaxSurge is maximum number of machines that are created during an update.
	// This value is divided by the number of configured zones for a fair distribution.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty" protobuf:"bytes,10,opt,name=maxSurge"`
	// MaxUnavailable is the maximum number of machines that can be unavailable during an update.
	// This value is divided by the number of configured zones for a fair distribution.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty" protobuf:"bytes,11,opt,name=maxUnavailable"`
	// ProviderConfig is the provider-specific configuration for this worker pool.
	// +optional
	ProviderConfig *runtime.RawExtension `json:"providerConfig,omitempty" protobuf:"bytes,12,opt,name=providerConfig"`
	// Taints is a list of taints for all the `Node` objects in this worker pool.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty" protobuf:"bytes,13,rep,name=taints"`
	// Volume contains information about the volume type and size.
	// +optional
	Volume *gardenercorev1beta1.Volume `json:"volume,omitempty" protobuf:"bytes,14,opt,name=volume"`
	// DataVolumes contains a list of additional worker volumes.
	// +optional
	DataVolumes []gardenercorev1beta1.DataVolume `json:"dataVolumes,omitempty" protobuf:"bytes,15,rep,name=dataVolumes"`
	// KubeletDataVolumeName contains the name of a dataVolume that should be used for storing kubelet state.
	// +optional
	KubeletDataVolumeName *string `json:"kubeletDataVolumeName,omitempty" protobuf:"bytes,16,opt,name=kubeletDataVolumeName"`
	// Zones is a list of availability zones that are used to evenly distribute this worker pool. Optional
	// as not every provider may support availability zones.
	// +optional
	Zones []string `json:"zones,omitempty" protobuf:"bytes,17,rep,name=zones"`
	// SystemComponents contains configuration for system components related to this worker pool
	// +optional
	SystemComponents *gardenercorev1beta1.WorkerSystemComponents `json:"systemComponents,omitempty" protobuf:"bytes,18,opt,name=systemComponents"`
	// MachineControllerManagerSettings contains configurations for different worker-pools. Eg. MachineDrainTimeout, MachineHealthTimeout.
	// +optional
	MachineControllerManagerSettings *gardenercorev1beta1.MachineControllerManagerSettings `json:"machineControllerManager,omitempty" protobuf:"bytes,19,opt,name=machineControllerManager"`
	// Sysctls is a map of kernel settings to apply on all machines in this worker pool.
	// +optional
	Sysctls map[string]string `json:"sysctls,omitempty" protobuf:"bytes,20,rep,name=sysctls"`
	// ClusterAutoscaler contains the cluster autoscaler configurations for the worker pool.
	// +optional
	ClusterAutoscaler *gardenercorev1beta1.ClusterAutoscalerOptions `json:"clusterAutoscaler,omitempty" protobuf:"bytes,21,opt,name=clusterAutoscaler"`
	// Priority (or weight) is the importance by which this worker group will be scaled by cluster autoscaling.
	// +optional
	Priority *int32 `json:"priority,omitempty" protobuf:"varint,22,opt,name=priority"`
	// UpdateStrategy specifies the machine update strategy for the worker pool.
	// +optional
	UpdateStrategy *gardenercorev1beta1.MachineUpdateStrategy `json:"updateStrategy,omitempty" protobuf:"bytes,23,opt,name=updateStrategy,casttype=MachineUpdateStrategy"`
	// ControlPlane specifies that the shoot cluster control plane components should be running in this worker pool.
	// This is only relevant for autonomous shoot clusters.
	// +optional
	ControlPlane *gardenercorev1beta1.WorkerControlPlane `json:"controlPlane,omitempty" protobuf:"bytes,24,opt,name=controlPlane"`
}

// GardenerWorkerPoolStatus defines the observed state of GardenerWorkerPool.
type GardenerWorkerPoolStatus struct {
	// Ready indicates whether the worker pool is ready.
	Ready bool `json:"ready,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GardenerWorkerPool is the Schema for the gardenerworkerpools API.
type GardenerWorkerPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GardenerWorkerPoolSpec   `json:"spec,omitempty"`
	Status GardenerWorkerPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GardenerWorkerPoolList contains a list of GardenerWorkerPool.
type GardenerWorkerPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GardenerWorkerPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GardenerWorkerPool{}, &GardenerWorkerPoolList{})
}
