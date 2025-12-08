// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
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
var gardenerworkerpoollog = logf.Log.WithName("gardenerworkerpool-resource")

// SetupGardenerWorkerPoolWebhookWithManager registers the webhook for GardenerWorkerPool in the manager.
func SetupGardenerWorkerPoolWebhookWithManager(mgr ctrl.Manager, gardenerClient client.Client) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&infrastructurev1alpha1.GardenerWorkerPool{}).
		WithValidator(&GardenerWorkerPoolCustomValidator{
			Client:         mgr.GetClient(),
			GardenerClient: gardenerClient,
		}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-gardenerworkerpool,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=gardenerworkerpools,verbs=create;update,versions=v1alpha1,name=vgardenerworkerpool-v1alpha1.kb.io,admissionReviewVersions=v1

// GardenerWorkerPoolCustomValidator struct is responsible for validating the GardenerWorkerPool resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type GardenerWorkerPoolCustomValidator struct {
	Client         client.Client
	GardenerClient client.Client
}

var _ webhook.CustomValidator = &GardenerWorkerPoolCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type GardenerWorkerPool.
func (v *GardenerWorkerPoolCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, ok := obj.(*infrastructurev1alpha1.GardenerWorkerPool)
	if !ok {
		return nil, fmt.Errorf("expected a GardenerWorkerPool object but got %T", obj)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type GardenerWorkerPool.
func (v *GardenerWorkerPoolCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	workerPool, ok := newObj.(*infrastructurev1alpha1.GardenerWorkerPool)
	if !ok {
		return nil, fmt.Errorf("expected a GardenerWorkerPool object for the newObj but got %T", newObj)
	}

	machinePool, err := providerutil.GetMachinePoolForWorkerPool(ctx, v.Client, workerPool)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine pool for worker pool: %w", err)
	}
	if machinePool == nil {
		return nil, fmt.Errorf("machine pool not found for worker pool %s/%s", workerPool.Namespace, workerPool.Name)
	}

	cluster, err := util.GetOwnerCluster(ctx, v.Client, machinePool.ObjectMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner cluster: %w", err)
	}
	if cluster == nil {
		return nil, nil
	}

	controlPlane := &controlplanev1alpha1.GardenerShootControlPlane{}
	if err := v.Client.Get(ctx, client.ObjectKey{Name: cluster.Spec.ControlPlaneRef.Name, Namespace: cluster.Namespace}, controlPlane); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	shoot := &v1beta1.Shoot{}
	if err := v.GardenerClient.Get(ctx, providerutil.ShootNameFromCAPIResources(*cluster, *controlPlane), shoot); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	providerutil.SyncShootSpecFromWorkerPool(shoot, workerPool)

	return nil, client.IgnoreNotFound(v.GardenerClient.Update(ctx, shoot, &client.UpdateOptions{DryRun: []string{"All"}}))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type GardenerWorkerPool.
func (v *GardenerWorkerPoolCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, ok := obj.(*infrastructurev1alpha1.GardenerWorkerPool)
	if !ok {
		return nil, fmt.Errorf("expected a GardenerWorkerPool object but got %T", obj)
	}

	return nil, nil
}
