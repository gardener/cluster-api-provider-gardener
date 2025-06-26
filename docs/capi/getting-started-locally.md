# Getting Started Locally 🚀🌱☁️

The local-setup assumes, that you deploy Cluster-API next to the virtual Garden-Cluster, which is _not_ intended for production. ⚠️

## Prerequisites 🛠️

- 🦦 go version v1.24.0+
- 🐳 docker version 17.03+.
- ☸️ kubectl version v1.11.3+.
- 🌻 a local Gardener cluster
  - Please refer to Gardener's [local setup guide](https://gardener.cloud/docs/gardener/deployment/getting_started_locally/)

You will need a Cluster-API management cluster, with the `EXP_MACHINE_POOL` feature gate enabled.
For this case, you can install it in the local Gardener cluster, which is _not_ intended for production.
You can use the following command to create a management cluster with the `EXP_MACHINE_POOL` feature gate enabled:

```sh
EXP_MACHINE_POOL=true clusterctl init
```

## To Deploy on the cluster 🚢

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=localhost:5001/cluster-api-provider-gardener/controller:latest
```

> [!NOTE] 
> This image ought to be published in the personal registry you specified.
> And it is required to have access to pull the image from the working environment.
> Make sure you have the proper permission to the registry if the above commands don’t work. 🔐

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

> [!NOTE] 
> This target assumes that you are running a local Gardener. 🌱
>
> For production environments, do not use the `config/overlays/dev` kustomization. 🚫

```sh
make deploy IMG=localhost:5001/cluster-api-provider-gardener/controller:latest GARDENER_KUBECONFIG=<path/to/gardener/kubeconfig.yaml>
```

**Create instances of your solution** ✨
You can apply one of the samples from the `config/samples`:

> [!NOTE]
> When using this in a local deployment, this works, but just with the name of `hello-gardener` in the namespace `default`. 🌸
>
> This is because of the special setup of Gardener and Cluster-API in the same cluster.
> For details of this setup, see `config/overlays/dev`.

Let's create a simple `Shoot` cluster: 🌱
```sh
kubectl apply -k config/samples/workerful
```

## To Uninstall 🧹

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
