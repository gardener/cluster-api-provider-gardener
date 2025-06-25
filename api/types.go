// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	gardenercorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	apisv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/controlplane/v1alpha1"
	infrastructurev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api/infrastructure/v1alpha1"
)

var (
	// Scheme is the global scheme for the Gardener provider.
	// TODO(tobschli,LucaBernstein): Fine-grain scheme scopes.
	Scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(Scheme))

	utilruntime.Must(clusterv1beta1.AddToScheme(Scheme))
	utilruntime.Must(expclusterv1.AddToScheme(Scheme))
	utilruntime.Must(controlplanev1alpha1.AddToScheme(Scheme))
	utilruntime.Must(infrastructurev1alpha1.AddToScheme(Scheme))

	utilruntime.Must(gardenercorev1beta1.AddToScheme(Scheme))
	utilruntime.Must(kubernetes.AddGardenSchemeToScheme(Scheme))
	utilruntime.Must(apisv1alpha1.AddToScheme(Scheme))
	// +kubebuilder:scaffold:scheme
}
