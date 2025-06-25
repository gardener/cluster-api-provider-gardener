# cluster-api-provider-gardener

Kubernetes-native declarative infrastructure for Gardener Shoots.

## Description
The `cluster-api-provider-gardener` integrates Gardener with Cluster API, enabling the management of Kubernetes clusters
using Gardener as the control plane provider.
This provider allows users to leverage the powerful features of Gardener for cluster lifecycle management.

The controller is also KCP-aware, meaning that it can also be used in KCP as a KCP-controller.

## Getting Started

### Common Prerequisites

The following prerequisites apply to all the following deployment-scenarios.
Please refer to the individual scenario of your choice for the specific prerequisites.

- go version v1.24.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.
- a (local) Gardener cluster

### KCP

#### Prerequisites

- a KCP server (`KUBECONFIG` usually is located in `.kcp/admin.kubeconfig`, relative from where KCP is started)
- KCP's `kubectl` plugins

#### To Deploy on the cluster

**Create controller workspace:**
> **NOTE**: For our quick-start, we use `:root:gardener` as our controller-workspace.
```shell
kubectl create workspace gardener --enter
```

**Create `APIResourceSchema`s, `APIExport` and `APIBinding` in Controller-workspace:**
```shell
kubectl apply -f schemas/gardener
```

> **NOTE**: `APIBinding.spec.reference.export.path` may needs to be adapted when you don't use `:root:gardener` as your controller-workspace.
```shell
kubectl apply -f schemas/binding.yaml
```

**Run controller:**
```shell
ENABLE_WEBHOOKS=false go run cmd/main.go --kubeconfig <path/to/kcp-kubeconfig> -gardener-kubeconfig  <path/to/gardener/kubeconfig.yaml>
```

**Create and enter consuming workspace:**
```shell
kubectl create workspace test --enter
```

**Create `APIBinding` for consuming workspace:**
```shell
kubectl apply -f schemas/binding.yaml
```

**Apply `Cluster` resources in consuming workspace:**
```shell
kubectl apply -f config/samples/workerful.yaml
```

### Cluster-API

The local-setup assumes, that you deploy Cluster-API next to the virtual Garden-Cluster, which is _not_ intended for production.

#### Prerequisites

You will need a Cluster-API management cluster, with the `EXP_MACHINE_POOL` feature gate enabled.
You can use the following command to create a management cluster with the `EXP_MACHINE_POOL` feature gate enabled:

```sh
EXP_MACHINE_POOL=true clusterctl init
```

#### To Deploy on the cluster

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=localhost:5001/cluster-api-provider-gardener/controller:latest
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

> **NOTE**: This target assumes that you are running a local Gardener.
>
> For production environments, do not use the `config/overlays/dev` kustomization.

```sh
make deploy IMG=localhost:5001/cluster-api-provider-gardener/controller:latest GARDENER_KUBECONFIG=<path/to/gardener/kubeconfig.yaml>
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

> **NOTE**: When using this in a local deployment, this works, but just with the name of `hello-gardener` in the namespace `default`.
>
> This is because of the special setup of Gardener and Cluster-API in the same cluster.
> For details of this setup, see `config/overlays/dev`.

```sh
kubectl apply -k config/samples/
```

#### To Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=localhost:5001/cluster-api-provider-gardener/controller:latest
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/lucabernstein/cluster-api-provider-gardener/main/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
   can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

### Running E2E Tests Locally

```bash
make kind-gardener-up clusterctl-init

make test-e2e
```
