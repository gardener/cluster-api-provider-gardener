# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

ENSURE_GARDENER_MOD         := $(shell go get github.com/gardener/gardener@$$(go list -m -f "{{.Version}}" github.com/gardener/gardener))
GARDENER_HACK_DIR           := $(shell go list -m -f "{{.Dir}}" github.com/gardener/gardener)/hack
REPO_ROOT                   := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
HACK_DIR                    := $(REPO_ROOT)/hack

# Image URL to use all building/pushing image targets
IMG ?= localhost:5001/cluster-api-provider-gardener/controller:latest
GARDENER_KUBECONFIG ?= ./bin/gardener/example/provider-local/seed-kind/base/kubeconfig
GARDENER_DIR ?= $(shell go list -m -f '{{.Dir}}' github.com/gardener/gardener)

#########################################
# Tools                                 #
#########################################

TOOLS_DIR := $(HACK_DIR)/tools
include $(GARDENER_HACK_DIR)/tools.mk

#################################################################
# Rules related to binary build, Docker image build and release #
#################################################################

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: verify-extended
verify-extended: check lint-config test sast-report ## Generate and reformat code, run tests

.PHONY: manifests
manifests: $(CONTROLLER_GEN) ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	@controller-gen rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./api/...;./cmd/...;./internal/..." output:crd:artifacts:config=config/crd/bases

.PHONY: deepcopy
deepcopy: $(CONTROLLER_GEN) ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	@controller-gen object:headerFile="hack/LICENSE_BOILERPLATE.txt" paths="./api/...;./cmd/...;./internal/..."

.PHONY: generate
generate: manifests deepcopy fmt lint-fix format vet generate-schemas $(YQ) ## Generate and reformat code.
	@./hack/generate-renovate-ignore-deps.sh

.PHONY: generate-schemas
generate-schemas: apigen $(YQ) ## Generate OpenAPI schemas.
	@./hack/generate-schemas.sh ${REPO_ROOT}

.PHONY: check
check: generate sast ## Run generators, formatters and linters and check whether files have been modified.
	@git diff --quiet || ( echo "Files have been modified. Need to run 'make generate'." && exit 1 )

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_BRANCH_NAME) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
.PHONY: test-e2e
test-e2e: $(KIND) ## Run the e2e tests. Expected an isolated environment using Kind.
	@kind get clusters | grep -q 'gardener' || { \
		echo "No Kind cluster is running. Please start a Kind cluster before running the e2e tests."; \
		exit 1; \
	}
	KUBECONFIG=$(GARDENER_KUBECONFIG) CERT_MANAGER_INSTALL_SKIP=true go test ./test/e2e/ -v -ginkgo.v

.PHONY: kind-gardener-up
kind-gardener-up: gardener
	@./hack/kind-gardener-up.sh $(GARDENER)

.PHONY: clusterctl-init
clusterctl-init: clusterctl
	KUBECONFIG=$(GARDENER_KUBECONFIG) EXP_MACHINE_POOL=true $(CLUSTERCTL) init

.PHONY: ci-e2e-kind
ci-e2e-kind: kind-gardener-up clusterctl-init test-e2e

.PHONY: format
format: $(GOIMPORTS) $(GOIMPORTSREVISER) ## Format imports.
	@./hack/format.sh ./api ./cmd ./internal ./test

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint linter
	@golangci-lint run --timeout 10m

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Run golangci-lint linter and perform fixes
	@golangci-lint run --fix --timeout 10m

.PHONY: lint-config
lint-config: $(GOLANGCI_LINT) ## Verify golangci-lint linter configuration
	@golangci-lint config verify

.PHONY: sast
sast: $(GOSEC)
	@bash $(GARDENER_HACK_DIR)/sast.sh --exclude-dirs hack,gardener

.PHONY: sast-report
sast-report: $(GOSEC)
	@bash $(GARDENER_HACK_DIR)/sast.sh --exclude-dirs hack,gardener --gosec-report true

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name cluster-api-provider-gardener-builder
	$(CONTAINER_TOOL) buildx use cluster-api-provider-gardener-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm cluster-api-provider-gardener-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate $(KUSTOMIZE) ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && kustomize edit set image controller=${IMG}
	@kustomize build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests $(KUSTOMIZE) $(KUBECTL) ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@kustomize build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests $(KUSTOMIZE) $(KUBECTL) ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@kustomize build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests $(KUSTOMIZE) envsubst $(KUBECTL) ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	$(eval B64_GARDENER_KUBECONFIG_ENV := $(shell ./hack/gardener-kubeconfig.sh $(GARDENER_KUBECONFIG)))
	@cd config/manager && kustomize edit set image controller=${IMG}
	@kustomize build config/overlays/dev | B64_GARDENER_KUBECONFIG=$(B64_GARDENER_KUBECONFIG_ENV) envsubst | kubectl apply -f -

.PHONY: deploy-prod
deploy-prod: manifests $(KUSTOMIZE) $(KUBECTL) ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	$(eval B64_GARDENER_KUBECONFIG_ENV := $(shell ./hack/gardener-kubeconfig.sh $(GARDENER_KUBECONFIG)))
	@cd config/manager && kustomize edit set image controller=${IMG}
	@kustomize build config/default | B64_GARDENER_KUBECONFIG=$(B64_GARDENER_KUBECONFIG_ENV) envsubst | kubectl apply -f -


.PHONY: deploy-kcp
deploy-kcp: manifests $(KUSTOMIZE) envsubst $(KUBECTL) ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	$(eval B64_GARDENER_KUBECONFIG_ENV := $(shell ./hack/gardener-kubeconfig.sh $(GARDENER_KUBECONFIG)))
	@cd config/manager && @kustomize edit set image controller=${IMG}
	@kustomize build config/overlays/kcp | B64_GARDENER_KUBECONFIG=$(B64_GARDENER_KUBECONFIG_ENV) envsubst | kubectl apply -f -

.PHONY: undeploy
undeploy: $(KUSTOMIZE) $(KUBECTL) ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@kustomize build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

export PATH := $(abspath $(LOCALBIN)):$(PATH)

## Tool Binaries
ENVTEST ?= $(LOCALBIN)/setup-envtest
CLUSTERCTL ?= $(LOCALBIN)/clusterctl
GARDENER ?= $(LOCALBIN)/gardener
APIGEN ?= $(LOCALBIN)/apigen

## Tool Versions
# renovate: datasource=github-releases depName=kubernetes-sigs/controller-runtime
ENVTEST_VERSION ?= v0.20.4
# renovate: datasource=github-tags depName=kubernetes/api
ENVTEST_K8S_VERSION ?= v0.32.6
# renovate: datasource=github-releases depName=kubernetes-sigs/cluster-api
CLUSTERCTL_VERSION ?= v1.9.9
# renovate: datasource=github-releases depName=kcp-dev/kcp
APIGEN_VERSION ?= v0.27.1

# Build ENVTEST release branch names to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_BRANCH_NAME ?= $(shell echo $(ENVTEST_VERSION) | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
ENVTEST_K8S_BRANCH_NAME ?= $(shell echo $(ENVTEST_K8S_VERSION) | awk -F'[v.]' '{printf "1.%d", $$3}')

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_BRANCH_NAME)..."
	@$(ENVTEST) use $(ENVTEST_K8S_BRANCH_NAME) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_BRANCH_NAME)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_BRANCH_NAME))

.PHONY: envsubst
envsubst:
	@which envsubst > /dev/null 2>&1 && echo "Found envsubst: $$(which envsubst)" || \
		{ apt update && apt install -y gettext && echo "Successfully installed gettext for envsubst" || (echo "envsubst is not available. Please install GNU gettext to use envsubst."; exit 1); }

.PHONY: clusterctl
clusterctl: $(CLUSTERCTL) ## Download clusterctl locally if necessary.
$(CLUSTERCTL): $(LOCALBIN)
	$(call go-install-tool,$(CLUSTERCTL),sigs.k8s.io/cluster-api/cmd/clusterctl,$(CLUSTERCTL_VERSION))

.PHONY: gardener
gardener: $(GARDENER) $(GARDENER_DIR) ## Copy gardener locally if necessary.
$(GARDENER): $(LOCALBIN)
	@[ -d $(GARDENER) ] || cp -r $(GARDENER_DIR) $(GARDENER)

.PHONY: apigen
apigen: $(APIGEN) ## Download apigen locally if necessary.
$(APIGEN): $(LOCALBIN)
	$(call go-install-tool,$(APIGEN),github.com/kcp-dev/kcp/sdk/cmd/apigen,$(APIGEN_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
