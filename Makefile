# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# If you update this file, please follow
# https://suva.sh/posts/well-documented-makefiles

.DEFAULT_GOAL:=help

# Use GOPROXY environment variable if set
GOPROXY := $(shell go env GOPROXY)
ifeq ($(GOPROXY),)
GOPROXY := https://proxy.golang.org
endif
export GOPROXY

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE=on

# Default timeout for starting/stopping the Kubebuilder test control plane
export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?=60s
export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?=60s

# This option is for running docker manifest command
export DOCKER_CLI_EXPERIMENTAL := enabled

# Directories.
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin
BIN_DIR := bin
E2E_FRAMEWORK_DIR := test/framework
CAPD_DIR := test/infrastructure/docker
RELEASE_NOTES_BIN := bin/release-notes
RELEASE_NOTES := $(TOOLS_DIR)/$(RELEASE_NOTES_BIN)

# Binaries.
KUSTOMIZE := $(TOOLS_BIN_DIR)/kustomize
CONTROLLER_GEN := $(TOOLS_BIN_DIR)/controller-gen
GOLANGCI_LINT := $(TOOLS_BIN_DIR)/golangci-lint
CONVERSION_GEN := $(TOOLS_BIN_DIR)/conversion-gen

# Bindata.
GOBINDATA := $(TOOLS_BIN_DIR)/go-bindata
GOBINDATA_CLUSTERCTL_DIR := cmd/clusterctl/config
CERTMANAGER_COMPONENTS_GENERATED_FILE := cert-manager.yaml

# Define Docker related variables. Releases should modify and double check these vars.
REGISTRY ?= gcr.io/$(shell gcloud config get-value project)
STAGING_REGISTRY ?= gcr.io/k8s-staging-cluster-api
PROD_REGISTRY ?= us.gcr.io/k8s-artifacts-prod/cluster-api

# core
IMAGE_NAME ?= cluster-api-controller
CONTROLLER_IMG ?= $(REGISTRY)/$(IMAGE_NAME)

# bootstrap
KUBEADM_BOOTSTRAP_IMAGE_NAME ?= kubeadm-bootstrap-controller
KUBEADM_BOOTSTRAP_CONTROLLER_IMG ?= $(REGISTRY)/$(KUBEADM_BOOTSTRAP_IMAGE_NAME)

# control plane
KUBEADM_CONTROL_PLANE_IMAGE_NAME ?= kubeadm-control-plane-controller
KUBEADM_CONTROL_PLANE_CONTROLLER_IMG ?= $(REGISTRY)/$(KUBEADM_CONTROL_PLANE_IMAGE_NAME)

TAG ?= dev
ARCH ?= amd64
ALL_ARCH = amd64 arm arm64 ppc64le s390x

# Allow overriding the imagePullPolicy
PULL_POLICY ?= Always

# Hosts running SELinux need :z added to volume mounts
SELINUX_ENABLED := $(shell cat /sys/fs/selinux/enforce 2> /dev/null || echo 0)

ifeq ($(SELINUX_ENABLED),1)
  DOCKER_VOL_OPTS?=:z
endif

# Set build time variables including version details
LDFLAGS := $(shell hack/version.sh)

all: test manager clusterctl

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-45s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Testing
## --------------------------------------

.PHONY: test
test: ## Run tests
	go test -v ./...

.PHONY: test-integration
test-integration: ## Run integration tests
	go test -v -tags=integration ./test/integration/...

.PHONY: test-e2e
test-e2e: ## Run e2e tests
	PULL_POLICY=IfNotPresent $(MAKE) docker-build
	$(MAKE) generate-manifests
	$(MAKE) release-manifests
	cd ./test/e2e; MANAGER_IMAGE=$(CONTROLLER_IMG)-$(ARCH):$(TAG) go test -v -tags=e2e -timeout=1h . -args -ginkgo.v -ginkgo.trace

## --------------------------------------
## Binaries
## --------------------------------------

.PHONY: manager-core
manager-core: ## Build core manager binary
	go build -o $(BIN_DIR)/manager sigs.k8s.io/cluster-api

.PHONY: manager-kubeadm-bootstrap
manager-kubeadm-bootstrap: ## Build kubeadm bootstrap manager
	go build -o $(BIN_DIR)/kubeadm-bootstrap-manager sigs.k8s.io/cluster-api/bootstrap/kubeadm

.PHONY: manager-kubeadm-control-plane
manager-kubeadm-control-plane: ## Build kubeadm control plane manager
	go build -o $(BIN_DIR)/kubeadm-control-plane-manager sigs.k8s.io/cluster-api/controlplane/kubeadm

.PHONY: managers
managers: ## Build all managers
	$(MAKE) manager-core
	$(MAKE) manager-kubeadm-bootstrap
	$(MAKE) manager-kubeadm-control-plane

.PHONY: clusterctl
clusterctl: ## Build clusterctl binary
	go build -ldflags "$(LDFLAGS)" -o bin/clusterctl sigs.k8s.io/cluster-api/cmd/clusterctl

$(KUSTOMIZE): $(TOOLS_DIR)/go.mod # Build kustomize from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/kustomize sigs.k8s.io/kustomize/kustomize/v3

$(CONTROLLER_GEN): $(TOOLS_DIR)/go.mod # Build controller-gen from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/controller-gen sigs.k8s.io/controller-tools/cmd/controller-gen

$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod # Build golangci-lint from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint

$(CONVERSION_GEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/conversion-gen k8s.io/code-generator/cmd/conversion-gen

$(GOBINDATA): $(TOOLS_DIR)/go.mod # Build go-bindata from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/go-bindata github.com/jteeuwen/go-bindata/go-bindata

$(RELEASE_NOTES) : $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR) && go build -tags=tools -o $(RELEASE_NOTES_BIN) ./release

.PHONY: e2e-framework
e2e-framework: ## Builds the CAPI e2e framework
	cd $(E2E_FRAMEWORK_DIR); go build ./...

## --------------------------------------
## Linting
## --------------------------------------

.PHONY: lint lint-full
lint: $(GOLANGCI_LINT) ## Lint codebase
	$(GOLANGCI_LINT) run -v

lint-full: $(GOLANGCI_LINT) ## Run slower linters to detect possible issues
	$(GOLANGCI_LINT) run -v --fast=false

## --------------------------------------
## Generate / Manifests
## --------------------------------------

.PHONY: generate
generate: ## Generate code
	$(MAKE) generate-manifests
	$(MAKE) generate-go
	$(MAKE) generate-bindata

.PHONY: generate-go
generate-go: ## Runs Go related generate targets
	$(MAKE) generate-go-core
	$(MAKE) generate-go-kubeadm-bootstrap
	$(MAKE) generate-go-kubeadm-control-plane

.PHONY: generate-go-core
generate-go-core: $(CONTROLLER_GEN) $(CONVERSION_GEN)
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./api/... \
		paths=./cmd/clusterctl/...
	$(CONVERSION_GEN) \
		--input-dirs=./api/v1alpha2 \
		--output-file-base=zz_generated.conversion \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt

.PHONY: generate-go-kubeadm-bootstrap
generate-go-kubeadm-bootstrap: $(CONTROLLER_GEN) $(CONVERSION_GEN) ## Runs Go related generate targets for the kubeadm bootstrapper
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./bootstrap/kubeadm/api/...
	$(CONVERSION_GEN) \
		--input-dirs=./bootstrap/kubeadm/api/v1alpha2 \
		--output-file-base=zz_generated.conversion \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt

.PHONY: generate-go-kubeadm-control-plane
generate-go-kubeadm-control-plane: $(CONTROLLER_GEN) $(CONVERSION_GEN) ## Runs Go related generate targets for the kubeadm control plane
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./controlplane/kubeadm/api/...

.PHONY: generate-bindata
generate-bindata: $(KUSTOMIZE) $(GOBINDATA) clean-bindata ## Generate code for embedding the clusterctl api manifest
	# Package manifest YAML into a single file.
	mkdir -p $(GOBINDATA_CLUSTERCTL_DIR)/manifest/
	$(KUSTOMIZE) build $(GOBINDATA_CLUSTERCTL_DIR)/crd > $(GOBINDATA_CLUSTERCTL_DIR)/manifest/clusterctl-api.yaml
	# Fetch the cert-manager manifest
	curl -sL https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml > "$(GOBINDATA_CLUSTERCTL_DIR)/manifest/${CERTMANAGER_COMPONENTS_GENERATED_FILE}"
	# Generate go-bindata, add boilerplate, then cleanup.
	$(GOBINDATA) -pkg=config -o=$(GOBINDATA_CLUSTERCTL_DIR)/zz_generated.bindata.go $(GOBINDATA_CLUSTERCTL_DIR)/manifest/
	cat ./hack/boilerplate/boilerplate.generatego.txt $(GOBINDATA_CLUSTERCTL_DIR)/zz_generated.bindata.go > $(GOBINDATA_CLUSTERCTL_DIR)/manifest/manifests.go
	cp $(GOBINDATA_CLUSTERCTL_DIR)/manifest/manifests.go $(GOBINDATA_CLUSTERCTL_DIR)/zz_generated.bindata.go
	# Cleanup the manifest folder.
	$(MAKE) clean-bindata

.PHONY: generate-manifests
generate-manifests: ## Generate manifests e.g. CRD, RBAC etc.
	$(MAKE) generate-core-manifests
	$(MAKE) generate-kubeadm-bootstrap-manifests
	$(MAKE) generate-kubeadm-control-plane-manifests

.PHONY: generate-core-manifests
generate-core-manifests: $(CONTROLLER_GEN) ## Generate manifests for the core provider e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) \
		paths=./api/... \
		paths=./controllers/... \
		crd:preserveUnknownFields=false \
		rbac:roleName=manager-role \
		output:crd:dir=./config/crd/bases \
		output:webhook:dir=./config/webhook \
		webhook
	$(CONTROLLER_GEN) \
		paths=./cmd/clusterctl/api/... \
		crd:trivialVersions=true,preserveUnknownFields=false \
		output:crd:dir=./cmd/clusterctl/config/crd/bases
	## Copy files in CI folders.
	cp -f ./config/rbac/*.yaml ./config/ci/rbac/
	cp -f ./config/manager/manager*.yaml ./config/ci/manager/

.PHONY: generate-kubeadm-bootstrap-manifests
generate-kubeadm-bootstrap-manifests: $(CONTROLLER_GEN) ## Generate manifests for the kubeadm bootstrap provider e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) \
		paths=./bootstrap/kubeadm/api/... \
		paths=./bootstrap/kubeadm/controllers/... \
		crd:trivialVersions=false,preserveUnknownFields=false \
		rbac:roleName=manager-role \
		output:crd:dir=./bootstrap/kubeadm/config/crd/bases \
		output:rbac:dir=./bootstrap/kubeadm/config/rbac \
		output:webhook:dir=./bootstrap/kubeadm/config/webhook \
		webhook

.PHONY: generate-kubeadm-control-plane-manifests
generate-kubeadm-control-plane-manifests: $(CONTROLLER_GEN) ## Generate manifests for the kubeadm control plane provider e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) \
		paths=./controlplane/kubeadm/api/... \
		paths=./controlplane/kubeadm/controllers/... \
		crd:preserveUnknownFields=false \
		rbac:roleName=manager-role \
		output:crd:dir=./controlplane/kubeadm/config/crd/bases \
		output:rbac:dir=./controlplane/kubeadm/config/rbac \
		output:webhook:dir=./controlplane/kubeadm/config/webhook \
		webhook

.PHONY: modules
modules: ## Runs go mod to ensure modules are up to date.
	go mod tidy
	cd $(TOOLS_DIR); go mod tidy
	cd $(E2E_FRAMEWORK_DIR); go mod tidy
	cd test/e2e; go mod tidy
	cd $(CAPD_DIR); $(MAKE) modules

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-build
docker-build: ## Build the docker images for controller managers
	$(MAKE) ARCH=$(ARCH) docker-build-core
	$(MAKE) ARCH=$(ARCH) docker-build-kubeadm-bootstrap
	$(MAKE) ARCH=$(ARCH) docker-build-kubeadm-control-plane

.PHONY: docker-build-core
docker-build-core: ## Build the docker image for core controller manager
	docker build --pull --build-arg ARCH=$(ARCH) . -t $(CONTROLLER_IMG)-$(ARCH):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./config/core/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./config/core/manager_pull_policy.yaml"

.PHONY: docker-build-kubeadm-bootstrap
docker-build-kubeadm-bootstrap: ## Build the docker image for kubeadm bootstrap controller manager
	docker build --pull --build-arg ARCH=$(ARCH) --build-arg package=./bootstrap/kubeadm . -t $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: docker-build-kubeadm-control-plane
docker-build-kubeadm-control-plane: ## Build the docker image for kubeadm control plane controller manager
	docker build --pull --build-arg ARCH=$(ARCH) --build-arg package=./controlplane/kubeadm . -t $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: docker-push
docker-push: ## Push the docker images
	docker push $(CONTROLLER_IMG)-$(ARCH):$(TAG)
	docker push $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH):$(TAG)
	docker push $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH):$(TAG)

## --------------------------------------
## Docker — All ARCH
## --------------------------------------

.PHONY: docker-build-all ## Build all the architecture docker images
docker-build-all: $(addprefix docker-build-,$(ALL_ARCH))

docker-build-%:
	$(MAKE) ARCH=$* docker-build

.PHONY: docker-push-all ## Push all the architecture docker images
docker-push-all: $(addprefix docker-push-,$(ALL_ARCH))
	$(MAKE) docker-push-core-manifest
	$(MAKE) docker-push-kubeadm-bootstrap-manifest
	$(MAKE) docker-push-kubeadm-control-plane-manifest

docker-push-%:
	$(MAKE) ARCH=$* docker-push

.PHONY: docker-push-core-manifest
docker-push-core-manifest: ## Push the fat manifest docker image for the core image.
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CONTROLLER_IMG}:${TAG} ${CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./config/core/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./config/core/manager_pull_policy.yaml"

.PHONY: docker-push-kubeadm-bootstrap-manifest
docker-push-kubeadm-bootstrap-manifest: ## Push the fat manifest docker image for the kubeadm bootstrap image.
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${KUBEADM_BOOTSTRAP_CONTROLLER_IMG}:${TAG} ${KUBEADM_BOOTSTRAP_CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: docker-push-kubeadm-control-plane-manifest
docker-push-kubeadm-control-plane-manifest: ## Push the fat manifest docker image for the kubeadm control plane image.
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${KUBEADM_CONTROL_PLANE_CONTROLLER_IMG}:${TAG} ${KUBEADM_CONTROL_PLANE_CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: set-manifest-pull-policy
set-manifest-pull-policy:
	$(info Updating kustomize pull policy file for manager resources)
	sed -i'' -e 's@imagePullPolicy: .*@imagePullPolicy: '"$(PULL_POLICY)"'@' $(TARGET_RESOURCE)

.PHONY: set-manifest-image
set-manifest-image:
	$(info Updating kustomize image patch file for manager resource)
	sed -i'' -e 's@image: .*@image: '"${MANIFEST_IMG}:$(MANIFEST_TAG)"'@' $(TARGET_RESOURCE)

## --------------------------------------
## Release
## --------------------------------------

RELEASE_TAG := $(shell git describe --abbrev=0 2>/dev/null)
RELEASE_DIR := out

$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)/

.PHONY: release
release: clean-release ## Builds and push container images using the latest git tag for the commit.
	@if [ -z "${RELEASE_TAG}" ]; then echo "RELEASE_TAG is not set"; exit 1; fi
	@if ! [ -z "$$(git status --porcelain)" ]; then echo "Your local git repository contains uncommitted changes, use git clean before proceeding."; exit 1; fi
	git checkout "${RELEASE_TAG}"
	# Set the core manifest image to the production bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(PROD_REGISTRY)/$(IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./config/core/manager_image_patch.yaml"
	# Set the kubeadm bootstrap image to the production bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(PROD_REGISTRY)/$(KUBEADM_BOOTSTRAP_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_image_patch.yaml"
	# Set the kubeadm control plane image to the production bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(PROD_REGISTRY)/$(KUBEADM_CONTROL_PLANE_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./config/core/manager_pull_policy.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_pull_policy.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_pull_policy.yaml"
	$(MAKE) release-manifests
	$(MAKE) release-binaries

.PHONY: release-manifests
release-manifests: $(RELEASE_DIR) $(KUSTOMIZE) ## Builds the manifests to publish with a release
	$(KUSTOMIZE) build config/core > $(RELEASE_DIR)/core-components.yaml
	$(KUSTOMIZE) build bootstrap/kubeadm/config/default > $(RELEASE_DIR)/bootstrap-components.yaml
	$(KUSTOMIZE) build controlplane/kubeadm/config/default > $(RELEASE_DIR)/control-plane-components.yaml
	cat $(RELEASE_DIR)/core-components.yaml > $(RELEASE_DIR)/cluster-api-components.yaml
	echo "---" >> $(RELEASE_DIR)/cluster-api-components.yaml
	cat $(RELEASE_DIR)/bootstrap-components.yaml >> $(RELEASE_DIR)/cluster-api-components.yaml
	echo "---" >> $(RELEASE_DIR)/cluster-api-components.yaml
	cat $(RELEASE_DIR)/control-plane-components.yaml >> $(RELEASE_DIR)/cluster-api-components.yaml

release-binaries: ## Builds the binaries to publish with a release
	RELEASE_BINARY=./cmd/clusterctl GOOS=linux GOARCH=amd64 $(MAKE) release-binary
	RELEASE_BINARY=./cmd/clusterctl GOOS=darwin GOARCH=amd64 $(MAKE) release-binary

release-binary: $(RELEASE_DIR)
	docker run \
		--rm \
		-e CGO_ENABLED=0 \
		-e GOOS=$(GOOS) \
		-e GOARCH=$(GOARCH) \
		-v "$$(pwd):/workspace$(DOCKER_VOL_OPTS)" \
		-w /workspace \
		golang:1.13.5 \
		go build -a -ldflags '-extldflags "-static"' \
		-o $(RELEASE_DIR)/$(notdir $(RELEASE_BINARY))-$(GOOS)-$(GOARCH) $(RELEASE_BINARY)

.PHONY: release-staging
release-staging: ## Builds and push container images to the staging bucket.
	REGISTRY=$(STAGING_REGISTRY) $(MAKE) docker-build-all docker-push-all release-alias-tag

RELEASE_ALIAS_TAG=$(PULL_BASE_REF)

.PHONY: release-alias-tag
release-alias-tag: ## Adds the tag to the last build tag.
	gcloud container images add-tag $(CONTROLLER_IMG):$(TAG) $(CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG) $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(TAG) $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)

.PHONY: release-notes
release-notes: $(RELEASE_NOTES)  ## Generates a release notes template to be used with a release.
	$(RELEASE_NOTES)

## --------------------------------------
## Docker - Example Provider
## --------------------------------------

EXAMPLE_PROVIDER_IMG ?= $(REGISTRY)/example-provider-controller

.PHONY: docker-build-example-provider
docker-build-example-provider: generate lint-full ## Build the docker image for example provider
	docker build --pull --build-arg ARCH=$(ARCH) . -f ./cmd/example-provider/Dockerfile -t $(EXAMPLE_PROVIDER_IMG)-$(ARCH):$(TAG)
	sed -i'' -e 's@image: .*@image: '"${EXAMPLE_PROVIDER_IMG}-$(ARCH):$(TAG)"'@' ./config/ci/manager_image_patch.yaml

## --------------------------------------
## Cleanup / Verification
## --------------------------------------

.PHONY: clean
clean: ## Remove all generated files
	$(MAKE) clean-bin
	$(MAKE) clean-book

.PHONY: clean-bin
clean-bin: ## Remove all generated binaries
	rm -rf bin
	rm -rf hack/tools/bin

.PHONY: clean-release
clean-release: ## Remove the release folder
	rm -rf $(RELEASE_DIR)

.PHONY: clean-book
clean-book: ## Remove all generated GitBook files
	rm -rf ./docs/book/_book

.PHONY: clean-bindata
clean-bindata: ## Remove bindata generated folder
	rm -rf $(GOBINDATA_CLUSTERCTL_DIR)/manifest

.PHONY: format-tiltfile
format-tiltfile: ## Format Tiltfile
	./hack/verify-starlark.sh fix

.PHONY: verify
verify:
	./hack/verify-boilerplate.sh
	./hack/verify-doctoc.sh
	./hack/verify-shellcheck.sh
	./hack/verify-starlark.sh
	$(MAKE) verify-modules
	$(MAKE) verify-gen
	$(MAKE) verify-docker-provider

.PHONY: verify-modules
verify-modules: modules
	@if !(git diff --quiet HEAD -- go.sum go.mod hack/tools/go.mod hack/tools/go.sum); then \
		echo "go module files are out of date"; exit 1; \
	fi

.PHONY: verify-gen
verify-gen: generate
	@if !(git diff --quiet HEAD); then \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

.PHONY: verify-docker-provider
verify-docker-provider:
	@echo "Verifying CAPD"
	cd $(CAPD_DIR); $(MAKE) verify

## --------------------------------------
## Others / Utilities
## --------------------------------------

.PHONY: diagrams
diagrams: ## Build proposal diagrams
	$(MAKE) -C docs diagrams

.PHONY: serve-book
serve-book: ## Build and serve the book with live-reloading enabled
	$(MAKE) -C docs/book serve
