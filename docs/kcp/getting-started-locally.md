# Getting Started Locally 🚀🌱

## Prerequisites 🛠️

- 🦦 go version v1.24.0+
- 🐳 docker version 17.03+.
- ☸️ kubectl version v1.11.3+.
- 🌻 a local Gardener cluster
- 🗂️ a KCP server (`KUBECONFIG` usually is located in `.kcp/admin.kubeconfig`, relative from where KCP is started)
- 🔌 KCP's `kubectl` plugins

## To Deploy on the cluster 🚢

**Create provider workspace:**
> [!NOTE]
> For our quick-start, we use `:root:gardener` as our provider-workspace. 🗂️
```shell
kubectl create workspace gardener --enter
```

**Create `APIResourceSchema`s, `APIExport` and `APIBinding` in provider-workspace:**
```shell
kubectl apply -f schemas/gardener
```

> [!NOTE]
> `APIBinding.spec.reference.export.path` may need to be adapted when you don't use `:root:gardener` as your provider-workspace. 🛠️
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