# Architecture Overview 🏗️

The Gardener CAPI provider for Gardener is a mean to manage Gardener Shoot clusters through the means of Cluster API.
Because Gardener project has a Kubernetes API, which supports the management of K8s clusters for multi-cloud scenarios and their lifecycle,
this provider fulfills many different CAPI contracts throughout the stack, _without_ compatibility to other CAPI providers.
This is because the Gardener already provides a full-fledged API for managing Kubernetes clusters in different cloud environments.

This provider fulfills the following CAPI contracts:
- **[`ControlPlane`](https://cluster-api.sigs.k8s.io/developer/providers/contracts/control-plane)**: `GardenerShootControlPlane`
- **[`InfraCluster`](https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster)**: `GardenerShootCluster`
- **[`MachinePool`](https://cluster-api.sigs.k8s.io/developer/core/controllers/machine-pool)**: `GardenerWorkerPool`

## `ControlPlane` / `InfraCluster` ☁️

Because Gardener is a hosted control plane provider, which abstracts beyond machines, we decided to implement both these contracts and make them dependent on each other.
This aligns to how other hosted control plane providers, e.g. [provider GCP](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/060f142535c1d51724f2884ad4c48b32159f9739/exp/controllers/gcpmanagedcontrolplane_controller.go#L119-L127), implement these contracts as well.

## `MachinePool` 🌱

Whilst the `MachinePool` API not being a contract like the previous two, because of it being a feature not yet being part of the CAPI core,
it was necessary to implement it.

The reason of not implementing the classical `Machine`, `MachineTemplate` etc., contracts, is that Gardener abstracts above `MachineDeployments` in what is called [`Worker`](https://gardener.cloud/docs/gardener/api-reference/core/#core.gardener.cloud/v1beta1.Worker)s.
It depicts a higher-level abstraction than the `MachineDeployment` contract, which is why we decided to implement the `MachinePool` contract instead.

## Translation to Gardener API 🔄
The Gardener CAPI provider basically serves as a translation layer between the CAPI API and the Gardener `Shoot` API.
The `Shoot` API is distributed over the different provider API resources (`GardenerShootControlPlane`, `GardenerShootCluster`, `GardenerWorkerPool`).

> [!IMPORTANT]
> For details on where to find which field, please refer to the [`CustomResourceDefinitions`](../../api) of this provider. 🗂️
> 
> If you are new to the Gardener API, please refer to the [Gardener documentation](https://gardener.cloud/docs). 📚
> 
> In addition to that, we provide sample manifests in the [`examples`](../../config/samples) directory of this repository. ✨

![Image of the translation between Gardener API and Gardener API](./translation.svg)