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

# Ensure Make is run with bash shell as some syntax below is bash-specific

ROOT_DIR_RELATIVE := .
include $(ROOT_DIR_RELATIVE)/common.mk
include $(ROOT_DIR_RELATIVE)/hack/tools/tools.mk

# Default timeout for starting/stopping the Kubebuilder test control plane
export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?=60s
export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?=60s

# Directories.
EXP_DIR := exp
BIN_DIR := bin
E2E_FRAMEWORK_DIR := test/framework
CAPD_DIR := test/infrastructure/docker

# Bindata.
GOBINDATA_CLUSTERCTL_DIR := cmd/clusterctl/config
CLOUDINIT_PKG_DIR := bootstrap/kubeadm/internal/cloudinit
CLOUDINIT_GENERATED := $(CLOUDINIT_PKG_DIR)/zz_generated.bindata.go
CLOUDINIT_SCRIPT := $(CLOUDINIT_PKG_DIR)/kubeadm-bootstrap-script.sh

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

# Set build time variables including version details
LDFLAGS := $(shell hack/version.sh)

all: test managers clusterctl

## --------------------------------------
## Testing
## --------------------------------------

TEST_ARGS ?=
GOTEST_OPTS ?=
GOTEST_CMD ?= go

.PHONY: test
test: ## Run tests. Parameters: GOTEST_CMD (command to use for testing. default="go"), TEST_ARGS (args to pass to the test package. default="")
	source ./scripts/fetch_ext_bins.sh
	fetch_tools
	setup_envs
	$(GOTEST_CMD) test -v $(GOTEST_OPTS) ./... $(TEST_ARGS)

.PHONY: test-cover
test-cover: ## Run tests with code coverage and code generate reports. See test for accepted parameters.
	$(MAKE) test GOTEST_OPTS="-coverprofile=out/coverage.out" TEST_ARGS=$(TEST_ARGS) GOTEST_CMD=$(GOTEST_CMD)
	go tool cover -func=out/coverage.out -o out/coverage.txt
	go tool cover -html=out/coverage.out -o out/coverage.html

GINKGO_NODES ?= 1
GINKGO_NOCOLOR ?= false
GINKGO_ARGS ?=
ARTIFACTS ?= $(abspath _artifacts)
E2E_CONF_FILE ?= config/docker-dev.yaml
SKIP_RESOURCE_CLEANUP ?= false
USE_EXISTING_CLUSTER ?= false
GINKGO_FOCUS ?=
export GINKGO_NODES GINKGO_NOCOLOR GINKGO_ARGS ARTIFACTS E2E_CONF_FILE SKIP_RESOURCE_CLEANUP USE_EXISTING_CLUSTER GINKGO_FOCUS

.PHONY: test-e2e
test-e2e: $(GINKGO) ## Run the e2e tests. Parameters: GINKGO_NODES (number of parallel Ginkgo runners. default=1), GINKGO_NOCOLOR (whether or not to use color output for Ginkgo. default=false), ARTIFACTS (where to save output. default=_artifacts), E2E_CONF_FILE (clusterctl framework e2e conf file location. default=config/docker-dev.yaml), SKIP_RESOURCE_CLEANUP (default=false), USE_EXISTING_CLUSTER (use current kubectl context, default=false)
	$(MAKE) test-e2e-run TEST_DIR=test/e2e

.PHONY: test-conformance
test-conformance: $(GINKGO) $(KUSTOMIZE) ## Run e2e conformance tests. See test-e2e for accepted parameters.
	$(MAKE) test-e2e-run TEST_DIR=test/e2e/conformance

.PHONY: test-e2e-run
test-e2e-run: docker-build-e2e
	$(GINKGO) -v -trace \
		-tags=e2e -focus="$(GINKGO_FOCUS)" \
		-nodes=$(GINKGO_NODES) \
		--noColor=$(GINKGO_NOCOLOR) \
		$(GINKGO_ARGS) $(TEST_DIR) \
		-- \
		-e2e.artifacts-folder="$(ARTIFACTS)" \
		-e2e.config="$(E2E_CONF_FILE)" \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: pull-cert-manager
pull-cert-manager: ## Use to cache cert manager images on the top-level Docker
	docker pull quay.io/jetstack/cert-manager-cainjector:$(CERT_MANAGER_VERSION)
	docker pull quay.io/jetstack/cert-manager-webhook:$(CERT_MANAGER_VERSION)
	docker pull quay.io/jetstack/cert-manager-controller:$(CERT_MANAGER_VERSION)

## --------------------------------------
## Binaries
## --------------------------------------

.PHONY: manager-core
manager-core: ## Build core manager binary
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/manager sigs.k8s.io/cluster-api

.PHONY: manager-kubeadm-bootstrap
manager-kubeadm-bootstrap: ## Build kubeadm bootstrap manager
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/kubeadm-bootstrap-manager sigs.k8s.io/cluster-api/bootstrap/kubeadm

.PHONY: manager-kubeadm-control-plane
manager-kubeadm-control-plane: ## Build kubeadm control plane manager
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/kubeadm-control-plane-manager sigs.k8s.io/cluster-api/controlplane/kubeadm

.PHONY: managers
managers: ## Build all managers
	$(MAKE) manager-core
	$(MAKE) manager-kubeadm-bootstrap
	$(MAKE) manager-kubeadm-control-plane

.PHONY: clusterctl
clusterctl: ## Build clusterctl binary
	go build -ldflags "$(LDFLAGS)" -o bin/clusterctl sigs.k8s.io/cluster-api/cmd/clusterctl

.PHONY: e2e-framework
e2e-framework: ## Builds the CAPI e2e framework
	go build ./test/framework/...

## --------------------------------------
## Linting
## --------------------------------------

LINT_ARGS ?=

.PHONY: lint lint-full
lint: $(GOLANGCI_LINT) ## Lint codebase
	$(GOLANGCI_LINT) run -v $(LINT_ARGS)
	$(MAKE) -C $(CAPD_DIR) lint LINT_ARGS=$(LINT_ARGS)

lint-full: ## Run slower linters to detect possible issues
	$(MAKE) lint LINT_ARGS=--fast=false

REMOTE ?=upstream

apidiff: $(GO_APIDIFF) ## Check for API differences during development. Parameters: REMOTE (which git remote to use. Default: upstream)
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "commit changes first"; exit 1; \
	fi

	HEAD_COMMIT=$$(git rev-parse $(REMOTE)/master); \
	TMP_DIR=$$(mktemp -d -t apidiff-XXXXXXXXXX); \
	git clone . $${TMP_DIR}; \
	cd $${TMP_DIR}; \
	$(abspath $(GO_APIDIFF)) $${HEAD_COMMIT} --print-compatible; \
	rm -rf $${TMP_DIR}

## --------------------------------------
## Generate / Manifests
## --------------------------------------

.PHONY: generate
generate: ## Generate code
	$(MAKE) generate-manifests
	$(MAKE) generate-go
	$(MAKE) generate-bindata
	$(MAKE) -C $(CAPD_DIR) generate

.PHONY: generate-go
generate-go: $(GOBINDATA) ## Runs Go related generate targets
	go generate ./...
	$(MAKE) generate-go-core
	$(MAKE) generate-go-kubeadm-bootstrap
	$(MAKE) generate-go-kubeadm-control-plane

.PHONY: generate-go-core
generate-go-core: $(CONTROLLER_GEN) $(CONVERSION_GEN)
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./api/... \
		paths=./$(EXP_DIR)/api/... \
		paths=./$(EXP_DIR)/addons/api/... \
		paths=./cmd/clusterctl/...
	$(CONVERSION_GEN) \
		--input-dirs=./api/v1alpha2 \
		--output-file-base=zz_generated.conversion \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt

.PHONY: generate-go-kubeadm-bootstrap
generate-go-kubeadm-bootstrap: $(CONTROLLER_GEN) $(CONVERSION_GEN) ## Runs Go related generate targets for the kubeadm bootstrapper
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./bootstrap/kubeadm/api/... \
		paths=./bootstrap/kubeadm/types/...
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
generate-bindata: $(KUSTOMIZE) $(GOBINDATA) clean-bindata $(CLOUDINIT_GENERATED) ## Generate code for embedding the clusterctl api manifest
	# Package manifest YAML into a single file.
	mkdir -p $(GOBINDATA_CLUSTERCTL_DIR)/manifest/
	$(KUSTOMIZE) build $(GOBINDATA_CLUSTERCTL_DIR)/crd > $(GOBINDATA_CLUSTERCTL_DIR)/manifest/clusterctl-api.yaml
	# Generate go-bindata, add boilerplate, then cleanup.
	$(GOBINDATA) -mode=420 -modtime=1 -pkg=config -o=$(GOBINDATA_CLUSTERCTL_DIR)/zz_generated.bindata.go $(GOBINDATA_CLUSTERCTL_DIR)/manifest/ $(GOBINDATA_CLUSTERCTL_DIR)/assets
	cat ./hack/boilerplate/boilerplate.generatego.txt $(GOBINDATA_CLUSTERCTL_DIR)/zz_generated.bindata.go > $(GOBINDATA_CLUSTERCTL_DIR)/manifest/manifests.go
	cp $(GOBINDATA_CLUSTERCTL_DIR)/manifest/manifests.go $(GOBINDATA_CLUSTERCTL_DIR)/zz_generated.bindata.go
	# Cleanup the manifest folder.
	$(MAKE) clean-bindata

$(CLOUDINIT_GENERATED): $(GOBINDATA) $(CLOUDINIT_SCRIPT)
	$(GOBINDATA) -mode=420 -modtime=1 -pkg=cloudinit -o=$(CLOUDINIT_GENERATED).tmp $(CLOUDINIT_SCRIPT)
	cat ./hack/boilerplate/boilerplate.generatego.txt $(CLOUDINIT_GENERATED).tmp > $(CLOUDINIT_GENERATED)
	rm $(CLOUDINIT_GENERATED).tmp

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
		paths=./$(EXP_DIR)/api/... \
		paths=./$(EXP_DIR)/controllers/... \
		paths=./$(EXP_DIR)/addons/api/... \
		paths=./$(EXP_DIR)/addons/controllers/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./config/crd/bases \
		output:webhook:dir=./config/webhook \
		webhook
	$(CONTROLLER_GEN) \
		paths=./cmd/clusterctl/api/... \
		crd:crdVersions=v1 \
		output:crd:dir=./cmd/clusterctl/config/crd/bases
	## Copy files in CI folders.
	cp -f ./config/rbac/*.yaml ./config/ci/rbac/
	cp -f ./config/manager/manager*.yaml ./config/ci/manager/

.PHONY: generate-kubeadm-bootstrap-manifests
generate-kubeadm-bootstrap-manifests: $(CONTROLLER_GEN) ## Generate manifests for the kubeadm bootstrap provider e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) \
		paths=./bootstrap/kubeadm/api/... \
		paths=./bootstrap/kubeadm/controllers/... \
		crd:crdVersions=v1 \
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
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./controlplane/kubeadm/config/crd/bases \
		output:rbac:dir=./controlplane/kubeadm/config/rbac \
		output:webhook:dir=./controlplane/kubeadm/config/webhook \
		webhook

.PHONY: modules
modules: ## Runs go mod to ensure modules are up to date.
	go mod tidy
	$(MAKE) -C $(TOOLS_DIR) modules
	$(MAKE) -C $(CAPD_DIR) modules

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-build
docker-build: docker-build-core docker-build-kubeadm-bootstrap docker-build-kubeadm-control-plane ## Build the docker images for controller managers

.PHONY: docker-build-core
docker-build-core: .docker-build-core.sentinel ## Build the docker image for core controller manager

CORE_SRCS := $(call rwildcard,controllers,*.*) $(call rwildcard,cmd,*.*) $(call rwildcard,feature,*.*) $(call rwildcard,exp,*.*) $(call rwildcard,api,*.*)  $(call rwildcard,third_party,*.*) go.mod go.sum

.docker-build-core.sentinel: $(CORE_SRCS)
	DOCKER_BUILDKIT=1 docker build --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg ldflags="$(LDFLAGS)" . -t $(CONTROLLER_IMG)-$(ARCH):$(TAG)
	touch $@

.PHONY: docker-build-kubeadm-bootstrap
docker-build-kubeadm-bootstrap: bootstrap/kubeadm/.docker-build-kubeadm-bootstrap.sentinel ## Build the docker image for kubeadm bootstrap controller manager

KUBEADM_BOOTSTRAP_SRCS := $(CORE_SRCS) $(call rwildcard,bootstrap,*.*)
bootstrap/kubeadm/.docker-build-kubeadm-bootstrap.sentinel: $(KUBEADM_BOOTSTRAP_SRCS)
	DOCKER_BUILDKIT=1 docker build --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./bootstrap/kubeadm --build-arg ldflags="$(LDFLAGS)" . -t $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH):$(TAG)
	touch $@

.PHONY: docker-build-kubeadm-control-plane
docker-build-kubeadm-control-plane: controlplane/kubeadm/.docker-build-kubeadm-control-plane.sentinel ## Build the docker image for kubeadm control plane controller manager

KUBEADM_CONTROLPLANE_SRCS := $(KUBEADM_BOOTSTRAP_SRCS) $(call rwildcard,controlplane,*.*)
controlplane/kubeadm/.docker-build-kubeadm-control-plane.sentinel: $(KUBEADM_CONTROLPLANE_SRCS)
	DOCKER_BUILDKIT=1 docker build --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./controlplane/kubeadm --build-arg ldflags="$(LDFLAGS)" . -t $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH):$(TAG)
	touch $@

.PHONY: docker-build-e2e
docker-build-e2e: ## Rebuild all Cluster API provider images to be used for e2e testing
	$(MAKE) docker-build REGISTRY=gcr.io/k8s-staging-cluster-api
	$(MAKE) -C $(CAPD_DIR) docker-build REGISTRY=gcr.io/k8s-staging-cluster-api

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
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./config/manager/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./config/manager/manager_pull_policy.yaml"

.PHONY: docker-push-kubeadm-bootstrap-manifest
docker-push-kubeadm-bootstrap-manifest: ## Push the fat manifest docker image for the kubeadm bootstrap image.
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${KUBEADM_BOOTSTRAP_CONTROLLER_IMG}:${TAG} ${KUBEADM_BOOTSTRAP_CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/kubeadm/config/manager/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/kubeadm/config/manager/manager_pull_policy.yaml"

.PHONY: docker-push-kubeadm-control-plane-manifest
docker-push-kubeadm-control-plane-manifest: ## Push the fat manifest docker image for the kubeadm control plane image.
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${KUBEADM_CONTROL_PLANE_CONTROLLER_IMG}:${TAG} ${KUBEADM_CONTROL_PLANE_CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./controlplane/kubeadm/config/manager/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./controlplane/kubeadm/config/manager/manager_pull_policy.yaml"

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
	# Build binaries first.
	$(MAKE) release-binaries
	# Set the core manifest image to the production bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(PROD_REGISTRY)/$(IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./config/manager/manager_image_patch.yaml"
	# Set the kubeadm bootstrap image to the production bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(PROD_REGISTRY)/$(KUBEADM_BOOTSTRAP_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./bootstrap/kubeadm/config/manager/manager_image_patch.yaml"
	# Set the kubeadm control plane image to the production bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(PROD_REGISTRY)/$(KUBEADM_CONTROL_PLANE_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./controlplane/kubeadm/config/manager/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./config/manager/manager_pull_policy.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./bootstrap/kubeadm/config/manager/manager_pull_policy.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./controlplane/kubeadm/config/manager/manager_pull_policy.yaml"
	$(MAKE) release-manifests
	# Release CAPD components and add them to the release dir
	$(MAKE) -C $(CAPD_DIR) release
	cp $(CAPD_DIR)/out/infrastructure-components.yaml $(RELEASE_DIR)/infrastructure-components-development.yaml
	# Adds CAPD templates
	cp $(CAPD_DIR)/templates/* $(RELEASE_DIR)/

.PHONY: release-manifests
release-manifests: $(RELEASE_DIR) $(KUSTOMIZE) ## Builds the manifests to publish with a release
	# Build core-components.
	$(KUSTOMIZE) build config > $(RELEASE_DIR)/core-components.yaml
	# Build bootstrap-components.
	$(KUSTOMIZE) build bootstrap/kubeadm/config > $(RELEASE_DIR)/bootstrap-components.yaml
	# Build control-plane-components.
	$(KUSTOMIZE) build controlplane/kubeadm/config > $(RELEASE_DIR)/control-plane-components.yaml

	## Build cluster-api-components (aggregate of all of the above).
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
		golang:1.13.15 \
		go build -a -ldflags "$(LDFLAGS) -extldflags '-static'" \
		-o $(RELEASE_DIR)/$(notdir $(RELEASE_BINARY))-$(GOOS)-$(GOARCH) $(RELEASE_BINARY)

.PHONY: release-staging
release-staging: ## Builds and push container images to the staging bucket.
	docker pull docker.io/docker/dockerfile:experimental
	docker pull docker.io/library/golang:1.13.15
	docker pull gcr.io/distroless/static:latest
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
docker-build-example-provider: ## Build the docker image for example provider
	DOCKER_BUILDKIT=1 docker build --pull --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) . -f ./cmd/example-provider/Dockerfile -t $(EXAMPLE_PROVIDER_IMG)-$(ARCH):$(TAG)
	sed -i'' -e 's@image: .*@image: '"${EXAMPLE_PROVIDER_IMG}-$(ARCH):$(TAG)"'@' ./config/ci/manager/manager_image_patch.yaml

## --------------------------------------
## Cleanup / Verification
## --------------------------------------

.PHONY: clean
clean: ## Remove all generated files
	$(MAKE) clean-bin
	$(MAKE) clean-book
	$(MAKE) clean-artifacts
	$(MAKE) clean-envtest
	$(MAKE) clean-sentinels

.PHONY: clean-sentinels
clean-sentinels: ## Delete docker build sentinels
	rm -f .docker-build-core.sentinel
	rm -f bootstrap/kubeadm/.docker-build-kubeadm-bootstrap.sentinel
	rm -f controlplane/kubeadm/.docker-build-kubeadm-control-plane.sentinel
	$(MAKE) -C $(CAPD_DIR) clean-sentinels

.PHONY: clean-artifacts
clean-artifacts: ## Remove all test artifacts
	rm -rf _artifacts

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

.PHONY: clean-manifests
clean-manifests:  ## Reset manifests in config directories back to master
	@read -p "WARNING: This will reset all config directories to local master. Press [ENTER] to continue."
	git checkout master config bootstrap/kubeadm/config controlplane/kubeadm/config test/infrastructure/docker/config

.PHONY: clean-envtest
clean-envtest: ## Remove kubebuilder envtest binaries and tars
	-rm -rf /tmp/kubebuilder*

.PHONY: format-tiltfile
format-tiltfile: ## Format Tiltfile
	./hack/verify-starlark.sh fix

.PHONY: verify
verify:
	./hack/verify-boilerplate.sh
	./hack/verify-doctoc.sh
	./hack/verify-shellcheck.sh
	./hack/verify-starlark.sh
	$(MAKE) verify-book-links
	$(MAKE) verify-modules
	$(MAKE) verify-gen
	$(MAKE) verify-docker-provider

.PHONY: verify-modules
verify-modules: modules
	@if !(git diff --quiet HEAD -- go.sum go.mod hack/tools/go.mod hack/tools/go.sum); then \
		git diff; \
		echo "go module files are out of date"; exit 1; \
	fi
	@if (find . -name 'go.mod' | xargs -n1 grep -q -i 'k8s.io/client-go.*+incompatible'); then \
		find . -name "go.mod" -exec grep -i 'k8s.io/client-go.*+incompatible' {} \; -print; \
		echo "go module contains an incompatible client-go version"; exit 1; \
	fi

.PHONY: verify-gen
verify-gen: generate
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

.PHONY: verify-docker-provider
verify-docker-provider:
	@echo "Verifying CAPD"
	$(MAKE) -C $(CAPD_DIR) verify

.PHONY: verify-book-links
verify-book-links: $(LINK_CHECKER)
	 # Ignore localhost links and set concurrency to a reasonable number
	$(LINK_CHECKER) -r docs/book -x "^https?://" -c 10

## --------------------------------------
## Others / Utilities
## --------------------------------------

.PHONY: diagrams
diagrams: ## Build proposal diagrams
	$(MAKE) -C docs diagrams

.PHONY: serve-book
serve-book: ## Build and serve the book with live-reloading enabled
	$(MAKE) -C docs/book serve

