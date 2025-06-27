# Deployment Guide 📦

This guide provides instructions on how to deploy the Gardener CAPI provider in a real-world scenario for the KCP deployment scenario.

## Prerequisites 🛠️

- ☸️ kubectl version v1.11.3+.
- 🌻 a `kubeconfig` for a Gardener cluster where you want to create clusters in.
- 🗂️ a `kubeconfig` for a KCP server
- 🔌 KCP's `kubectl` plugins

## Deployment 🚀

Generally, the process is similar to the [Getting started locally](./getting-started-locally.md) guide, but let's abstract the steps to be a bit more generic.

**Create provider workspace:**
> [!NOTE]
> `<PROVIDER_WORKSPACE>` is the workspace name you want to use for the provider. 🗂️
> We will later refer to `<PROVIDER_WORKSPACE_PATH>` which is the _full_ path to the provider workspace, e.g. `:root:<PROVIDER_WORKSPACE>`. 📁
```shell
kubectl create workspace <PROVIDER_WORKSPACE> --enter
```

**Create `APIResourceSchema`s, `APIExport` and `APIBinding` in provider-workspace:**
```shell
kubectl apply -f schemas/gardener
```

> [!NOTE]
> `APIBinding.spec.reference.export.path`  needs to be adapted to `<PROVIDER_WORKSPACE_PATH>` `:root:gardener` as your provider-workspace. 🛠️
```shell
kubectl apply -f schemas/binding.yaml
```

**Run controller:**

When you run the controller on a bare-metal machine, you can use the following command to start the controller:
```shell
ENABLE_WEBHOOKS=false go run cmd/main.go --kubeconfig <path/to/kcp-kubeconfig> -gardener-kubeconfig  <path/to/gardener/kubeconfig.yaml>
```

In case you want to run the controller in a container, you can build the image and push it to a registry, then deploy it using a Kubernetes deployment.


## Usage ✨

In order to provision Shoot clusters using the Gardener provider in KCP, you need to create a consuming workspace and bind the API:

**Create and enter consuming workspace:**
```shell
kubectl create workspace <CONSUMIER_WORKSPACE> --enter
```

**Create `APIBinding` for consuming workspace:**
```shell
kubectl apply -f schemas/binding.yaml
```

**Apply `Cluster` resources in consuming workspace:**
```shell
kubectl apply -f config/samples/workerful.yaml
```