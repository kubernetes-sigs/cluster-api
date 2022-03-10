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
# https://www.thapaliya.com/en/writings/well-documented-makefiles/

# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL:=/usr/bin/env bash

.DEFAULT_GOAL:=help

#
# Go.
#
GO_VERSION ?= 1.17.3
GO_CONTAINER_IMAGE ?= docker.io/library/golang:$(GO_VERSION)

# Use GOPROXY environment variable if set
GOPROXY := $(shell go env GOPROXY)
ifeq ($(GOPROXY),)
GOPROXY := https://proxy.golang.org
endif
export GOPROXY

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE=on

#
# Kubebuilder.
#
export KUBEBUILDER_ENVTEST_KUBERNETES_VERSION ?= 1.23.3
export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?= 60s
export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?= 60s

# This option is for running docker manifest command
export DOCKER_CLI_EXPERIMENTAL := enabled

#
# Directories.
#
# Full directory of where the Makefile resides
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
EXP_DIR := exp
BIN_DIR := bin
TEST_DIR := test
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(abspath $(TOOLS_DIR)/$(BIN_DIR))
E2E_FRAMEWORK_DIR := $(TEST_DIR)/framework
CAPD_DIR := $(TEST_DIR)/infrastructure/docker
GO_INSTALL := ./scripts/go_install.sh

export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

# Set --output-base for conversion-gen if we are not within GOPATH
ifneq ($(abspath $(ROOT_DIR)),$(shell go env GOPATH)/src/sigs.k8s.io/cluster-api)
	CONVERSION_GEN_OUTPUT_BASE := --output-base=$(ROOT_DIR)
else
	export GOPATH := $(shell go env GOPATH)
endif

#
# Binaries.
#
# Note: Need to use abspath so we can invoke these from subdirectories
KUSTOMIZE_VER := v4.5.2
KUSTOMIZE_BIN := kustomize
KUSTOMIZE := $(abspath $(TOOLS_BIN_DIR)/$(KUSTOMIZE_BIN)-$(KUSTOMIZE_VER))
KUSTOMIZE_PKG := sigs.k8s.io/kustomize/kustomize/v4

SETUP_ENVTEST_VER := v0.0.0-20211110210527-619e6b92dab9
SETUP_ENVTEST_BIN := setup-envtest
SETUP_ENVTEST := $(abspath $(TOOLS_BIN_DIR)/$(SETUP_ENVTEST_BIN)-$(SETUP_ENVTEST_VER))
SETUP_ENVTEST_PKG := sigs.k8s.io/controller-runtime/tools/setup-envtest

CONTROLLER_GEN_VER := v0.8.0
CONTROLLER_GEN_BIN := controller-gen
CONTROLLER_GEN := $(abspath $(TOOLS_BIN_DIR)/$(CONTROLLER_GEN_BIN)-$(CONTROLLER_GEN_VER))
CONTROLLER_GEN_PKG := sigs.k8s.io/controller-tools/cmd/controller-gen

GOTESTSUM_VER := v1.6.4
GOTESTSUM_BIN := gotestsum
GOTESTSUM := $(abspath $(TOOLS_BIN_DIR)/$(GOTESTSUM_BIN)-$(GOTESTSUM_VER))
GOTESTSUM_PKG := gotest.tools/gotestsum

CONVERSION_GEN_VER := v0.23.1
CONVERSION_GEN_BIN := conversion-gen
# We are intentionally using the binary without version suffix, to avoid the version
# in generated files.
CONVERSION_GEN := $(abspath $(TOOLS_BIN_DIR)/$(CONVERSION_GEN_BIN))
CONVERSION_GEN_PKG := k8s.io/code-generator/cmd/conversion-gen

ENVSUBST_VER := v2.0.0-20210730161058-179042472c46
ENVSUBST_BIN := envsubst
ENVSUBST := $(abspath $(TOOLS_BIN_DIR)/$(ENVSUBST_BIN)-$(ENVSUBST_VER))
ENVSUBST_PKG := github.com/drone/envsubst/v2/cmd/envsubst

GO_APIDIFF_VER := v0.1.0
GO_APIDIFF_BIN := go-apidiff
GO_APIDIFF := $(abspath $(TOOLS_BIN_DIR)/$(GO_APIDIFF_BIN)-$(GO_APIDIFF_VER))
GO_APIDIFF_PKG := github.com/joelanford/go-apidiff

KPROMO_VER := v3.3.0-beta.3
KPROMO_BIN := kpromo
KPROMO :=  $(abspath $(TOOLS_BIN_DIR)/$(KPROMO_BIN)-$(KPROMO_VER))
KPROMO_PKG := sigs.k8s.io/promo-tools/v3/cmd/kpromo

CONVERSION_VERIFIER_BIN := conversion-verifier
CONVERSION_VERIFIER := $(abspath $(TOOLS_BIN_DIR)/$(CONVERSION_VERIFIER_BIN))

TILT_PREPARE_BIN := tilt-prepare
TILT_PREPARE := $(abspath $(TOOLS_BIN_DIR)/$(TILT_PREPARE_BIN))

GOLANGCI_LINT_BIN := golangci-lint
GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN))

# clusterctl.
CLUSTERCTL_MANIFEST_DIR := cmd/clusterctl/config

# Define Docker related variables. Releases should modify and double check these vars.
REGISTRY ?= gcr.io/$(shell gcloud config get-value project)
PROD_REGISTRY ?= k8s.gcr.io/cluster-api

STAGING_REGISTRY ?= gcr.io/k8s-staging-cluster-api
STAGING_BUCKET ?= artifacts.k8s-staging-cluster-api.appspot.com

# core
IMAGE_NAME ?= cluster-api-controller
CONTROLLER_IMG ?= $(REGISTRY)/$(IMAGE_NAME)

# bootstrap
KUBEADM_BOOTSTRAP_IMAGE_NAME ?= kubeadm-bootstrap-controller
KUBEADM_BOOTSTRAP_CONTROLLER_IMG ?= $(REGISTRY)/$(KUBEADM_BOOTSTRAP_IMAGE_NAME)

# control plane
KUBEADM_CONTROL_PLANE_IMAGE_NAME ?= kubeadm-control-plane-controller
KUBEADM_CONTROL_PLANE_CONTROLLER_IMG ?= $(REGISTRY)/$(KUBEADM_CONTROL_PLANE_IMAGE_NAME)

# It is set by Prow GIT_TAG, a git-based tag of the form vYYYYMMDD-hash, e.g., v20210120-v0.3.10-308-gc61521971

TAG ?= dev
ARCH ?= $(shell go env GOARCH)
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

all: test managers clusterctl

help:  # Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[0-9A-Za-z_-]+:.*?##/ { printf "  \033[36m%-45s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Generate / Manifests
## --------------------------------------

##@ generate:

ALL_GENERATE_MODULES = core kubeadm-bootstrap kubeadm-control-plane

.PHONY: generate
generate: ## Run all generate-manifests-*, generate-go-deepcopy-* and generate-go-conversions-* targets
	$(MAKE) generate-modules generate-manifests generate-go-deepcopy generate-go-conversions
	$(MAKE) -C $(CAPD_DIR) generate

.PHONY: generate-manifests
generate-manifests: $(addprefix generate-manifests-,$(ALL_GENERATE_MODULES)) ## Run all generate-manifests-* targets

.PHONY: generate-manifests-core
generate-manifests-core: $(CONTROLLER_GEN) $(KUSTOMIZE) ## Generate manifests e.g. CRD, RBAC etc. for core
	$(CONTROLLER_GEN) \
		paths=./api/... \
		paths=./internal/controllers/... \
		paths=./internal/webhooks/... \
		paths=./$(EXP_DIR)/api/... \
		paths=./$(EXP_DIR)/internal/controllers/... \
		paths=./$(EXP_DIR)/addons/api/... \
		paths=./$(EXP_DIR)/addons/internal/controllers/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./config/crd/bases \
		output:webhook:dir=./config/webhook \
		webhook
	$(CONTROLLER_GEN) \
		paths=./cmd/clusterctl/api/... \
		crd:crdVersions=v1 \
		output:crd:dir=./cmd/clusterctl/config/crd/bases
	$(KUSTOMIZE) build $(CLUSTERCTL_MANIFEST_DIR)/crd > $(CLUSTERCTL_MANIFEST_DIR)/manifest/clusterctl-api.yaml

.PHONY: generate-manifests-kubeadm-bootstrap
generate-manifests-kubeadm-bootstrap: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for kubeadm bootstrap
	$(CONTROLLER_GEN) \
		paths=./bootstrap/kubeadm/api/... \
		paths=./bootstrap/kubeadm/internal/controllers/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./bootstrap/kubeadm/config/crd/bases \
		output:rbac:dir=./bootstrap/kubeadm/config/rbac \
		output:webhook:dir=./bootstrap/kubeadm/config/webhook \
		webhook

.PHONY: generate-manifests-kubeadm-control-plane
generate-manifests-kubeadm-control-plane: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for kubeadm control plane
	$(CONTROLLER_GEN) \
		paths=./controlplane/kubeadm/api/... \
		paths=./controlplane/kubeadm/internal/controllers/... \
		paths=./controlplane/kubeadm/internal/webhooks/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./controlplane/kubeadm/config/crd/bases \
		output:rbac:dir=./controlplane/kubeadm/config/rbac \
		output:webhook:dir=./controlplane/kubeadm/config/webhook \
		webhook

.PHONY: generate-go-deepcopy
generate-go-deepcopy:  ## Run all generate-go-deepcopy-* targets
	$(MAKE) $(addprefix generate-go-deepcopy-,$(ALL_GENERATE_MODULES))

.PHONY: generate-go-deepcopy-core
generate-go-deepcopy-core: $(CONTROLLER_GEN) ## Generate deepcopy go code for core
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./api/... \
		paths=./$(EXP_DIR)/api/... \
		paths=./$(EXP_DIR)/addons/api/... \
		paths=./cmd/clusterctl/... \
		paths=./internal/test/builder/...

.PHONY: generate-go-deepcopy-kubeadm-bootstrap
generate-go-deepcopy-kubeadm-bootstrap: $(CONTROLLER_GEN) ## Generate deepcopy go code for kubeadm bootstrap
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./bootstrap/kubeadm/api/... \
		paths=./bootstrap/kubeadm/types/...

.PHONY: generate-go-deepcopy-kubeadm-control-plane
generate-go-deepcopy-kubeadm-control-plane: $(CONTROLLER_GEN) ## Generate deepcopy go code for kubeadm control plane
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./controlplane/kubeadm/api/...

.PHONY: generate-go-conversions
generate-go-conversions: ## Run all generate-go-conversions-* targets
	$(MAKE) $(addprefix generate-go-conversions-,$(ALL_GENERATE_MODULES))

.PHONY: generate-go-conversions-core
generate-go-conversions-core: $(CONVERSION_GEN) ## Generate conversions go code for core
	$(MAKE) clean-generated-conversions SRC_DIRS="./api/v1alpha3,./$(EXP_DIR)/api/v1alpha3,./$(EXP_DIR)/addons/api/v1alpha3"
	$(MAKE) clean-generated-conversions SRC_DIRS="./api/v1alpha4,./$(EXP_DIR)/api/v1alpha4,./$(EXP_DIR)/addons/api/v1alpha4"
	$(CONVERSION_GEN) \
		--input-dirs=./api/v1alpha3 \
		--input-dirs=./api/v1alpha4 \
		--build-tag=ignore_autogenerated_core \
		--output-file-base=zz_generated.conversion $(CONVERSION_GEN_OUTPUT_BASE) \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt
	$(CONVERSION_GEN) \
		--input-dirs=./$(EXP_DIR)/api/v1alpha3 \
		--input-dirs=./$(EXP_DIR)/api/v1alpha4 \
		--input-dirs=./$(EXP_DIR)/addons/api/v1alpha3 \
		--input-dirs=./$(EXP_DIR)/addons/api/v1alpha4 \
		--extra-peer-dirs=sigs.k8s.io/cluster-api/api/v1alpha3 \
		--extra-peer-dirs=sigs.k8s.io/cluster-api/api/v1alpha4 \
		--output-file-base=zz_generated.conversion $(CONVERSION_GEN_OUTPUT_BASE) \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt

.PHONY: generate-go-conversions-kubeadm-bootstrap
generate-go-conversions-kubeadm-bootstrap: $(CONVERSION_GEN) ## Generate conversions go code for kubeadm bootstrap
	$(MAKE) clean-generated-conversions SRC_DIRS="./bootstrap/kubeadm/api"
	$(CONVERSION_GEN) \
		--input-dirs=./bootstrap/kubeadm/api/v1alpha3 \
		--input-dirs=./bootstrap/kubeadm/api/v1alpha4 \
		--build-tag=ignore_autogenerated_kubeadm_bootstrap \
		--extra-peer-dirs=sigs.k8s.io/cluster-api/api/v1alpha3 \
		--extra-peer-dirs=sigs.k8s.io/cluster-api/api/v1alpha4 \
		--output-file-base=zz_generated.conversion $(CONVERSION_GEN_OUTPUT_BASE) \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt
	$(MAKE) clean-generated-conversions SRC_DIRS="./bootstrap/kubeadm/types/upstreamv1beta1,./bootstrap/kubeadm/types/upstreamv1beta2,./bootstrap/kubeadm/types/upstreamv1beta3"
	$(CONVERSION_GEN) \
		--input-dirs=./bootstrap/kubeadm/types/upstreamv1beta1 \
		--input-dirs=./bootstrap/kubeadm/types/upstreamv1beta2 \
		--input-dirs=./bootstrap/kubeadm/types/upstreamv1beta3 \
		--build-tag=ignore_autogenerated_kubeadm_types \
		--output-file-base=zz_generated.conversion $(CONVERSION_GEN_OUTPUT_BASE) \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt

.PHONY: generate-go-conversions-kubeadm-control-plane
generate-go-conversions-kubeadm-control-plane: $(CONVERSION_GEN) ## Generate conversions go code for kubeadm control plane
	$(MAKE) clean-generated-conversions SRC_DIRS="./controlplane/kubeadm/api"
	$(CONVERSION_GEN) \
		--input-dirs=./controlplane/kubeadm/api/v1alpha3 \
		--input-dirs=./controlplane/kubeadm/api/v1alpha4 \
		--build-tag=ignore_autogenerated_kubeadm_controlplane \
		--extra-peer-dirs=sigs.k8s.io/cluster-api/api/v1alpha3 \
		--extra-peer-dirs=sigs.k8s.io/cluster-api/api/v1alpha4 \
		--extra-peer-dirs=sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3 \
		--extra-peer-dirs=sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4 \
		--output-file-base=zz_generated.conversion $(CONVERSION_GEN_OUTPUT_BASE) \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt

.PHONY: generate-modules
generate-modules: ## Run go mod tidy to ensure modules are up to date
	go mod tidy
	cd $(TOOLS_DIR); go mod tidy
	cd $(TEST_DIR); go mod tidy

.PHONY: generate-diagrams
generate-diagrams: ## Generate diagrams for *.plantuml files
	$(MAKE) -C docs diagrams

## --------------------------------------
## Lint / Verify
## --------------------------------------

##@ lint and verify:

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Lint the codebase
	$(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS)
	cd $(TEST_DIR); $(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS)
	cd $(TOOLS_DIR); $(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS)

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Lint the codebase and run auto-fixers if supported by the linter
	GOLANGCI_LINT_EXTRA_ARGS=--fix $(MAKE) lint

.PHONY: tiltfile-fix
tiltfile-fix: ## Format the Tiltfile
	./hack/verify-starlark.sh fix

APIDIFF_OLD_COMMIT ?= $(shell git rev-parse origin/main)

.PHONY: apidiff
apidiff: $(GO_APIDIFF) ## Check for API differences
	$(GO_APIDIFF) $(APIDIFF_OLD_COMMIT) --print-compatible

ALL_VERIFY_CHECKS = doctoc boilerplate shellcheck tiltfile modules gen conversions docker-provider

.PHONY: verify
verify: $(addprefix verify-,$(ALL_VERIFY_CHECKS)) ## Run all verify-* targets

.PHONY: verify-modules
verify-modules: generate-modules  ## Verify go modules are up to date
	@if !(git diff --quiet HEAD -- go.sum go.mod $(TOOLS_DIR)/go.mod $(TOOLS_DIR)/go.sum $(TEST_DIR)/go.mod $(TEST_DIR)/go.sum); then \
		git diff; \
		echo "go module files are out of date"; exit 1; \
	fi
	@if (find . -name 'go.mod' | xargs -n1 grep -q -i 'k8s.io/client-go.*+incompatible'); then \
		find . -name "go.mod" -exec grep -i 'k8s.io/client-go.*+incompatible' {} \; -print; \
		echo "go module contains an incompatible client-go version"; exit 1; \
	fi

.PHONY: verify-gen
verify-gen: generate  ## Verfiy go generated files are up to date
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

.PHONY: verify-conversions
verify-conversions: $(CONVERSION_VERIFIER)  ## Verifies expected API conversion are in place
	$(CONVERSION_VERIFIER)

.PHONY: verify-doctoc
verify-doctoc:
	./hack/verify-doctoc.sh

.PHONY: verify-boilerplate
verify-boilerplate: ## Verify boilerplate text exists in each file
	./hack/verify-boilerplate.sh

.PHONY: verify-shellcheck
verify-shellcheck: ## Verify shell files
	./hack/verify-shellcheck.sh

.PHONY: verify-tiltfile
verify-tiltfile: ## Verify Tiltfile format
	./hack/verify-starlark.sh

.PHONY: verify-docker-provider
verify-docker-provider:
	@echo "Verifying CAPD"
	cd $(CAPD_DIR); $(MAKE) verify

## --------------------------------------
## Binaries
## --------------------------------------

##@ build:

.PHONY: clusterctl
clusterctl: ## Build the clusterctl binary
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/clusterctl sigs.k8s.io/cluster-api/cmd/clusterctl

ALL_MANAGERS = core kubeadm-bootstrap kubeadm-control-plane

.PHONY: managers
managers: $(addprefix manager-,$(ALL_MANAGERS)) ## Run all manager-* targets

.PHONY: manager-core
manager-core: ## Build the core manager binary into the ./bin folder
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/manager sigs.k8s.io/cluster-api

.PHONY: manager-kubeadm-bootstrap
manager-kubeadm-bootstrap: ## Build the kubeadm bootstrap manager binary into the ./bin folder
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/kubeadm-bootstrap-manager sigs.k8s.io/cluster-api/bootstrap/kubeadm

.PHONY: manager-kubeadm-control-plane
manager-kubeadm-control-plane: ## Build the kubeadm control plane manager binary into the ./bin folder
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/kubeadm-control-plane-manager sigs.k8s.io/cluster-api/controlplane/kubeadm

.PHONY: docker-pull-prerequisites
docker-pull-prerequisites:
	docker pull docker.io/docker/dockerfile:1.1-experimental
	docker pull $(GO_CONTAINER_IMAGE)
	docker pull gcr.io/distroless/static:latest

.PHONY: docker-build-all
docker-build-all: $(addprefix docker-build-,$(ALL_ARCH)) ## Build docker images for all architectures

docker-build-%:
	$(MAKE) ARCH=$* docker-build

ALL_DOCKER_BUILD = core kubeadm-bootstrap kubeadm-control-plane

.PHONY: docker-build
docker-build: docker-pull-prerequisites ## Run docker-build-* targets for all providers
	$(MAKE) ARCH=$(ARCH) $(addprefix docker-build-,$(ALL_DOCKER_BUILD))

.PHONY: docker-build-core
docker-build-core: ## Build the docker image for core controller manager
	DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg ldflags="$(LDFLAGS)" . -t $(CONTROLLER_IMG)-$(ARCH):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./config/default/manager_pull_policy.yaml"

.PHONY: docker-build-kubeadm-bootstrap
docker-build-kubeadm-bootstrap: ## Build the docker image for kubeadm bootstrap controller manager
	DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./bootstrap/kubeadm --build-arg ldflags="$(LDFLAGS)" . -t $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: docker-build-kubeadm-control-plane
docker-build-kubeadm-control-plane: ## Build the docker image for kubeadm control plane controller manager
	DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./controlplane/kubeadm --build-arg ldflags="$(LDFLAGS)" . -t $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: e2e-framework
e2e-framework: ## Builds the CAPI e2e framework
	cd $(E2E_FRAMEWORK_DIR); go build ./...

.PHONY: build-book
build-book: ## Build the book
	$(MAKE) -C docs/book build

.PHONY: serve-book
serve-book: ## Build and serve the book (with live-reload)
	$(MAKE) -C docs/book serve

## --------------------------------------
## Testing
## --------------------------------------

##@ test:

ARTIFACTS ?= ${ROOT_DIR}/_artifacts

ifeq ($(shell go env GOOS),darwin) # Use the darwin/amd64 binary until an arm64 version is available
	KUBEBUILDER_ASSETS ?= $(shell $(SETUP_ENVTEST) use --use-env -p path --arch amd64 $(KUBEBUILDER_ENVTEST_KUBERNETES_VERSION))
else
	KUBEBUILDER_ASSETS ?= $(shell $(SETUP_ENVTEST) use --use-env -p path $(KUBEBUILDER_ENVTEST_KUBERNETES_VERSION))
endif

.PHONY: test
test: $(SETUP_ENVTEST) ## Run unit and integration tests
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./... $(TEST_ARGS)

.PHONY: test-verbose
test-verbose: ## Run unit and integration tests with verbose flag
	$(MAKE) test TEST_ARGS="$(TEST_ARGS) -v"

.PHONY: test-junit
test-junit: $(SETUP_ENVTEST) $(GOTESTSUM) ## Run unit and integration tests and generate a junit report
	set +o errexit; (KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -json ./... $(TEST_ARGS); echo $$? > $(ARTIFACTS)/junit.exitcode) | tee $(ARTIFACTS)/junit.stdout
	$(GOTESTSUM) --junitfile $(ARTIFACTS)/junit.xml --raw-command cat $(ARTIFACTS)/junit.stdout
	exit $$(cat $(ARTIFACTS)/junit.exitcode)

.PHONY: test-cover
test-cover: ## Run unit and integration tests and generate a coverage report
	$(MAKE) test TEST_ARGS="$(TEST_ARGS) -coverprofile=out/coverage.out"
	go tool cover -func=out/coverage.out -o out/coverage.txt
	go tool cover -html=out/coverage.out -o out/coverage.html

.PHONY: test-e2e
test-e2e: ## Run e2e tests
	$(MAKE) -C $(TEST_DIR)/e2e run

.PHONY: kind-cluster
kind-cluster: ## Create a new kind cluster designed for development with Tilt
	hack/kind-install-for-capd.sh

.PHONY: docker-build-e2e
docker-build-e2e: ## Rebuild all Cluster API provider images to be used in the e2e tests
	make docker-build REGISTRY=gcr.io/k8s-staging-cluster-api PULL_POLICY=IfNotPresent
	$(MAKE) -C $(CAPD_DIR) docker-build REGISTRY=gcr.io/k8s-staging-cluster-api PULL_POLICY=IfNotPresent

## --------------------------------------
## Release
## --------------------------------------

##@ release:

## latest git tag for the commit, e.g., v0.3.10
RELEASE_TAG ?= $(shell git describe --abbrev=0 2>/dev/null)
ifneq (,$(findstring -,$(RELEASE_TAG)))
    PRE_RELEASE=true
endif
# the previous release tag, e.g., v0.3.9, excluding pre-release tags
PREVIOUS_TAG ?= $(shell git tag -l | grep -E "^v[0-9]+\.[0-9]+\.[0-9]+$$" | sort -V | grep -B1 $(RELEASE_TAG) | head -n 1 2>/dev/null)
## set by Prow, ref name of the base branch, e.g., main
RELEASE_ALIAS_TAG := $(PULL_BASE_REF)
RELEASE_DIR := out
RELEASE_NOTES_DIR := _releasenotes
USER_FORK ?= $(shell git config --get remote.origin.url | cut -d/ -f4)
IMAGE_REVIEWERS ?= $(shell ./hack/get-project-maintainers.sh)

$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)/

$(RELEASE_NOTES_DIR):
	mkdir -p $(RELEASE_NOTES_DIR)/

.PHONY: release
release: clean-release ## Build and push container images using the latest git tag for the commit
	@if [ -z "${RELEASE_TAG}" ]; then echo "RELEASE_TAG is not set"; exit 1; fi
	@if ! [ -z "$$(git status --porcelain)" ]; then echo "Your local git repository contains uncommitted changes, use git clean before proceeding."; exit 1; fi
	git checkout "${RELEASE_TAG}"
	# Build binaries first.
	GIT_VERSION=$(RELEASE_TAG) $(MAKE) release-binaries
	# Set the manifest image to the production bucket.
	$(MAKE) manifest-modification REGISTRY=$(PROD_REGISTRY)
	## Build the manifests
	$(MAKE) release-manifests clean-release-git
	## Build the development manifests
	$(MAKE) release-manifests-dev clean-release-git

.PHONY: manifest-modification
manifest-modification: # Set the manifest images to the staging/production bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(REGISTRY)/$(IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(REGISTRY)/$(KUBEADM_BOOTSTRAP_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(REGISTRY)/$(KUBEADM_CONTROL_PLANE_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./config/default/manager_pull_policy.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_pull_policy.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: release-manifests
release-manifests: $(RELEASE_DIR) $(KUSTOMIZE) ## Build the manifests to publish with a release
	# Build core-components.
	$(KUSTOMIZE) build config/default > $(RELEASE_DIR)/core-components.yaml
	# Build bootstrap-components.
	$(KUSTOMIZE) build bootstrap/kubeadm/config/default > $(RELEASE_DIR)/bootstrap-components.yaml
	# Build control-plane-components.
	$(KUSTOMIZE) build controlplane/kubeadm/config/default > $(RELEASE_DIR)/control-plane-components.yaml

	## Build cluster-api-components (aggregate of all of the above).
	cat $(RELEASE_DIR)/core-components.yaml > $(RELEASE_DIR)/cluster-api-components.yaml
	echo "---" >> $(RELEASE_DIR)/cluster-api-components.yaml
	cat $(RELEASE_DIR)/bootstrap-components.yaml >> $(RELEASE_DIR)/cluster-api-components.yaml
	echo "---" >> $(RELEASE_DIR)/cluster-api-components.yaml
	cat $(RELEASE_DIR)/control-plane-components.yaml >> $(RELEASE_DIR)/cluster-api-components.yaml
	# Add metadata to the release artifacts
	cp metadata.yaml $(RELEASE_DIR)/metadata.yaml

.PHONY: release-manifests-dev
release-manifests-dev: ## Build the development manifests and copies them in the release folder
	# Release CAPD components and add them to the release dir
	$(MAKE) -C $(CAPD_DIR) release
	cp $(CAPD_DIR)/out/infrastructure-components.yaml $(RELEASE_DIR)/infrastructure-components-development.yaml
	# Adds CAPD templates
	cp $(CAPD_DIR)/templates/* $(RELEASE_DIR)/

release-binaries: ## Build the binaries to publish with a release
	RELEASE_BINARY=clusterctl-linux-amd64 BUILD_PATH=./cmd/clusterctl GOOS=linux GOARCH=amd64 $(MAKE) release-binary
	RELEASE_BINARY=clusterctl-linux-arm64 BUILD_PATH=./cmd/clusterctl GOOS=linux GOARCH=arm64 $(MAKE) release-binary
	RELEASE_BINARY=clusterctl-darwin-amd64 BUILD_PATH=./cmd/clusterctl GOOS=darwin GOARCH=amd64 $(MAKE) release-binary
	RELEASE_BINARY=clusterctl-darwin-arm64 BUILD_PATH=./cmd/clusterctl GOOS=darwin GOARCH=arm64 $(MAKE) release-binary
	RELEASE_BINARY=clusterctl-windows-amd64.exe BUILD_PATH=./cmd/clusterctl GOOS=windows GOARCH=amd64 $(MAKE) release-binary

release-binary: $(RELEASE_DIR)
	docker run \
		--rm \
		-e CGO_ENABLED=0 \
		-e GOOS=$(GOOS) \
		-e GOARCH=$(GOARCH) \
		-v "$$(pwd):/workspace$(DOCKER_VOL_OPTS)" \
		-w /workspace \
		golang:$(GO_VERSION) \
		go build -a -trimpath -ldflags "$(LDFLAGS) -extldflags '-static'" \
		-o $(RELEASE_DIR)/$(notdir $(RELEASE_BINARY)) $(BUILD_PATH)

.PHONY: release-staging
release-staging: ## Build and push container images to the staging bucket
	REGISTRY=$(STAGING_REGISTRY) $(MAKE) docker-build-all docker-push-all release-alias-tag

.PHONY: release-staging-nightly
release-staging-nightly: ## Tag and push container images to the staging bucket. Example image tag: cluster-api-controller:nightly_main_20210121
	$(eval NEW_RELEASE_ALIAS_TAG := nightly_$(RELEASE_ALIAS_TAG)_$(shell date +'%Y%m%d'))
	echo $(NEW_RELEASE_ALIAS_TAG)
	$(MAKE) release-alias-tag TAG=$(RELEASE_ALIAS_TAG) RELEASE_ALIAS_TAG=$(NEW_RELEASE_ALIAS_TAG)
	# Set the manifest image to the staging bucket.
	$(MAKE) manifest-modification REGISTRY=$(STAGING_REGISTRY) RELEASE_TAG=$(NEW_RELEASE_ALIAS_TAG)
	## Build the manifests
	$(MAKE) release-manifests
	# Example manifest location: artifacts.k8s-staging-cluster-api.appspot.com/components/nightly_main_20210121/bootstrap-components.yaml
	gsutil cp $(RELEASE_DIR)/* gs://$(STAGING_BUCKET)/components/$(NEW_RELEASE_ALIAS_TAG)

.PHONY: release-alias-tag
release-alias-tag: ## Add the release alias tag to the last build tag
	gcloud container images add-tag $(CONTROLLER_IMG):$(TAG) $(CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG) $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(TAG) $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)

.PHONY: release-notes
release-notes: $(RELEASE_NOTES_DIR) $(RELEASE_NOTES)
	if [ -n "${PRE_RELEASE}" ]; then \
	echo ":rotating_light: This is a RELEASE CANDIDATE. Use it only for testing purposes. If you find any bugs, file an [issue](https://github.com/kubernetes-sigs/cluster-api/issues/new)." > $(RELEASE_NOTES_DIR)/$(RELEASE_TAG).md; \
	else \
	go run ./hack/tools/release/notes.go --from=$(PREVIOUS_TAG) > $(RELEASE_NOTES_DIR)/$(RELEASE_TAG).md; \
	fi

.PHONY: promote-images
promote-images: $(KPROMO)
	$(KPROMO) pr --project cluster-api --tag $(RELEASE_TAG) --reviewers "$(IMAGE_REVIEWERS)" --fork $(USER_FORK)

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-push
docker-push: ## Push the docker images
	docker push $(CONTROLLER_IMG)-$(ARCH):$(TAG)
	docker push $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH):$(TAG)
	docker push $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH):$(TAG)

.PHONY: docker-push-all
docker-push-all: $(addprefix docker-push-,$(ALL_ARCH))  ## Push all the architecture docker images
	$(MAKE) docker-push-manifest-core
	$(MAKE) docker-push-manifest-kubeadm-bootstrap
	$(MAKE) docker-push-manifest-kubeadm-control-plane

docker-push-%:
	$(MAKE) ARCH=$* docker-push

.PHONY: docker-push-manifest-core
docker-push-manifest-core: ## Push the multiarch manifest for the core docker images
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CONTROLLER_IMG}:${TAG} ${CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./config/default/manager_pull_policy.yaml"

.PHONY: docker-push-manifest-kubeadm-bootstrap
docker-push-manifest-kubeadm-bootstrap: ## Push the multiarch manifest for the the kubeadm bootstrap docker images
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${KUBEADM_BOOTSTRAP_CONTROLLER_IMG}:${TAG} ${KUBEADM_BOOTSTRAP_CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: docker-push-manifest-kubeadm-control-plane
docker-push-manifest-kubeadm-control-plane: ## Push the multiarch manifest for the the kubeadm control plane docker images
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
## Cleanup / Verification
## --------------------------------------

##@ clean:

.PHONY: clean
clean: ## Remove all generated files
	$(MAKE) clean-bin
	$(MAKE) clean-book

.PHONY: clean-bin
clean-bin: ## Remove all generated binaries
	rm -rf $(BIN_DIR)
	rm -rf $(TOOLS_BIN_DIR)

.PHONY: clean-book
clean-book: ## Remove all generated GitBook files
	rm -rf ./docs/book/_book

.PHONY: clean-release
clean-release: ## Remove the release folder
	rm -rf $(RELEASE_DIR)

.PHONY: clean-manifests ## Reset manifests in config directories back to main
clean-manifests:
	@read -p "WARNING: This will reset all config directories to local main. Press [ENTER] to continue."
	git checkout main config bootstrap/kubeadm/config controlplane/kubeadm/config $(CAPD_DIR)/config

.PHONY: clean-release-git
clean-release-git: ## Restores the git files usually modified during a release
	git restore ./*manager_image_patch.yaml ./*manager_pull_policy.yaml

.PHONY: clean-generated-conversions
clean-generated-conversions: ## Remove files generated by conversion-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	(IFS=','; for i in $(SRC_DIRS); do find $$i -type f -name 'zz_generated.conversion*' -exec rm -f {} \;; done)

## --------------------------------------
## Hack / Tools
## --------------------------------------

##@ hack/tools:

.PHONY: $(CONTROLLER_GEN_BIN)
$(CONTROLLER_GEN_BIN): $(CONTROLLER_GEN) ## Build a local copy of controller-gen.

.PHONY: $(CONVERSION_GEN_BIN)
$(CONVERSION_GEN_BIN): $(CONVERSION_GEN) ## Build a local copy of conversion-gen.

.PHONY: $(CONVERSION_VERIFIER_BIN)
$(CONVERSION_VERIFIER_BIN): $(CONVERSION_VERIFIER) ## Build a local copy of conversion-verifier.

.PHONY: $(GOTESTSUM_BIN)
$(GOTESTSUM_BIN): $(GOTESTSUM) ## Build a local copy of gotestsum.

.PHONY: $(GO_APIDIFF_BIN)
$(GO_APIDIFF_BIN): $(GO_APIDIFF) ## Build a local copy of go-apidiff

.PHONY: $(ENVSUBST_BIN)
$(ENVSUBST_BIN): $(ENVSUBST) ## Build a local copy of envsubst.

.PHONY: $(KUSTOMIZE_BIN)
$(KUSTOMIZE_BIN): $(KUSTOMIZE) ## Build a local copy of kustomize.

.PHONY: $(SETUP_ENVTEST_BIN)
$(SETUP_ENVTEST_BIN): $(SETUP_ENVTEST) ## Build a local copy of setup-envtest.

.PHONY: $(KPROMO_BIN)
$(KPROMO_BIN): $(KPROMO) ## Build a local copy of kpromo

.PHONY: $(TILT_PREPARE_BIN)
$(TILT_PREPARE_BIN): $(TILT_PREPARE) ## Build a local copy of tilt-prepare.

.PHONY: $(GOLANGCI_LINT_BIN)
$(GOLANGCI_LINT_BIN): $(GOLANGCI_LINT) ## Build a local copy of golangci-lint

$(CONTROLLER_GEN): # Build controller-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(CONTROLLER_GEN_PKG) $(CONTROLLER_GEN_BIN) $(CONTROLLER_GEN_VER)

## We are forcing a rebuilt of conversion-gen via PHONY so that we're always using an up-to-date version.
## We can't use a versioned name for the binary, because that would be reflected in generated files.
.PHONY: $(CONVERSION_GEN)
$(CONVERSION_GEN): # Build conversion-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(CONVERSION_GEN_PKG) $(CONVERSION_GEN_BIN) $(CONVERSION_GEN_VER)

$(CONVERSION_VERIFIER): $(TOOLS_DIR)/go.mod # Build conversion-verifier from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/conversion-verifier sigs.k8s.io/cluster-api/hack/tools/conversion-verifier

$(GOTESTSUM): # Build gotestsum from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(GOTESTSUM_PKG) $(GOTESTSUM_BIN) $(GOTESTSUM_VER)

$(GO_APIDIFF): # Build go-apidiff from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(GO_APIDIFF_PKG) $(GO_APIDIFF_BIN) $(GO_APIDIFF_VER)

$(ENVSUBST): # Build gotestsum from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(ENVSUBST_PKG) $(ENVSUBST_BIN) $(ENVSUBST_VER)

$(KUSTOMIZE): # Build kustomize from tools folder.
	CGO_ENABLED=0 GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(KUSTOMIZE_PKG) $(KUSTOMIZE_BIN) $(KUSTOMIZE_VER)

$(SETUP_ENVTEST): # Build setup-envtest from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(SETUP_ENVTEST_PKG) $(SETUP_ENVTEST_BIN) $(SETUP_ENVTEST_VER)

$(TILT_PREPARE): $(TOOLS_DIR)/go.mod # Build tilt-prepare from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/tilt-prepare sigs.k8s.io/cluster-api/hack/tools/tilt-prepare

$(KPROMO):
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(KPROMO_PKG) $(KPROMO_BIN) ${KPROMO_VER}

$(GOLANGCI_LINT): .github/workflows/golangci-lint.yml # Download golangci-lint using hack script into tools folder.
	hack/ensure-golangci-lint.sh \
		-b $(TOOLS_BIN_DIR) \
		$(shell cat .github/workflows/golangci-lint.yml | grep version | sed 's/.*version: //')
