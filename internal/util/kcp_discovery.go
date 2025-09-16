/*
Copyright 2024 The KCP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Modifications Copyright SAP SE or an SAP affiliate company and Gardener contributors
*/

package util

import (
	"context"
	"fmt"

	apisv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1alpha1 "github.com/gardener/cluster-api-provider-gardener/api"
)

// RestConfigForLogicalClusterHostingAPIExport returns a *rest.Config properly configured
// to communicate with the endpoint for the APIExport's virtual workspace.
func RestConfigForLogicalClusterHostingAPIExport(
	ctx context.Context, cfg *rest.Config, apiExportName string) (*rest.Config, error) {
	apiExportClient, err := client.New(cfg, client.Options{
		Scheme: controlplanev1alpha1.Scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating APIExport client: %w", err)
	}

	apiExportEndpointSlice := &apisv1alpha1.APIExportEndpointSlice{}
	if err := apiExportClient.Get(ctx, types.NamespacedName{Name: apiExportName}, apiExportEndpointSlice); err != nil {
		return nil, fmt.Errorf("error getting APIExport %q: %w", apiExportName, err)
	}
	if len(apiExportEndpointSlice.Status.APIExportEndpoints) < 1 { // nolint:staticcheck
		return nil, fmt.Errorf("APIExportEndpointSlice %q status.endpoints is empty", apiExportName)
	}

	// create a new rest.Config with the APIExport's virtual workspace URL
	exportConfig := rest.CopyConfig(cfg)
	exportConfig.Host = apiExportEndpointSlice.Status.APIExportEndpoints[0].URL // nolint:staticcheck

	return exportConfig, nil
}

// HasKcpAPIGroups checks if the KCP API groups are available in a given cluster.
func HasKcpAPIGroups(cfg *rest.Config) (bool, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return false, fmt.Errorf("failed to create discovery client: %w", err)
	}
	apiGroupList, err := discoveryClient.ServerGroups()
	if err != nil {
		return false, fmt.Errorf("failed to get server groups: %w", err)
	}

	for _, group := range apiGroupList.Groups {
		if group.Name == apisv1alpha1.SchemeGroupVersion.Group {
			for _, version := range group.Versions {
				if version.Version == apisv1alpha1.SchemeGroupVersion.Version {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
