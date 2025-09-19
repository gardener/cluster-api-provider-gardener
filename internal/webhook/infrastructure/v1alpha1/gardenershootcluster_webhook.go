// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/controlplane/v1alpha1"
	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
	providerutil "github.com/gardener/cluster-api-provider-gardener/internal/util"
)

// nolint:unused
// log is for logging in this package.
var _ = logf.Log.WithName("gardenershootcluster-resource")

// SetupGardenerShootClusterWebhookWithManager registers the webhook for GardenerShootCluster in the manager.
func SetupGardenerShootClusterWebhookWithManager(mgr ctrl.Manager, gardenerClient client.Client) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&infrastructurev1alpha1.GardenerShootCluster{}).
		WithValidator(&GardenerShootClusterCustomValidator{
			Client:         mgr.GetClient(),
			GardenerClient: gardenerClient,
		}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-gardenershootcluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=gardenershootclusters,verbs=create;update,versions=v1alpha1,name=vgardenershootcluster-v1alpha1.kb.io,admissionReviewVersions=v1

// GardenerShootClusterCustomValidator struct is responsible for validating the GardenerShootCluster resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type GardenerShootClusterCustomValidator struct {
	GardenerClient client.Client
	Client         client.Client
}

var _ webhook.CustomValidator = &GardenerShootClusterCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type GardenerShootCluster.
func (v *GardenerShootClusterCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, ok := obj.(*infrastructurev1alpha1.GardenerShootCluster)
	if !ok {
		return nil, fmt.Errorf("expected a GardenerShootCluster object but got %T", obj)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type GardenerShootCluster.
func (v *GardenerShootClusterCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	shootCluster, ok := newObj.(*infrastructurev1alpha1.GardenerShootCluster)
	if !ok {
		return nil, fmt.Errorf("expected a GardenerShootCluster object for the newObj but got %T", newObj)
	}

	// For the update, we need to get the actual cluster.
	cluster, err := util.GetOwnerCluster(ctx, v.Client, shootCluster.ObjectMeta)
	if err != nil {
		return nil, fmt.Errorf("could not get owner cluster: %w", err)
	}
	// Unfortunately, we need to get the control plane object to get the typed namespace of the shoot.
	controlPlane := &controlplanev1alpha1.GardenerShootControlPlane{}
	if err := v.Client.Get(ctx, types.NamespacedName{
		Name:      cluster.Spec.ControlPlaneRef.Name,
		Namespace: cluster.Namespace,
	}, controlPlane); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	shoot := &v1beta1.Shoot{}
	if err := v.GardenerClient.Get(ctx, providerutil.ShootNameFromCAPIResources(*cluster, *controlPlane), shoot); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	providerutil.SyncShootSpecFromCluster(shoot, shootCluster)

	// During deletion, it can happen that the Shoot wants to be patched, when it does not exist anymore,
	// therefore ignoring this error to prevent the reconciliation to be blocked.
	return nil, client.IgnoreNotFound(v.GardenerClient.Update(ctx, shoot, &client.UpdateOptions{DryRun: []string{"All"}}))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type GardenerShootCluster.
func (v *GardenerShootClusterCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, ok := obj.(*infrastructurev1alpha1.GardenerShootCluster)
	if !ok {
		return nil, fmt.Errorf("expected a GardenerShootCluster object but got %T", obj)
	}

	return nil, nil
}
