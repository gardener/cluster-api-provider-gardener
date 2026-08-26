// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/controlplane/v1alpha1"
	providerutil "github.com/gardener/cluster-api-provider-gardener/internal/util"
)

// nolint:unused
// log is for logging in this package.
var _ = logf.Log.WithName("gardenershootcontrolplane-resource")

// SetupGardenerShootControlPlaneWebhookWithManager registers the webhook for GardenerShootControlPlane in the manager.
func SetupGardenerShootControlPlaneWebhookWithManager(mgr ctrl.Manager, gardenerClient client.Client) error {
	return ctrl.NewWebhookManagedBy(mgr, &controlplanev1alpha1.GardenerShootControlPlane{}).
		WithValidator(&GardenerShootControlPlaneCustomValidator{
			GardenerClient: gardenerClient,
			Client:         mgr.GetClient(),
		}).
		WithDefaulter(&GardenerShootControlPlaneCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1alpha1-gardenershootcontrolplane,mutating=true,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=gardenershootcontrolplanes,verbs=create;update,versions=v1alpha1,name=vgardenershootcontrolplane-v1alpha1.kb.io,admissionReviewVersions=v1

// GardenerShootControlPlaneCustomDefaulter struct is responsible for defaulting the GardenerShootControlPlane resource.
type GardenerShootControlPlaneCustomDefaulter struct {
	GardenerClient client.Client
}

var _ admission.Defaulter[*controlplanev1alpha1.GardenerShootControlPlane] = &GardenerShootControlPlaneCustomDefaulter{}

// Default implements admission.Defaulter so a webhook will be registered for the type GardenerShootControlPlane.
func (d GardenerShootControlPlaneCustomDefaulter) Default(_ context.Context, shootControlPlane *controlplanev1alpha1.GardenerShootControlPlane) error {
	if len(shootControlPlane.Spec.ProjectNamespace) == 0 {
		shootControlPlane.Spec.ProjectNamespace = shootControlPlane.Namespace
	}

	return nil
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1alpha1-gardenershootcontrolplane,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=gardenershootcontrolplanes,verbs=create;update,versions=v1alpha1,name=vgardenershootcontrolplane-v1alpha1.kb.io,admissionReviewVersions=v1

// GardenerShootControlPlaneCustomValidator struct is responsible for validating the GardenerShootControlPlane resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type GardenerShootControlPlaneCustomValidator struct {
	GardenerClient client.Client
	Client         client.Client
}

var _ admission.Validator[*controlplanev1alpha1.GardenerShootControlPlane] = &GardenerShootControlPlaneCustomValidator{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type GardenerShootControlPlane.
func (v *GardenerShootControlPlaneCustomValidator) ValidateCreate(_ context.Context, _ *controlplanev1alpha1.GardenerShootControlPlane) (admission.Warnings, error) {
	// Do not validate anything here, as the shoot does not exist, and all CAPI resources need to be put together to
	// initially create the shoot spec.
	return nil, nil
}

// ValidateUpdate implements admission.Validator so a webhook will be registered for the type GardenerShootControlPlane.
func (v *GardenerShootControlPlaneCustomValidator) ValidateUpdate(ctx context.Context, _, shootControlPlane *controlplanev1alpha1.GardenerShootControlPlane) (admission.Warnings, error) {
	// For the update, we need to get the actual cluster and inject the new config, because e.g. the resourceVersion must be set.
	cluster, err := util.GetOwnerCluster(ctx, v.Client, shootControlPlane.ObjectMeta)
	if err != nil {
		return nil, err
	}
	shoot := &gardenercorev1beta1.Shoot{}
	if err := v.GardenerClient.Get(ctx, providerutil.ShootNameFromCAPIResources(*cluster, *shootControlPlane), shoot); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	providerutil.SyncShootSpecFromGSCP(shoot, shootControlPlane)

	// During deletion, it can happen that the Shoot wants to be patched, when it does not exist anymore,
	// therefore ignoring this error to prevent the reconciliation to be blocked.
	return nil, client.IgnoreNotFound(v.GardenerClient.Update(ctx, shoot, &client.UpdateOptions{DryRun: []string{"All"}}))
}

// ValidateDelete implements admission.Validator so a webhook will be registered for the type GardenerShootControlPlane.
func (v *GardenerShootControlPlaneCustomValidator) ValidateDelete(_ context.Context, _ *controlplanev1alpha1.GardenerShootControlPlane) (admission.Warnings, error) {
	return nil, nil
}
