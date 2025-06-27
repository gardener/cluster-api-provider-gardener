# Architecture Overview 🏗️

The provider is developed so it can be used in a plug and play manner against a KCP server.
This means that the provider detects if the `KUBECONFIG` points to a KCP server and if so, it will use the KCP controller manager to run the controller,
whilst still behaving like the standalone provider.

The key difference is that you need to decide where to run the provider, as KCP does not support running workload.

![Image of the provider architecture in the kcp deployment scenario](./architecture.svg)