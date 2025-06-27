# Deployment Guide 📦

This guide provides instruction on how to deploy the Gardener CAPI provider in a real world scenario.

## Prerequisites 🛠️
- 🦦 go version v1.24.0+
- 🐳 docker version 17.03+.
- ☸️ kubectl version v1.11.3+.
- 🌻 a `kubeconfig` for a Gardener cluster where you want to create clusters in.
- 🐢 a CAPI management cluster, with the `EXP_MACHINE_POOL` feature gate enabled.

## Building and pushing the image 🏗️

> [!IMPORTANT]
> At the moment we do not have container images built yet, so you will need to build and release the images yourself for now. 🏗️
> We are working on getting the images built and released automatically, so stay tuned for updates. 🚀
> 
> This step will become obsolete once we have the images available in a public registry. 📦

To build the image, please refer to the [`Getting started locally`](./getting-started-locally.md) guide, which describes how to build and push the image to a registry.

## Deploying the provider 🚀

> [!NOTE]
> About the `GARDENER_KUBECONFIG` ☸️
> 
> The `GARDENER_KUBECONFIG` is the path to a `kubeconfig` file that points to a Garden cluster where you want to manage `Shoot`s in. 🌱
> Make sure that the `kubeconfig` file has necessary permissions to create and manage `Shoot`s in the projects you desire. 🔐

To deploy the provider, make sure your current `kubectl` context points to the CAPI management cluster, and run the following command:

```sh
make install deploy-prod IMG=<IMAGE_REFERENCE> GARDENER_KUBECONFIG=<path/to/gardener/kubeconfig.yaml>
```

> [!NOTE]
> The `IMAGE_REFERENCE` should point to the image you built and pushed in the previous step. 🐳
