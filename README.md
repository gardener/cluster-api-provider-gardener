# cluster-api-provider-gardener

Kubernetes-native declarative infrastructure for Gardener Shoots.

## Description
The `cluster-api-provider-gardener` integrates Gardener with Cluster API, enabling the management of Kubernetes clusters
using Gardener as the control plane provider.
This provider allows users to leverage the powerful features of Gardener for cluster lifecycle management.

> [!IMPORTANT]
> **Experimental:** CAPGa is currently experimental and therefore may change or be unstable. Use with caution.

The controller is also KCP-aware, meaning that it can also be used in KCP as a KCP-controller.

## [Documentation](./docs/README.md)
### [Getting Started - CAPI](./docs/capi/README.md)
### [Getting Started - KCP](./docs/kcp/README.md)
