// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1beta2 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	// GSCPReferenceNamespaceKey is the key used to store the namespace of the GardenerShootControlPlane in a reference.
	GSCPReferenceNamespaceKey = "controlplane.cluster.x-k8s.io/gscp_namespace"
	// GSCPReferenceNameKey is the key used to store the name of the GardenerShootControlPlane in a reference.
	GSCPReferenceNameKey = "controlplane.cluster.x-k8s.io/gscp_name"
	// GSCPReferenceClusterNameKey is the key used to store the name of the cluster in a reference.
	GSCPReferenceClusterNameKey = "controlplane.cluster.x-k8s.io/gscp_cluster"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=gscp
// +kubebuilder:printcolumn:name="Initialized",type=boolean,JSONPath=`.status.initialized`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GardenerShootControlPlane represents a Shoot cluster.
type GardenerShootControlPlane struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the Shoot cluster.
	// If the object's deletion timestamp is set, this field is immutable.
	// +optional
	Spec   GardenerShootControlPlaneSpec   `json:"spec,omitempty"`
	Status GardenerShootControlPlaneStatus `json:"status,omitempty"`
}

// ProviderGSCP contains provider-specific information that are handed-over to the provider-specific
// extension controller.
// This only contains the fields that the GSCP is responsible for.
// The workers are managed through the GardenerWorkerPool CRD.
type ProviderGSCP struct {
	// Type is the type of the provider. This field is immutable.
	Type string `json:"type" protobuf:"bytes,1,opt,name=type"`
	// ControlPlaneConfig contains the provider-specific control plane config blob. Please look up the concrete
	// definition in the documentation of your provider extension.
	// +optional
	ControlPlaneConfig *runtime.RawExtension `json:"controlPlaneConfig,omitempty" protobuf:"bytes,2,opt,name=controlPlaneConfig"`
	// InfrastructureConfig contains the provider-specific infrastructure config blob. Please look up the concrete
	// definition in the documentation of your provider extension.
	// +optional
	InfrastructureConfig *runtime.RawExtension `json:"infrastructureConfig,omitempty" protobuf:"bytes,3,opt,name=infrastructureConfig"`
	// WorkersSettings contains settings for all workers.
	// +optional
	WorkersSettings *gardenercorev1beta1.WorkersSettings `json:"workersSettings,omitempty" protobuf:"bytes,5,opt,name=workersSettings"`
}

// GardenerShootControlPlaneSpec represents the Spec of the Shoot Cluster,
// as well as the fields defined by the Cluster API contract.
type GardenerShootControlPlaneSpec struct {
	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1beta2.APIEndpoint `json:"controlPlaneEndpoint,omitempty,omitzero"`

	// Version defines the desired Kubernetes version for the control plane.
	// The value must be a valid semantic version; also if the value provided by the user does not start with the v prefix, it
	// must be added.
	// +optional
	Version string `json:"version,omitempty"`

	// ProjectNamespace is the namespace in which the Shoot should be placed in.
	// This has to be a valid project namespace within the Gardener cluster.
	// If not set, the namespace of this object will be used in the Gardener cluster.
	// +optional
	ProjectNamespace string `json:"projectNamespace,omitempty"`

	// Workerless indicates whether the Shoot is workerless or not.
	// If set to false, Cluster creation will wait until at least one worker pool is defined.
	Workerless bool `json:"workerless"`

	// Addons contains information about enabled/disabled addons and their configuration.
	// +optional
	Addons *gardenercorev1beta1.Addons `json:"addons,omitempty" protobuf:"bytes,1,opt,name=addons"`
	// CloudProfileName is a name of a CloudProfile object.
	// Deprecated: This field will be removed in a future version of Gardener. Use `CloudProfile` instead.
	// Until removed, this field is synced with the `CloudProfile` field.
	// +optional
	CloudProfileName *string `json:"cloudProfileName,omitempty" protobuf:"bytes,2,opt,name=cloudProfileName"`
	// DNS contains information about the DNS settings of the Shoot.
	// +optional
	DNS *gardenercorev1beta1.DNS `json:"dns,omitempty" protobuf:"bytes,3,opt,name=dns"`
	// Extensions contain type and provider information for Shoot extensions.
	// +optional
	Extensions []gardenercorev1beta1.Extension `json:"extensions,omitempty" protobuf:"bytes,4,rep,name=extensions"`
	// Kubernetes contains the version and configuration settings of the control plane components.
	Kubernetes gardenercorev1beta1.Kubernetes `json:"kubernetes" protobuf:"bytes,6,opt,name=kubernetes"`
	// Networking contains information about cluster networking such as CNI Plugin type, CIDRs, ...etc.
	// +optional
	Networking *gardenercorev1beta1.Networking `json:"networking,omitempty" protobuf:"bytes,7,opt,name=networking"`
	// Monitoring contains information about custom monitoring configurations for the shoot.
	// +optional
	Monitoring *gardenercorev1beta1.Monitoring `json:"monitoring,omitempty" protobuf:"bytes,9,opt,name=monitoring"`
	// Provider contains all provider-specific and provider-relevant information.
	Provider ProviderGSCP `json:"provider" protobuf:"bytes,10,opt,name=provider"`
	// Purpose is the purpose class for this cluster.
	// +optional
	Purpose *gardenercorev1beta1.ShootPurpose `json:"purpose,omitempty" protobuf:"bytes,11,opt,name=purpose,casttype=ShootPurpose"`
	// SecretBindingName is the name of a SecretBinding that has a reference to the provider secret.
	// The credentials inside the provider secret will be used to create the shoot in the respective account.
	// The field is mutually exclusive with CredentialsBindingName.
	// This field is immutable.
	// +optional
	// Deprecated: Use CredentialsBindingName instead. See https://github.com/gardener/gardener/blob/master/docs/usage/shoot-operations/secretbinding-to-credentialsbinding-migration.md for migration instructions.
	SecretBindingName *string `json:"secretBindingName,omitempty" protobuf:"bytes,13,opt,name=secretBindingName"`
	// Resources holds a list of named resource references that can be referred to in extension configs by their names.
	// +optional
	Resources []gardenercorev1beta1.NamedResourceReference `json:"resources,omitempty" protobuf:"bytes,16,rep,name=resources"`
	// Tolerations contains the tolerations for taints on seed clusters.
	// +patchMergeKey=key
	// +patchStrategy=merge
	// +optional
	Tolerations []gardenercorev1beta1.Toleration `json:"tolerations,omitempty" patchStrategy:"merge" patchMergeKey:"key" protobuf:"bytes,17,rep,name=tolerations"`
	// ExposureClassName is the optional name of an exposure class to apply a control plane endpoint exposure strategy.
	// This field is immutable.
	// +optional
	ExposureClassName *string `json:"exposureClassName,omitempty" protobuf:"bytes,18,opt,name=exposureClassName"`
	// SystemComponents contains the settings of system components in the control or data plane of the Shoot cluster.
	// +optional
	SystemComponents *gardenercorev1beta1.SystemComponents `json:"systemComponents,omitempty" protobuf:"bytes,19,opt,name=systemComponents"`
	// ControlPlane contains general settings for the control plane of the shoot.
	// +optional
	ControlPlane *gardenercorev1beta1.ControlPlane `json:"controlPlane,omitempty" protobuf:"bytes,20,opt,name=controlPlane"`
	// SchedulerName is the name of the responsible scheduler which schedules the shoot.
	// If not specified, the default scheduler takes over.
	// This field is immutable.
	// +optional
	SchedulerName *string `json:"schedulerName,omitempty" protobuf:"bytes,21,opt,name=schedulerName"`
	// CloudProfile contains a reference to a CloudProfile or a NamespacedCloudProfile.
	// +optional
	CloudProfile *gardenercorev1beta1.CloudProfileReference `json:"cloudProfile,omitempty"`
	// CredentialsBindingName is the name of a CredentialsBinding that has a reference to the provider credentials.
	// The credentials will be used to create the shoot in the respective account. The field is mutually exclusive with SecretBindingName.
	// +optional
	CredentialsBindingName *string `json:"credentialsBindingName,omitempty" protobuf:"bytes,23,opt,name=credentialsBindingName"`
	// AccessRestrictions describe a list of access restrictions for this shoot cluster.
	// +optional
	AccessRestrictions []gardenercorev1beta1.AccessRestrictionWithOptions `json:"accessRestrictions,omitempty" protobuf:"bytes,24,rep,name=accessRestrictions"`
}

// GardenerShootControlPlaneStatus defines the observed state of GardenerShootControlPlane.
type GardenerShootControlPlaneStatus struct {
	// ShootStatus is the status of the Shoot cluster.
	// +optional
	ShootStatus gardenercorev1beta1.ShootStatus `json:"shootStatus"`

	// Initialized denotes that the Gardener Shoot control plane API Server is initialized and thus
	// it can accept requests.
	// NOTE: this field is part of the Cluster API contract and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed. Please use conditions
	// to check the operational state of the control plane.
	// +optional
	// +kubebuilder:default=false
	Initialized bool `json:"initialized"`

	// Ready denotes that the Gardener Shoot control plane is ready to serve requests.
	// NOTE: this field is part of the Cluster API contract and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed. Please use conditions
	// to check the operational state of the control plane.
	// +optional
	Ready bool `json:"ready"`
}

// +kubebuilder:object:root=true

// GardenerShootControlPlaneList contains a list of GardenerShootControlPlane.
type GardenerShootControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []GardenerShootControlPlane `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &GardenerShootControlPlane{}, &GardenerShootControlPlaneList{})
}
