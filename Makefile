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
GO_VERSION ?= 1.23.0
GO_DIRECTIVE_VERSION ?= 1.23.0
GO_CONTAINER_IMAGE ?= docker.io/library/golang:$(GO_VERSION)

# Ensure correct toolchain is used
GOTOOLCHAIN = go$(GO_VERSION)
export GOTOOLCHAIN

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
export KUBEBUILDER_ENVTEST_KUBERNETES_VERSION ?= 1.31.0
export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?= 60s
export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?= 60s

# This option is for running docker manifest command
export DOCKER_CLI_EXPERIMENTAL := enabled

# Enables shell script tracing. Enable by running: TRACE=1 make <target>
TRACE ?= 0

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
DOCS_DIR := docs
E2E_FRAMEWORK_DIR := $(TEST_DIR)/framework
CAPD_DIR := $(TEST_DIR)/infrastructure/docker
CAPIM_DIR := $(TEST_DIR)/infrastructure/inmemory
TEST_EXTENSION_DIR := $(TEST_DIR)/extension
GO_INSTALL := ./scripts/go_install.sh
OBSERVABILITY_DIR := hack/observability

export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

#
# Ginkgo configuration.
#
GINKGO_FOCUS ?=
GINKGO_SKIP ?=
GINKGO_NODES ?= 1
GINKGO_TIMEOUT ?= 2h
GINKGO_POLL_PROGRESS_AFTER ?= 60m
GINKGO_POLL_PROGRESS_INTERVAL ?= 5m
E2E_CONF_FILE ?= $(ROOT_DIR)/$(TEST_DIR)/e2e/config/docker.yaml
SKIP_RESOURCE_CLEANUP ?= false
USE_EXISTING_CLUSTER ?= false
GINKGO_NOCOLOR ?= false

# to set multiple ginkgo skip flags, if any
ifneq ($(strip $(GINKGO_SKIP)),)
_SKIP_ARGS := $(foreach arg,$(strip $(GINKGO_SKIP)),-skip="$(arg)")
endif

# Helper function to get dependency version from go.mod
get_go_version = $(shell go list -m $1 | awk '{print $$2}')

#
# Binaries.
#
# Note: Need to use abspath so we can invoke these from subdirectories
KUSTOMIZE_VER := v5.3.0
KUSTOMIZE_BIN := kustomize
KUSTOMIZE := $(abspath $(TOOLS_BIN_DIR)/$(KUSTOMIZE_BIN)-$(KUSTOMIZE_VER))
KUSTOMIZE_PKG := sigs.k8s.io/kustomize/kustomize/v5

SETUP_ENVTEST_VER := release-0.19
SETUP_ENVTEST_BIN := setup-envtest
SETUP_ENVTEST := $(abspath $(TOOLS_BIN_DIR)/$(SETUP_ENVTEST_BIN)-$(SETUP_ENVTEST_VER))
SETUP_ENVTEST_PKG := sigs.k8s.io/controller-runtime/tools/setup-envtest

CONTROLLER_GEN_VER := v0.17.0
CONTROLLER_GEN_BIN := controller-gen
CONTROLLER_GEN := $(abspath $(TOOLS_BIN_DIR)/$(CONTROLLER_GEN_BIN)-$(CONTROLLER_GEN_VER))
CONTROLLER_GEN_PKG := sigs.k8s.io/controller-tools/cmd/controller-gen

GOTESTSUM_VER := v1.11.0
GOTESTSUM_BIN := gotestsum
GOTESTSUM := $(abspath $(TOOLS_BIN_DIR)/$(GOTESTSUM_BIN)-$(GOTESTSUM_VER))
GOTESTSUM_PKG := gotest.tools/gotestsum

CONVERSION_GEN_VER := v0.32.0
CONVERSION_GEN_BIN := conversion-gen
# We are intentionally using the binary without version suffix, to avoid the version
# in generated files.
CONVERSION_GEN := $(abspath $(TOOLS_BIN_DIR)/$(CONVERSION_GEN_BIN))
CONVERSION_GEN_PKG := k8s.io/code-generator/cmd/conversion-gen

ENVSUBST_BIN := envsubst
ENVSUBST_VER := $(call get_go_version,github.com/drone/envsubst/v2)
ENVSUBST := $(abspath $(TOOLS_BIN_DIR)/$(ENVSUBST_BIN)-$(ENVSUBST_VER))
ENVSUBST_PKG := github.com/drone/envsubst/v2/cmd/envsubst

GO_APIDIFF_VER := v0.8.2
GO_APIDIFF_BIN := go-apidiff
GO_APIDIFF := $(abspath $(TOOLS_BIN_DIR)/$(GO_APIDIFF_BIN)-$(GO_APIDIFF_VER))
GO_APIDIFF_PKG := github.com/joelanford/go-apidiff

HADOLINT_VER := v2.12.0
HADOLINT_FAILURE_THRESHOLD = warning

SHELLCHECK_VER := v0.9.0

TRIVY_VER := 0.49.1

KPROMO_VER := 5ab0dbc74b0228c22a93d240596dff77464aee8f
KPROMO_BIN := kpromo
KPROMO :=  $(abspath $(TOOLS_BIN_DIR)/$(KPROMO_BIN)-$(KPROMO_VER))
# KPROMO_PKG may have to be changed if KPROMO_VER increases its major version.
KPROMO_PKG := sigs.k8s.io/promo-tools/v4/cmd/kpromo

YQ_VER := v4.35.2
YQ_BIN := yq
YQ :=  $(abspath $(TOOLS_BIN_DIR)/$(YQ_BIN)-$(YQ_VER))
YQ_PKG := github.com/mikefarah/yq/v4

PLANTUML_VER := 1.2024.3

GINKGO_BIN := ginkgo
GINKGO_VER := $(call get_go_version,github.com/onsi/ginkgo/v2)
GINKGO := $(abspath $(TOOLS_BIN_DIR)/$(GINKGO_BIN)-$(GINKGO_VER))
GINKGO_PKG := github.com/onsi/ginkgo/v2/ginkgo

GOLANGCI_LINT_BIN := golangci-lint
GOLANGCI_LINT_VER := $(shell cat .github/workflows/pr-golangci-lint.yaml | grep [[:space:]]version: | sed 's/.*version: //')
GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN)-$(GOLANGCI_LINT_VER))
GOLANGCI_LINT_PKG := github.com/golangci/golangci-lint/cmd/golangci-lint

GOVULNCHECK_BIN := govulncheck
GOVULNCHECK_VER := v1.0.4
GOVULNCHECK := $(abspath $(TOOLS_BIN_DIR)/$(GOVULNCHECK_BIN)-$(GOVULNCHECK_VER))
GOVULNCHECK_PKG := golang.org/x/vuln/cmd/govulncheck

IMPORT_BOSS_BIN := import-boss
IMPORT_BOSS_VER := v0.28.1
IMPORT_BOSS := $(abspath $(TOOLS_BIN_DIR)/$(IMPORT_BOSS_BIN))
IMPORT_BOSS_PKG := k8s.io/code-generator/cmd/import-boss

TRIAGE_PARTY_IMAGE_NAME ?= extra/triage-party
TRIAGE_PARTY_CONTROLLER_IMG ?= $(STAGING_REGISTRY)/$(TRIAGE_PARTY_IMAGE_NAME)
TRIAGE_PARTY_DIR := hack/tools/triage
TRIAGE_PARTY_TMP_DIR ?= $(TRIAGE_PARTY_DIR)/triage-party.tmp
TRIAGE_PARTY_VERSION ?= v1.6.0

CONVERSION_VERIFIER_BIN := conversion-verifier
CONVERSION_VERIFIER := $(abspath $(TOOLS_BIN_DIR)/$(CONVERSION_VERIFIER_BIN))

OPENAPI_GEN_VER := 2c72e55 # main branch as of 03.01.2025
OPENAPI_GEN_BIN := openapi-gen
# We are intentionally using the binary without version suffix, to avoid the version
# in generated files.
OPENAPI_GEN := $(abspath $(TOOLS_BIN_DIR)/$(OPENAPI_GEN_BIN))
OPENAPI_GEN_PKG := k8s.io/kube-openapi/cmd/openapi-gen

PROWJOB_GEN_BIN := prowjob-gen
PROWJOB_GEN := $(abspath $(TOOLS_BIN_DIR)/$(PROWJOB_GEN_BIN))

RUNTIME_OPENAPI_GEN_BIN := runtime-openapi-gen
RUNTIME_OPENAPI_GEN := $(abspath $(TOOLS_BIN_DIR)/$(RUNTIME_OPENAPI_GEN_BIN))

TILT_PREPARE_BIN := tilt-prepare
TILT_PREPARE := $(abspath $(TOOLS_BIN_DIR)/$(TILT_PREPARE_BIN))

# Define Docker related variables. Releases should modify and double check these vars.
REGISTRY ?= gcr.io/$(shell gcloud config get-value project)
PROD_REGISTRY ?= registry.k8s.io/cluster-api

STAGING_REGISTRY ?= gcr.io/k8s-staging-cluster-api
STAGING_BUCKET ?= k8s-staging-cluster-api

# core
IMAGE_NAME ?= cluster-api-controller
CONTROLLER_IMG ?= $(REGISTRY)/$(IMAGE_NAME)

# bootstrap
KUBEADM_BOOTSTRAP_IMAGE_NAME ?= kubeadm-bootstrap-controller
KUBEADM_BOOTSTRAP_CONTROLLER_IMG ?= $(REGISTRY)/$(KUBEADM_BOOTSTRAP_IMAGE_NAME)

# control plane
KUBEADM_CONTROL_PLANE_IMAGE_NAME ?= kubeadm-control-plane-controller
KUBEADM_CONTROL_PLANE_CONTROLLER_IMG ?= $(REGISTRY)/$(KUBEADM_CONTROL_PLANE_IMAGE_NAME)

# capd
CAPD_IMAGE_NAME ?= capd-manager
CAPD_CONTROLLER_IMG ?= $(REGISTRY)/$(CAPD_IMAGE_NAME)

# capim
CAPIM_IMAGE_NAME ?= capim-manager
CAPIM_CONTROLLER_IMG ?= $(REGISTRY)/$(CAPIM_IMAGE_NAME)

# clusterctl
CLUSTERCTL_MANIFEST_DIR := cmd/clusterctl/config
CLUSTERCTL_IMAGE_NAME ?= clusterctl
CLUSTERCTL_IMG ?= $(REGISTRY)/$(CLUSTERCTL_IMAGE_NAME)

# test extension
TEST_EXTENSION_IMAGE_NAME ?= test-extension
TEST_EXTENSION_IMG ?= $(REGISTRY)/$(TEST_EXTENSION_IMAGE_NAME)

# kind
CAPI_KIND_CLUSTER_NAME ?= capi-test

# It is set by Prow GIT_TAG, a git-based tag of the form vYYYYMMDD-hash, e.g., v20210120-v0.3.10-308-gc61521971

TAG ?= dev
ARCH ?= $(shell go env GOARCH)
ALL_ARCH ?= amd64 arm arm64 ppc64le s390x

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
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[0-9A-Za-z_-]+:.*?##/ { printf "  \033[36m%-50s\033[0m %s\n", $$1, $$2 } /^\$$\([0-9A-Za-z_-]+\):.*?##/ { gsub("_","-", $$1); printf "  \033[36m%-50s\033[0m %s\n", tolower(substr($$1, 3, length($$1)-7)), $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Generate / Manifests
## --------------------------------------

##@ generate:

ALL_GENERATE_MODULES = core kubeadm-bootstrap kubeadm-control-plane docker-infrastructure in-memory-infrastructure test-extension

.PHONY: generate
generate: ## Run all generate-manifests-*, generate-go-deepcopy-*, generate-go-conversions-* and generate-go-openapi targets
	$(MAKE) generate-modules generate-manifests generate-go-deepcopy generate-go-conversions generate-go-openapi generate-metrics-config

.PHONY: generate-manifests
generate-manifests: $(addprefix generate-manifests-,$(ALL_GENERATE_MODULES)) ## Run all generate-manifests-* targets

.PHONY: generate-manifests-core
generate-manifests-core: $(CONTROLLER_GEN) $(KUSTOMIZE) ## Generate manifests e.g. CRD, RBAC etc. for core
	$(MAKE) clean-generated-yaml SRC_DIRS="./config/crd/bases,./config/webhook/manifests.yaml"
	$(CONTROLLER_GEN) \
		paths=./ \
		paths=./api/... \
		paths=./internal/apis/core/... \
		paths=./internal/controllers/... \
		paths=./internal/webhooks/... \
		paths=./$(EXP_DIR)/api/... \
		paths=./$(EXP_DIR)/internal/controllers/... \
		paths=./$(EXP_DIR)/internal/webhooks/... \
		paths=./$(EXP_DIR)/addons/api/... \
		paths=./$(EXP_DIR)/addons/internal/controllers/... \
		paths=./$(EXP_DIR)/addons/internal/webhooks/... \
		paths=./$(EXP_DIR)/ipam/api/... \
		paths=./$(EXP_DIR)/ipam/internal/webhooks/... \
		paths=./$(EXP_DIR)/runtime/api/... \
		paths=./$(EXP_DIR)/runtime/internal/controllers/... \
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
	$(CONTROLLER_GEN) \
		paths=./util/test/builder/... \
		crd:crdVersions=v1 \
		output:crd:dir=./util/test/builder/crd

.PHONY: generate-manifests-kubeadm-bootstrap
generate-manifests-kubeadm-bootstrap: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for kubeadm bootstrap
	$(MAKE) clean-generated-yaml SRC_DIRS="./bootstrap/kubeadm/config/crd/bases,./bootstrap/kubeadm/config/webhook/manifests.yaml"
	$(CONTROLLER_GEN) \
		paths=./bootstrap/kubeadm \
		paths=./bootstrap/kubeadm/api/... \
		paths=./bootstrap/kubeadm/internal/controllers/... \
		paths=./bootstrap/kubeadm/internal/webhooks/... \
		paths=./internal/apis/bootstrap/kubeadm/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./bootstrap/kubeadm/config/crd/bases \
		output:rbac:dir=./bootstrap/kubeadm/config/rbac \
		output:webhook:dir=./bootstrap/kubeadm/config/webhook \
		webhook

.PHONY: generate-manifests-kubeadm-control-plane
generate-manifests-kubeadm-control-plane: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for kubeadm control plane
	$(MAKE) clean-generated-yaml SRC_DIRS="./controlplane/kubeadm/config/crd/bases,./controlplane/kubeadm/config/webhook/manifests.yaml"
	$(CONTROLLER_GEN) \
		paths=./controlplane/kubeadm \
		paths=./controlplane/kubeadm/api/... \
		paths=./controlplane/kubeadm/internal/controllers/... \
		paths=./controlplane/kubeadm/internal/webhooks/... \
		paths=./internal/apis/controlplane/kubeadm/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./controlplane/kubeadm/config/crd/bases \
		output:rbac:dir=./controlplane/kubeadm/config/rbac \
		output:webhook:dir=./controlplane/kubeadm/config/webhook \
		webhook

.PHONY: generate-manifests-docker-infrastructure
generate-manifests-docker-infrastructure: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for docker infrastructure provider
	$(MAKE) clean-generated-yaml SRC_DIRS="$(CAPD_DIR)/config/crd/bases,$(CAPD_DIR)/config/webhook/manifests.yaml"
	cd $(CAPD_DIR); $(CONTROLLER_GEN) \
		paths=./ \
		paths=./api/... \
		paths=./$(EXP_DIR)/api/... \
		paths=./$(EXP_DIR)/internal/controllers/... \
		paths=./$(EXP_DIR)/internal/webhooks/... \
		paths=./internal/controllers/... \
		paths=./internal/webhooks/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./config/crd/bases \
		output:webhook:dir=./config/webhook \
		webhook


.PHONY: generate-manifests-in-memory-infrastructure
generate-manifests-in-memory-infrastructure: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for in-memory infrastructure provider
	$(MAKE) clean-generated-yaml SRC_DIRS="$(CAPIM_DIR)/config/crd/bases,$(CAPIM_DIR)/config/webhook/manifests.yaml"
	cd $(CAPIM_DIR); $(CONTROLLER_GEN) \
		paths=./ \
		paths=./api/... \
		paths=./internal/controllers/... \
		paths=./internal/webhooks/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./config/crd/bases \
		output:webhook:dir=./config/webhook \
		webhook

.PHONY: generate-manifests-test-extension
generate-manifests-test-extension: $(CONTROLLER_GEN) ## Generate manifests e.g. RBAC for test-extension provider
	cd ./test/extension; $(CONTROLLER_GEN) \
		paths=./... \
		output:rbac:dir=./config/rbac \
		rbac:roleName=manager-role

.PHONY: generate-go-deepcopy
generate-go-deepcopy:  ## Run all generate-go-deepcopy-* targets
	$(MAKE) $(addprefix generate-go-deepcopy-,$(ALL_GENERATE_MODULES))

.PHONY: generate-go-deepcopy-core
generate-go-deepcopy-core: $(CONTROLLER_GEN) ## Generate deepcopy go code for core
	$(MAKE) clean-generated-deepcopy SRC_DIRS="./api,./$(EXP_DIR)/api,./$(EXP_DIR)/addons/api,./$(EXP_DIR)/runtime/api,./$(EXP_DIR)/runtime/hooks/api"
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./api/... \
		paths=./$(EXP_DIR)/api/... \
		paths=./$(EXP_DIR)/addons/api/... \
		paths=./$(EXP_DIR)/ipam/api/... \
		paths=./$(EXP_DIR)/runtime/api/... \
		paths=./$(EXP_DIR)/runtime/hooks/api/... \
		paths=./internal/runtime/test/... \
		paths=./cmd/clusterctl/... \
		paths=./util/test/builder/...

.PHONY: generate-go-deepcopy-kubeadm-bootstrap
generate-go-deepcopy-kubeadm-bootstrap: $(CONTROLLER_GEN) ## Generate deepcopy go code for kubeadm bootstrap
	$(MAKE) clean-generated-deepcopy SRC_DIRS="./bootstrap/kubeadm/api,./bootstrap/kubeadm/types"
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./bootstrap/kubeadm/api/... \
		paths=./bootstrap/kubeadm/types/...

.PHONY: generate-go-deepcopy-kubeadm-control-plane
generate-go-deepcopy-kubeadm-control-plane: $(CONTROLLER_GEN) ## Generate deepcopy go code for kubeadm control plane
	$(MAKE) clean-generated-deepcopy SRC_DIRS="./controlplane/kubeadm/api"
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./controlplane/kubeadm/api/...

.PHONY: generate-go-deepcopy-docker-infrastructure
generate-go-deepcopy-docker-infrastructure: $(CONTROLLER_GEN) ## Generate deepcopy go code for docker infrastructure provider
	$(MAKE) clean-generated-deepcopy SRC_DIRS="$(CAPD_DIR)/api,$(CAPD_DIR)/$(EXP_DIR)/api"
	cd $(CAPD_DIR); $(CONTROLLER_GEN) \
		object:headerFile=../../../hack/boilerplate/boilerplate.generatego.txt \
		paths=./api/... \
		paths=./$(EXP_DIR)/api/...

.PHONY: generate-go-deepcopy-in-memory-infrastructure
generate-go-deepcopy-in-memory-infrastructure: $(CONTROLLER_GEN) ## Generate deepcopy go code for in-memory infrastructure provider
	$(MAKE) clean-generated-deepcopy SRC_DIRS="$(CAPIM_DIR)/api,$(CAPIM_DIR)/internal/cloud/api"
	cd $(CAPIM_DIR); $(CONTROLLER_GEN) \
		object:headerFile=../../../hack/boilerplate/boilerplate.generatego.txt \
		paths=./api/... \
        paths=./internal/cloud/api/...

.PHONY: generate-go-deepcopy-test-extension
generate-go-deepcopy-test-extension: $(CONTROLLER_GEN) ## Generate deepcopy go code for test-extension

.PHONY: generate-go-conversions
generate-go-conversions: ## Run all generate-go-conversions-* targets
	$(MAKE) $(addprefix generate-go-conversions-,$(ALL_GENERATE_MODULES))

.PHONY: generate-go-conversions-core
generate-go-conversions-core: ## Run all generate-go-conversions-core-* targets
	$(MAKE) generate-go-conversions-core-api
	$(MAKE) generate-go-conversions-core-exp
	$(MAKE) generate-go-conversions-core-exp-ipam
	$(MAKE) generate-go-conversions-core-runtime

.PHONY: generate-go-conversions-core-api
generate-go-conversions-core-api: $(CONVERSION_GEN) ## Generate conversions go code for core api
	$(MAKE) clean-generated-conversions SRC_DIRS="./internal/apis/core/v1alpha3,./internal/apis/core/v1alpha4"
	$(CONVERSION_GEN) \
		--output-file=zz_generated.conversion.go \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt \
		./internal/apis/core/v1alpha3 \
		./internal/apis/core/v1alpha4

.PHONY: generate-go-conversions-core-exp
generate-go-conversions-core-exp: $(CONVERSION_GEN) ## Generate conversions go code for core exp
	$(MAKE) clean-generated-conversions SRC_DIRS="./internal/apis/core/exp/v1alpha3,./internal/apis/core/exp/addons/v1alpha3,./internal/apis/core/exp/v1alpha4,./internal/apis/core/exp/addons/v1alpha4"
	$(CONVERSION_GEN) \
		--output-file=zz_generated.conversion.go \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt \
		./internal/apis/core/exp/v1alpha3 \
		./internal/apis/core/exp/v1alpha4 \
		./internal/apis/core/exp/addons/v1alpha3 \
		./internal/apis/core/exp/addons/v1alpha4

.PHONY: generate-go-conversions-core-exp-ipam
generate-go-conversions-core-exp-ipam: $(CONVERSION_GEN) ## Generate conversions go code for core exp IPAM
	$(MAKE) clean-generated-conversions SRC_DIRS="./$(EXP_DIR)/ipam/api/v1alpha1"
	$(CONVERSION_GEN) \
		--output-file=zz_generated.conversion.go \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt \
		./$(EXP_DIR)/ipam/api/v1alpha1

.PHONY: generate-go-conversions-core-runtime
generate-go-conversions-core-runtime: $(CONVERSION_GEN) ## Generate conversions go code for core runtime
	$(MAKE) clean-generated-conversions SRC_DIRS="./internal/runtime/test/v1alpha1,./internal/runtime/test/v1alpha2"
	$(CONVERSION_GEN) \
		--output-file=zz_generated.conversion.go \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt \
		./internal/runtime/test/v1alpha1 \
		./internal/runtime/test/v1alpha2

.PHONY: generate-go-conversions-kubeadm-bootstrap
generate-go-conversions-kubeadm-bootstrap: $(CONVERSION_GEN) ## Generate conversions go code for kubeadm bootstrap
	$(MAKE) clean-generated-conversions SRC_DIRS="./internal/apis/bootstrap/kubeadm"
	$(CONVERSION_GEN) \
		--output-file=zz_generated.conversion.go \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt \
		./internal/apis/bootstrap/kubeadm/v1alpha3 \
		./internal/apis/bootstrap/kubeadm/v1alpha4
	$(MAKE) clean-generated-conversions SRC_DIRS="./bootstrap/kubeadm/types/upstreamv1beta1,./bootstrap/kubeadm/types/upstreamv1beta2,./bootstrap/kubeadm/types/upstreamv1beta3,./bootstrap/kubeadm/types/upstreamv1beta4"
	$(CONVERSION_GEN) \
		--output-file=zz_generated.conversion.go \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt \
		./bootstrap/kubeadm/types/upstreamv1beta1 \
		./bootstrap/kubeadm/types/upstreamv1beta2 \
		./bootstrap/kubeadm/types/upstreamv1beta3 \
		./bootstrap/kubeadm/types/upstreamv1beta4

.PHONY: generate-go-conversions-kubeadm-control-plane
generate-go-conversions-kubeadm-control-plane: $(CONVERSION_GEN) ## Generate conversions go code for kubeadm control plane
	$(MAKE) clean-generated-conversions SRC_DIRS="./internal/apis/controlplane/kubeadm"
	$(CONVERSION_GEN) \
		--output-file=zz_generated.conversion.go \
		--go-header-file=./hack/boilerplate/boilerplate.generatego.txt \
		./internal/apis/controlplane/kubeadm/v1alpha3 \
		./internal/apis/controlplane/kubeadm/v1alpha4

.PHONY: generate-go-conversions-docker-infrastructure
generate-go-conversions-docker-infrastructure: $(CONVERSION_GEN) ## Generate conversions go code for docker infrastructure provider
	cd $(CAPD_DIR); $(CONVERSION_GEN) \
		--output-file=zz_generated.conversion.go \
		--go-header-file=../../../hack/boilerplate/boilerplate.generatego.txt \
		./api/v1alpha3 \
		./api/v1alpha4 \
		./$(EXP_DIR)/api/v1alpha3 \
		./$(EXP_DIR)/api/v1alpha4

.PHONY: generate-go-conversions-in-memory-infrastructure
generate-go-conversions-in-memory-infrastructure: $(CONVERSION_GEN) ## Generate conversions go code for in-memory infrastructure provider
	cd $(CAPIM_DIR)

.PHONY: generate-go-conversions-test-extension
generate-go-conversions-test-extension: $(CONVERSION_GEN) ## Generate conversions go code for in-memory infrastructure provider

# The tmp/sigs.k8s.io/cluster-api symlink is a workaround to make this target run outside of GOPATH
.PHONY: generate-go-openapi
generate-go-openapi: $(OPENAPI_GEN) ## Generate openapi go code for runtime SDK
	@mkdir -p ./tmp/sigs.k8s.io; ln -s $(ROOT_DIR) ./tmp/sigs.k8s.io/; cd ./tmp; \
	for pkg in "api/v1beta1" "$(EXP_DIR)/runtime/hooks/api/v1alpha1"; do \
		(cd ../ && $(MAKE) clean-generated-openapi-definitions SRC_DIRS="./$${pkg}"); \
		echo "** Generating openapi schema for types in ./$${pkg} **"; \
		$(OPENAPI_GEN) \
			--output-dir=../$${pkg} \
			--output-file=zz_generated.openapi.go \
			--output-pkg=sigs.k8s.io/cluster-api/$${pkg} \
			--go-header-file=../hack/boilerplate/boilerplate.generatego.txt \
			sigs.k8s.io/cluster-api/$${pkg}; \
	done; \
	rm sigs.k8s.io/cluster-api

.PHONY: generate-modules
generate-modules: ## Run go mod tidy to ensure modules are up to date
	go mod tidy
	cd $(TOOLS_DIR); go mod tidy
	cd $(TEST_DIR); go mod tidy

.PHONY: generate-doctoc
generate-doctoc:
	TRACE=$(TRACE) ./hack/generate-doctoc.sh

.PHONY: generate-e2e-templates
generate-e2e-templates: $(KUSTOMIZE) $(addprefix generate-e2e-templates-, v0.3 v0.4 v1.0 v1.5 v1.6 v1.7 v1.8 main) ## Generate cluster templates for all versions

DOCKER_TEMPLATES := test/e2e/data/infrastructure-docker
INMEMORY_TEMPLATES := test/e2e/data/infrastructure-inmemory

.PHONY: generate-e2e-templates-v0.3
generate-e2e-templates-v0.3: $(KUSTOMIZE)
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v0.3/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v0.3/cluster-template.yaml

.PHONY: generate-e2e-templates-v0.4
generate-e2e-templates-v0.4: $(KUSTOMIZE)
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v0.4/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v0.4/cluster-template.yaml

.PHONY: generate-e2e-templates-v1.0
generate-e2e-templates-v1.0: $(KUSTOMIZE)
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1.0/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1.0/cluster-template.yaml

.PHONY: generate-e2e-templates-v1.5
generate-e2e-templates-v1.5: $(KUSTOMIZE)
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1.5/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1.5/cluster-template.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1.5/cluster-template-topology --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1.5/cluster-template-topology.yaml

.PHONY: generate-e2e-templates-v1.6
generate-e2e-templates-v1.6: $(KUSTOMIZE)
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1.6/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1.6/cluster-template.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1.6/cluster-template-topology --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1.6/cluster-template-topology.yaml

.PHONY: generate-e2e-templates-v1.7
generate-e2e-templates-v1.7: $(KUSTOMIZE)
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1.7/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1.7/cluster-template.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1.7/cluster-template-topology --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1.7/cluster-template-topology.yaml

.PHONY: generate-e2e-templates-v1.8
generate-e2e-templates-v1.8: $(KUSTOMIZE)
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1.8/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1.8/cluster-template.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1.8/cluster-template-topology --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1.8/cluster-template-topology.yaml

.PHONY: generate-e2e-templates-main
generate-e2e-templates-main: $(KUSTOMIZE)
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-md-remediation --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-md-remediation.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-kcp-remediation --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-kcp-remediation.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-kcp-adoption/step1 --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-kcp-adoption.yaml
	echo "---" >> $(DOCKER_TEMPLATES)/main/cluster-template-kcp-adoption.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-kcp-adoption/step2 --load-restrictor LoadRestrictionsNone >> $(DOCKER_TEMPLATES)/main/cluster-template-kcp-adoption.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-machine-pool --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-machine-pool.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-upgrades --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-upgrades.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-upgrades-runtimesdk --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-upgrades-runtimesdk.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-kcp-pre-drain --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-kcp-pre-drain.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-kcp-scale-in --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-kcp-scale-in.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-ipv6 --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-ipv6.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-topology-dualstack-ipv6-primary --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-topology-dualstack-ipv6-primary.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-topology-dualstack-ipv4-primary --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-topology-dualstack-ipv4-primary.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-topology-no-workers --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-topology-no-workers.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-topology-kcp-only --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-topology-kcp-only.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-topology-autoscaler --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-topology-autoscaler.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-topology --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-topology.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/cluster-template-ignition --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/cluster-template-ignition.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/main/clusterclass-quick-start-kcp-only --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/main/clusterclass-quick-start-kcp-only.yaml

	$(KUSTOMIZE) build $(INMEMORY_TEMPLATES)/main/cluster-template --load-restrictor LoadRestrictionsNone > $(INMEMORY_TEMPLATES)/main/cluster-template.yaml

.PHONY: generate-metrics-config
generate-metrics-config: $(ENVSUBST_BIN) ## Generate ./config/metrics/crd-metrics-config.yaml
	OUTPUT_FILE="./config/metrics/crd-metrics-config.yaml"; \
	METRIC_TEMPLATES_DIR="./config/metrics/templates"; \
	echo "# This file was auto-generated via: make generate-metrics-config" > "$${OUTPUT_FILE}"; \
	cat "$${METRIC_TEMPLATES_DIR}/header.yaml" >> "$${OUTPUT_FILE}"; \
	for resource in clusterclass cluster kubeadmcontrolplane kubeadmconfig machine machinedeployment machinehealthcheck machineset machinepool; do \
		cat "$${METRIC_TEMPLATES_DIR}/$${resource}.yaml"; \
		RESOURCE="$${resource}" ${ENVSUBST_BIN} < "$${METRIC_TEMPLATES_DIR}/common_metrics.yaml"; \
		if [[ "$${resource}" != "cluster" ]]; then \
			cat "$${METRIC_TEMPLATES_DIR}/owner_metric.yaml"; \
		fi \
	done >> "$${OUTPUT_FILE}"; \

.PHONY: generate-diagrams
generate-diagrams: ## Generate diagrams for *.plantuml files
	$(MAKE) generate-diagrams-book
	$(MAKE) generate-diagrams-proposals

.PHONY: generate-diagrams-book
generate-diagrams-book: ## Generate diagrams for *.plantuml files in book
	docker run -v $(ROOT_DIR)/$(DOCS_DIR):/$(DOCS_DIR)$(DOCKER_VOL_OPTS)  plantuml/plantuml:$(PLANTUML_VER) /$(DOCS_DIR)/book/**/*.plantuml

.PHONY: generate-diagrams-proposals
generate-diagrams-proposals: ## Generate diagrams for *.plantuml files in proposals
	docker run -v $(ROOT_DIR)/$(DOCS_DIR):/$(DOCS_DIR)$(DOCKER_VOL_OPTS)  plantuml/plantuml:$(PLANTUML_VER) /$(DOCS_DIR)/proposals/**/*.plantuml

.PHONY: generate-test-infra-prowjobs
generate-test-infra-prowjobs: $(PROWJOB_GEN) ## Generates the prowjob configurations in test-infra
	@if [ -z "${TEST_INFRA_DIR}" ]; then echo "TEST_INFRA_DIR is not set"; exit 1; fi
	$(PROWJOB_GEN) \
		-config "$(TEST_INFRA_DIR)/config/jobs/kubernetes-sigs/cluster-api/cluster-api-prowjob-gen.yaml" \
		-templates-dir "$(TEST_INFRA_DIR)/config/jobs/kubernetes-sigs/cluster-api/templates" \
		-output-dir "$(TEST_INFRA_DIR)/config/jobs/kubernetes-sigs/cluster-api"

## --------------------------------------
## Lint / Verify
## --------------------------------------

##@ lint and verify:

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Lint the codebase
	$(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS)
	cd $(TEST_DIR); $(GOLANGCI_LINT) run --path-prefix $(TEST_DIR) --config $(ROOT_DIR)/.golangci.yml -v $(GOLANGCI_LINT_EXTRA_ARGS)
	cd $(TOOLS_DIR); $(GOLANGCI_LINT) run --path-prefix $(TOOLS_DIR) --config $(ROOT_DIR)/.golangci.yml -v $(GOLANGCI_LINT_EXTRA_ARGS)
	./scripts/lint-dockerfiles.sh $(HADOLINT_VER) $(HADOLINT_FAILURE_THRESHOLD)

.PHONY: lint-dockerfiles
lint-dockerfiles:
	./scripts/lint-dockerfiles.sh $(HADOLINT_VER) $(HADOLINT_FAILURE_THRESHOLD)

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Lint the codebase and run auto-fixers if supported by the linter
	GOLANGCI_LINT_EXTRA_ARGS=--fix $(MAKE) lint

.PHONY: tiltfile-fix
tiltfile-fix: ## Format the Tiltfile
	TRACE=$(TRACE) ./hack/verify-starlark.sh fix

APIDIFF_OLD_COMMIT ?= $(shell git rev-parse origin/main)

.PHONY: apidiff
apidiff: $(GO_APIDIFF) ## Check for API differences
	$(GO_APIDIFF) $(APIDIFF_OLD_COMMIT) --print-compatible

ALL_VERIFY_CHECKS = licenses boilerplate shellcheck tiltfile modules gen conversions doctoc capi-book-summary diagrams import-restrictions go-directive

.PHONY: verify
verify: $(addprefix verify-,$(ALL_VERIFY_CHECKS)) lint-dockerfiles ## Run all verify-* targets

.PHONY: verify-go-directive
verify-go-directive:
	TRACE=$(TRACE) ./hack/verify-go-directive.sh -g $(GO_DIRECTIVE_VERSION)

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
verify-gen: generate  ## Verify go generated files are up to date
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

.PHONY: verify-conversions
verify-conversions: $(CONVERSION_VERIFIER)  ## Verifies expected API conversion are in place
	$(CONVERSION_VERIFIER)

.PHONY: verify-doctoc
verify-doctoc: generate-doctoc
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "doctoc is out of date, run make generate-doctoc"; exit 1; \
	fi

.PHONY: verify-capi-book-summary
verify-capi-book-summary:
	TRACE=$(TRACE) ./hack/verify-capi-book-summary.sh

.PHONY: verify-boilerplate
verify-boilerplate: ## Verify boilerplate text exists in each file
	TRACE=$(TRACE) ./hack/verify-boilerplate.sh

.PHONY: verify-shellcheck
verify-shellcheck: ## Verify shell files
	TRACE=$(TRACE) ./hack/verify-shellcheck.sh $(SHELLCHECK_VER)

.PHONY: verify-tiltfile
verify-tiltfile: ## Verify Tiltfile format
	TRACE=$(TRACE) ./hack/verify-starlark.sh

.PHONY: verify-container-images
verify-container-images: ## Verify container images
	TRACE=$(TRACE) ./hack/verify-container-images.sh $(TRIVY_VER)

.PHONY: verify-licenses
verify-licenses: ## Verify licenses
	TRACE=$(TRACE) ./hack/verify-licenses.sh $(TRIVY_VER)

.PHONY: verify-govulncheck
verify-govulncheck: $(GOVULNCHECK) ## Verify code for vulnerabilities
	$(GOVULNCHECK) ./... && R1=$$? || R1=$$?; \
	$(GOVULNCHECK) -C "$(TOOLS_DIR)" ./... && R2=$$? || R2=$$?; \
	$(GOVULNCHECK) -C "$(TEST_DIR)" ./... && R3=$$? || R3=$$?; \
	if [ "$$R1" -ne "0" ] || [ "$$R2" -ne "0" ] || [ "$$R3" -ne "0" ]; then \
		exit 1; \
	fi

.PHONY: verify-diagrams
verify-diagrams: generate-diagrams ## Verify generated diagrams are up to date
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated diagrams are out of date, run make generate-diagrams"; exit 1; \
	fi

.PHONY: verify-security
verify-security: ## Verify code and images for vulnerabilities
	$(MAKE) verify-container-images && R1=$$? || R1=$$?; \
	$(MAKE) verify-govulncheck && R2=$$? || R2=$$?; \
	if [ "$$R1" -ne "0" ] || [ "$$R2" -ne "0" ]; then \
	  echo "Check for vulnerabilities failed! There are vulnerabilities to be fixed"; \
		exit 1; \
	fi

.PHONY: verify-import-restrictions
verify-import-restrictions: $(IMPORT_BOSS) ## Verify import restrictions with import-boss
	./hack/verify-import-restrictions.sh

## --------------------------------------
## Binaries
## --------------------------------------

##@ build:

.PHONY: clusterctl
clusterctl: ## Build the clusterctl binary
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/clusterctl sigs.k8s.io/cluster-api/cmd/clusterctl

ALL_MANAGERS = core kubeadm-bootstrap kubeadm-control-plane docker-infrastructure in-memory-infrastructure

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

.PHONY: manager-docker-infrastructure
manager-docker-infrastructure: ## Build the docker infrastructure manager binary into the ./bin folder
	cd $(CAPD_DIR); go build -trimpath -ldflags "$(LDFLAGS)" -o ../../../$(BIN_DIR)/capd-manager sigs.k8s.io/cluster-api/test/infrastructure/docker

.PHONY: manager-in-memory-infrastructure
manager-in-memory-infrastructure: ## Build the in-memory-infrastructure infrastructure manager binary into the ./bin folder
	cd $(CAPIM_DIR); go build -trimpath -ldflags "$(LDFLAGS)" -o ../../../$(BIN_DIR)/capim-manager sigs.k8s.io/cluster-api/test/infrastructure/inmemory

.PHONY: docker-pull-prerequisites
docker-pull-prerequisites:
	docker pull docker.io/docker/dockerfile:1.4
	docker pull $(GO_CONTAINER_IMAGE)
	docker pull gcr.io/distroless/static:latest

.PHONY: docker-build-all
docker-build-all: $(addprefix docker-build-,$(ALL_ARCH)) ## Build docker images for all architectures

docker-build-%:
	$(MAKE) ARCH=$* docker-build

# Choice of images to build/push
ALL_DOCKER_BUILD ?= core kubeadm-bootstrap kubeadm-control-plane docker-infrastructure in-memory-infrastructure test-extension clusterctl

.PHONY: docker-build
docker-build: docker-pull-prerequisites ## Run docker-build-* targets for all the images
	$(MAKE) ARCH=$(ARCH) $(addprefix docker-build-,$(ALL_DOCKER_BUILD))

ALL_DOCKER_BUILD_E2E = core kubeadm-bootstrap kubeadm-control-plane docker-infrastructure in-memory-infrastructure test-extension

.PHONY: docker-build-e2e
docker-build-e2e: ## Run docker-build-* targets for all the images with settings to be used for the e2e tests
    # please ensure the generated image name matches image names used in the E2E_CONF_FILE;
    # also the same settings must exist in ci-e2e-lib.sh, capi:buildDockerImage func.
	$(MAKE) REGISTRY=gcr.io/k8s-staging-cluster-api PULL_POLICY=IfNotPresent TAG=dev $(addprefix docker-build-,$(ALL_DOCKER_BUILD_E2E))

.PHONY: docker-build-core
docker-build-core: ## Build the docker image for core controller manager
## reads Dockerfile from stdin to avoid an incorrectly cached Dockerfile (https://github.com/moby/buildkit/issues/1368)
	cat ./Dockerfile | DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg ldflags="$(LDFLAGS)" . -t $(CONTROLLER_IMG)-$(ARCH):$(TAG) --file -
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./config/default/manager_pull_policy.yaml"

.PHONY: docker-build-kubeadm-bootstrap
docker-build-kubeadm-bootstrap: ## Build the docker image for kubeadm bootstrap controller manager
## reads Dockerfile from stdin to avoid an incorrectly cached Dockerfile (https://github.com/moby/buildkit/issues/1368)
	cat ./Dockerfile | DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./bootstrap/kubeadm --build-arg ldflags="$(LDFLAGS)" . -t $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH):$(TAG) --file -
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: docker-build-kubeadm-control-plane
docker-build-kubeadm-control-plane: ## Build the docker image for kubeadm control plane controller manager
## reads Dockerfile from stdin to avoid an incorrectly cached Dockerfile (https://github.com/moby/buildkit/issues/1368)
	cat ./Dockerfile | DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./controlplane/kubeadm --build-arg ldflags="$(LDFLAGS)" . -t $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH):$(TAG) --file -
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: docker-build-docker-infrastructure
docker-build-docker-infrastructure: ## Build the docker image for docker infrastructure controller manager
## reads Dockerfile from stdin to avoid an incorrectly cached Dockerfile (https://github.com/moby/buildkit/issues/1368)
	cat $(CAPD_DIR)/Dockerfile | DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg ldflags="$(LDFLAGS)" . -t $(CAPD_CONTROLLER_IMG)-$(ARCH):$(TAG) --file -
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CAPD_CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="$(CAPD_DIR)/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="$(CAPD_DIR)/config/default/manager_pull_policy.yaml"

.PHONY: docker-build-in-memory-infrastructure
docker-build-in-memory-infrastructure: ## Build the docker image for in-memory infrastructure controller manager
## reads Dockerfile from stdin to avoid an incorrectly cached Dockerfile (https://github.com/moby/buildkit/issues/1368)
	cat $(CAPIM_DIR)/Dockerfile | DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg ldflags="$(LDFLAGS)" . -t $(CAPIM_CONTROLLER_IMG)-$(ARCH):$(TAG) --file -
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CAPIM_CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="$(CAPIM_DIR)/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="$(CAPIM_DIR)/config/default/manager_pull_policy.yaml"

.PHONY: docker-build-clusterctl
docker-build-clusterctl: ## Build the docker image for clusterctl
## reads Dockerfile from stdin to avoid an incorrectly cached Dockerfile (https://github.com/moby/buildkit/issues/1368)
	cat ./cmd/clusterctl/Dockerfile | DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./cmd/clusterctl --build-arg ldflags="$(LDFLAGS)" . -t $(CLUSTERCTL_IMG)-$(ARCH):$(TAG) --file -

.PHONY: docker-build-test-extension
docker-build-test-extension: ## Build the docker image for core controller manager
## reads Dockerfile from stdin to avoid an incorrectly cached Dockerfile (https://github.com/moby/buildkit/issues/1368)
	cat ./test/extension/Dockerfile | DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg ldflags="$(LDFLAGS)" . -t $(TEST_EXTENSION_IMG)-$(ARCH):$(TAG) --file -
	$(MAKE) set-manifest-image MANIFEST_IMG=$(TEST_EXTENSION_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./test/extension/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./test/extension/config/default/manager_pull_policy.yaml"

.PHONY: e2e-framework
e2e-framework: ## Builds the CAPI e2e framework
	cd $(E2E_FRAMEWORK_DIR); go build ./...

.PHONY: build-book
build-book: ## Build the book
	$(MAKE) -C docs/book build

## --------------------------------------
## Testing
## --------------------------------------

##@ test:

ARTIFACTS ?= ${ROOT_DIR}/_artifacts

KUBEBUILDER_ASSETS ?= $(shell $(SETUP_ENVTEST) use --use-env -p path $(KUBEBUILDER_ENVTEST_KUBERNETES_VERSION))

.PHONY: setup-envtest
setup-envtest: $(SETUP_ENVTEST) ## Set up envtest (download kubebuilder assets)
	@echo KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS)

.PHONY: test-no-race
test-no-race: $(SETUP_ENVTEST) ## Run unit and integration tests
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./... $(TEST_ARGS)

.PHONY: test
test: $(SETUP_ENVTEST) ## Run unit and integration tests with race detector
	# Note: Fuzz tests are not executed with race detector because they would just time out.
	# To achieve that, all files with fuzz tests have the "!race" build tag, to still run fuzz tests
	# we have an additional `go test` run that focuses on "TestFuzzyConversion".
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -race ./... $(TEST_ARGS)
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -run "^TestFuzzyConversion$$" ./... $(TEST_ARGS)

.PHONY: test-verbose
test-verbose: ## Run unit and integration tests with race detector and with verbose flag
	$(MAKE) test TEST_ARGS="$(TEST_ARGS) -v"

.PHONY: test-junit
test-junit: $(SETUP_ENVTEST) $(GOTESTSUM) ## Run unit and integration tests with race detector and generate a junit report
	set +o errexit; (KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -race -json ./... $(TEST_ARGS); echo $$? > $(ARTIFACTS)/junit.exitcode) | tee $(ARTIFACTS)/junit.stdout
	$(GOTESTSUM) --junitfile $(ARTIFACTS)/junit.xml --raw-command cat $(ARTIFACTS)/junit.stdout
	exit $$(cat $(ARTIFACTS)/junit.exitcode)
	set +o errexit; (KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -run "^TestFuzzyConversion$$" -json ./... $(TEST_ARGS); echo $$? > $(ARTIFACTS)/junit-fuzz.exitcode) | tee $(ARTIFACTS)/junit-fuzz.stdout
	$(GOTESTSUM) --junitfile $(ARTIFACTS)/junit-fuzz.xml --raw-command cat $(ARTIFACTS)/junit-fuzz.stdout
	exit $$(cat $(ARTIFACTS)/junit-fuzz.exitcode)

.PHONY: test-cover
test-cover: ## Run unit and integration tests and generate a coverage report
	$(MAKE) test TEST_ARGS="$(TEST_ARGS) -coverprofile=out/coverage.out"
	go tool cover -func=out/coverage.out -o out/coverage.txt
	go tool cover -html=out/coverage.out -o out/coverage.html

.PHONY: test-docker-infrastructure
test-docker-infrastructure: $(SETUP_ENVTEST) ## Run unit and integration tests for docker infrastructure provider
	cd $(CAPD_DIR); KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -race ./... $(TEST_ARGS)

.PHONY: test-docker-infrastructure-verbose
test-docker-infrastructure-verbose: ## Run unit and integration tests for docker infrastructure provider with verbose flag
	$(MAKE) test-docker-infrastructure TEST_ARGS="$(TEST_ARGS) -v"

.PHONY: test-docker-infrastructure-junit
test-docker-infrastructure-junit: $(SETUP_ENVTEST) $(GOTESTSUM) ## Run unit and integration tests and generate a junit report for docker infrastructure provider
	cd $(CAPD_DIR); set +o errexit; (KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -race -json ./... $(TEST_ARGS); echo $$? > $(ARTIFACTS)/junit.infra_docker.exitcode) | tee $(ARTIFACTS)/junit.infra_docker.stdout
	$(GOTESTSUM) --junitfile $(ARTIFACTS)/junit.infra_docker.xml --raw-command cat $(ARTIFACTS)/junit.infra_docker.stdout
	exit $$(cat $(ARTIFACTS)/junit.infra_docker.exitcode)

.PHONY: test-in-memory-infrastructure
test-in-memory-infrastructure: $(SETUP_ENVTEST) ## Run unit and integration tests for in-memory infrastructure provider
	cd $(CAPIM_DIR); KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -race ./... $(TEST_ARGS)

.PHONY: test-in-memory-infrastructure-verbose
test-in-memory-infrastructure-verbose: ## Run unit and integration tests for in-memory infrastructure provider with verbose flag
	$(MAKE) test-in-memory-infrastructure TEST_ARGS="$(TEST_ARGS) -v"

.PHONY: test-in-memory-infrastructure-junit
test-in-memory-infrastructure-junit: $(SETUP_ENVTEST) $(GOTESTSUM) ## Run unit and integration tests and generate a junit report for in-memory infrastructure provider
	cd $(CAPIM_DIR); set +o errexit; (KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -race -json ./... $(TEST_ARGS); echo $$? > $(ARTIFACTS)/junit.infra_inmemory.exitcode) | tee $(ARTIFACTS)/junit.infra_inmemory.stdout
	$(GOTESTSUM) --junitfile $(ARTIFACTS)/junit.infra_inmemory.xml --raw-command cat $(ARTIFACTS)/junit.infra_inmemory.stdout
	exit $$(cat $(ARTIFACTS)/junit.infra_inmemory.exitcode)

.PHONY: test-test-extension
test-test-extension: $(SETUP_ENVTEST) ## Run unit and integration tests for the test extension
	cd $(TEST_EXTENSION_DIR); KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -race ./... $(TEST_ARGS)

.PHONY: test-test-extension-verbose
test-test-extension-verbose: ## Run unit and integration tests with verbose flag
	$(MAKE) test-test-extension TEST_ARGS="$(TEST_ARGS) -v"

.PHONY: test-test-extension-junit
test-test-extension-junit: $(SETUP_ENVTEST) $(GOTESTSUM) ## Run unit and integration tests and generate a junit report for the test extension
	cd $(TEST_EXTENSION_DIR); set +o errexit; (KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -race -json ./... $(TEST_ARGS); echo $$? > $(ARTIFACTS)/junit.test_extension.exitcode) | tee $(ARTIFACTS)/junit.test_extension.stdout
	$(GOTESTSUM) --junitfile $(ARTIFACTS)/junit.test_extension.xml --raw-command cat $(ARTIFACTS)/junit.test_extension.stdout
	exit $$(cat $(ARTIFACTS)/junit.test_extension.exitcode)

.PHONY: test-e2e
test-e2e: $(GINKGO) generate-e2e-templates ## Run the end-to-end tests
	$(GINKGO) -v --trace -poll-progress-after=$(GINKGO_POLL_PROGRESS_AFTER) \
		-poll-progress-interval=$(GINKGO_POLL_PROGRESS_INTERVAL) --tags=e2e --focus="$(GINKGO_FOCUS)" \
		$(_SKIP_ARGS) --nodes=$(GINKGO_NODES) --timeout=$(GINKGO_TIMEOUT) --no-color=$(GINKGO_NOCOLOR) \
		--output-dir="$(ARTIFACTS)" --junit-report="junit.e2e_suite.1.xml" $(GINKGO_ARGS) $(ROOT_DIR)/$(TEST_DIR)/e2e -- \
	    -e2e.artifacts-folder="$(ARTIFACTS)" \
	    -e2e.config="$(E2E_CONF_FILE)" \
	    -e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) -e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)


.PHONY: kind-cluster
kind-cluster: ## Create a new kind cluster designed for development with Tilt
	hack/kind-install-for-capd.sh

.PHONY: tilt-e2e-prerequisites
tilt-e2e-prerequisites: ## Build the corresponding kindest/node images required for e2e testing and generate the e2e templates
	scripts/build-kind.sh
	$(MAKE) generate-e2e-templates

.PHONY: tilt-up
tilt-up: kind-cluster ## Start tilt and build kind cluster if needed.
	tilt up

.PHONY: serve-book
serve-book: ## Build and serve the book (with live-reload)
	$(MAKE) -C docs/book serve

## --------------------------------------
## Release
## --------------------------------------

##@ release:

## latest git tag for the commit, e.g., v0.3.10
RELEASE_TAG ?= $(shell git describe --abbrev=0 2>/dev/null)
## set by Prow, ref name of the base branch, e.g., main
RELEASE_ALIAS_TAG := $(PULL_BASE_REF)
RELEASE_DIR := out
RELEASE_NOTES_DIR := CHANGELOG
USER_FORK ?= $(shell git config --get remote.origin.url | cut -d/ -f4) # only works on https://github.com/<username>/cluster-api.git style URLs
ifeq ($(USER_FORK),)
USER_FORK := $(shell git config --get remote.origin.url | cut -d: -f2 | cut -d/ -f1) # for git@github.com:<username>/cluster-api.git style URLs
endif
IMAGE_REVIEWERS ?= $(shell ./hack/get-project-maintainers.sh)

.PHONY: $(RELEASE_DIR)
$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)/

.PHONY: $(RELEASE_NOTES_DIR)
$(RELEASE_NOTES_DIR):
	mkdir -p $(RELEASE_NOTES_DIR)/

.PHONY: release
release: clean-release ## Build and push container images using the latest git tag for the commit
	@if [ -z "${RELEASE_TAG}" ]; then echo "RELEASE_TAG is not set"; exit 1; fi
	@if ! [ -z "$$(git status --porcelain)" ]; then echo "Your local git repository contains uncommitted changes, use git clean before proceeding."; exit 1; fi
	git checkout "${RELEASE_TAG}"
	# Build binaries first.
	GIT_VERSION=$(RELEASE_TAG) $(MAKE) release-binaries
	# Set the manifest images to the staging/production bucket and Builds the manifests to publish with a release.
	$(MAKE) release-manifests-all

.PHONY: release-manifests-all
release-manifests-all: # Set the manifest images to the staging/production bucket and Builds the manifests to publish with a release.
	# Set the manifest image to the production bucket.
	$(MAKE) manifest-modification REGISTRY=$(PROD_REGISTRY)
	## Build the manifests
	$(MAKE) release-manifests
	# Set the development manifest image to the staging bucket.
	$(MAKE) manifest-modification-dev REGISTRY=$(STAGING_REGISTRY)
	## Build the development manifests
	$(MAKE) release-manifests-dev
	## Clean the git artifacts modified in the release process
	$(MAKE) clean-release-git

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

.PHONY: manifest-modification-dev
manifest-modification-dev: # Set the manifest images to the staging bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(REGISTRY)/$(CAPD_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="$(CAPD_DIR)/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="$(CAPD_DIR)/config/default/manager_pull_policy.yaml"
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(REGISTRY)/$(CAPIM_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="$(CAPIM_DIR)/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="$(CAPIM_DIR)/config/default/manager_pull_policy.yaml"
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(REGISTRY)/$(TEST_EXTENSION_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="$(TEST_EXTENSION_DIR)/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="$(CAPD_DIR)/config/default/manager_pull_policy.yaml"


.PHONY: release-manifests
release-manifests: $(RELEASE_DIR) $(KUSTOMIZE) $(RUNTIME_OPENAPI_GEN) ## Build the manifests to publish with a release
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

	# Generate OpenAPI specification.
	$(RUNTIME_OPENAPI_GEN) --version $(RELEASE_TAG) --output-file $(RELEASE_DIR)/runtime-sdk-openapi.yaml

.PHONY: release-manifests-dev
release-manifests-dev: $(RELEASE_DIR) $(KUSTOMIZE) ## Build the development manifests and copies them in the release folder
	cd $(CAPD_DIR); $(KUSTOMIZE) build config/default > ../../../$(RELEASE_DIR)/infrastructure-components-development.yaml
	cp $(CAPD_DIR)/templates/* $(RELEASE_DIR)/
	cd $(CAPIM_DIR); $(KUSTOMIZE) build config/default > ../../../$(RELEASE_DIR)/infrastructure-components-in-memory-development.yaml
	cp $(CAPIM_DIR)/templates/* $(RELEASE_DIR)/
	cd $(TEST_EXTENSION_DIR); $(KUSTOMIZE) build config/default > ../../$(RELEASE_DIR)/runtime-extension-components-development.yaml

.PHONY: release-binaries
release-binaries: ## Build the binaries to publish with a release
	RELEASE_BINARY=clusterctl-linux-amd64 BUILD_PATH=./cmd/clusterctl GOOS=linux GOARCH=amd64 $(MAKE) release-binary
	RELEASE_BINARY=clusterctl-linux-arm64 BUILD_PATH=./cmd/clusterctl GOOS=linux GOARCH=arm64 $(MAKE) release-binary
	RELEASE_BINARY=clusterctl-darwin-amd64 BUILD_PATH=./cmd/clusterctl GOOS=darwin GOARCH=amd64 $(MAKE) release-binary
	RELEASE_BINARY=clusterctl-darwin-arm64 BUILD_PATH=./cmd/clusterctl GOOS=darwin GOARCH=arm64 $(MAKE) release-binary
	RELEASE_BINARY=clusterctl-windows-amd64.exe BUILD_PATH=./cmd/clusterctl GOOS=windows GOARCH=amd64 $(MAKE) release-binary
	RELEASE_BINARY=clusterctl-linux-ppc64le BUILD_PATH=./cmd/clusterctl GOOS=linux GOARCH=ppc64le $(MAKE) release-binary

.PHONY: release-binary
release-binary: $(RELEASE_DIR)
	docker run \
		--rm \
		-e CGO_ENABLED=0 \
		-e GOOS=$(GOOS) \
		-e GOARCH=$(GOARCH) \
		-e GOCACHE=/tmp/ \
		--user $$(id -u):$$(id -g) \
		-v "$$(pwd):/workspace$(DOCKER_VOL_OPTS)" \
		-w /workspace \
		golang:$(GO_VERSION) \
		go build -a -trimpath -ldflags "$(LDFLAGS) -extldflags '-static'" \
		-o $(RELEASE_DIR)/$(notdir $(RELEASE_BINARY)) $(BUILD_PATH)

.PHONY: release-staging
release-staging: ## Build and push container images to the staging bucket
	REGISTRY=$(STAGING_REGISTRY) $(MAKE) docker-build-all
	REGISTRY=$(STAGING_REGISTRY) $(MAKE) docker-image-verify
	REGISTRY=$(STAGING_REGISTRY) $(MAKE) docker-push-all
	REGISTRY=$(STAGING_REGISTRY) $(MAKE) release-alias-tag
	# Set the manifest image to the staging bucket.
	$(MAKE) manifest-modification REGISTRY=$(STAGING_REGISTRY) RELEASE_TAG=$(RELEASE_ALIAS_TAG)
	## Build the manifests
	$(MAKE) release-manifests
	# Set the manifest image to the staging bucket.
	$(MAKE) manifest-modification-dev REGISTRY=$(STAGING_REGISTRY) RELEASE_TAG=$(RELEASE_ALIAS_TAG)
	## Build the dev manifests
	$(MAKE) release-manifests-dev
	# Example manifest location: https://storage.googleapis.com/k8s-staging-cluster-api/components/main/core-components.yaml
	# Please note that these files are deleted after a certain period, at the time of this writing 60 days after file creation.
	gsutil cp $(RELEASE_DIR)/* gs://$(STAGING_BUCKET)/components/$(RELEASE_ALIAS_TAG)

.PHONY: release-staging-nightly
release-staging-nightly: ## Tag and push container images to the staging bucket. Example image tag: cluster-api-controller:nightly_main_20210121
	$(eval NEW_RELEASE_ALIAS_TAG := nightly_$(RELEASE_ALIAS_TAG)_$(shell date +'%Y%m%d'))
	echo $(NEW_RELEASE_ALIAS_TAG)
	$(MAKE) release-alias-tag TAG=$(RELEASE_ALIAS_TAG) RELEASE_ALIAS_TAG=$(NEW_RELEASE_ALIAS_TAG)
	# Set the manifest image to the staging bucket.
	$(MAKE) manifest-modification REGISTRY=$(STAGING_REGISTRY) RELEASE_TAG=$(NEW_RELEASE_ALIAS_TAG)
	## Build the manifests
	$(MAKE) release-manifests
	# Set the manifest image to the staging bucket.
	$(MAKE) manifest-modification-dev REGISTRY=$(STAGING_REGISTRY) RELEASE_TAG=$(NEW_RELEASE_ALIAS_TAG)
	## Build the dev manifests
	$(MAKE) release-manifests-dev
	# Example manifest location: https://storage.googleapis.com/k8s-staging-cluster-api/components/nightly_main_20240425/core-components.yaml
	# Please note that these files are deleted after a certain period, at the time of this writing 60 days after file creation.
	gsutil cp $(RELEASE_DIR)/* gs://$(STAGING_BUCKET)/components/$(NEW_RELEASE_ALIAS_TAG)

.PHONY: release-alias-tag
release-alias-tag: ## Add the release alias tag to the last build tag
	gcloud container images add-tag $(CONTROLLER_IMG):$(TAG) $(CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG) $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(TAG) $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(CLUSTERCTL_IMG):$(TAG) $(CLUSTERCTL_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(CAPD_CONTROLLER_IMG):$(TAG) $(CAPD_CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(CAPIM_CONTROLLER_IMG):$(TAG) $(CAPIM_CONTROLLER_IMG):$(RELEASE_ALIAS_TAG)
	gcloud container images add-tag $(TEST_EXTENSION_IMG):$(TAG) $(TEST_EXTENSION_IMG):$(RELEASE_ALIAS_TAG)

.PHONY: release-notes-tool
release-notes-tool:
	go build -C hack/tools -o $(ROOT_DIR)/bin/notes -tags tools sigs.k8s.io/cluster-api/hack/tools/release/notes

.PHONY: release-notes
release-notes: release-notes-tool
	./bin/notes --release $(RELEASE_TAG) --previous-release-version "$(PREVIOUS_RELEASE_TAG)" > CHANGELOG/$(RELEASE_TAG).md

.PHONY: test-release-notes-tool
test-release-notes-tool:
	go test -race -C hack/tools -v -tags tools,integration sigs.k8s.io/cluster-api/hack/tools/release/notes

.PHONY: release-provider-issues-tool
release-provider-issues-tool: # Creates GitHub issues in a pre-defined list of CAPI provider repositories
	@go run ./hack/tools/release/internal/update_providers/provider_issues.go

.PHONY: release-weekly-update-tool
release-weekly-update-tool:
	go build -C hack/tools -o $(ROOT_DIR)/bin/weekly -tags tools sigs.k8s.io/cluster-api/hack/tools/release/weekly

.PHONY: promote-images
promote-images: $(KPROMO)
	$(KPROMO) pr --project cluster-api --tag $(RELEASE_TAG) --reviewers "$(IMAGE_REVIEWERS)" --fork $(USER_FORK) --image cluster-api-controller --image kubeadm-control-plane-controller --image kubeadm-bootstrap-controller --image clusterctl

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-image-verify
docker-image-verify: ## Verifies all built images to contain the correct binary in the expected arch
	ALL_ARCH="$(ALL_ARCH)" TAG="$(TAG)" ./hack/docker-image-verify.sh

.PHONY: docker-push-all
docker-push-all: $(addprefix docker-push-,$(ALL_ARCH))  ## Push the docker images to be included in the release for all architectures + related multiarch manifests
	$(MAKE) ALL_ARCH="$(ALL_ARCH)" $(addprefix docker-push-manifest-,$(ALL_DOCKER_BUILD))

docker-push-%:
	$(MAKE) ARCH=$* docker-push

.PHONY: docker-push
docker-push: $(addprefix docker-push-,$(ALL_DOCKER_BUILD)) ## Push the docker images to be included in the release

.PHONY: docker-push-core
docker-push-core: ## Push the core docker image
	docker push $(CONTROLLER_IMG)-$(ARCH):$(TAG)

.PHONY: docker-push-manifest-core
docker-push-manifest-core: ## Push the multiarch manifest for the core docker images
	docker manifest create --amend $(CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CONTROLLER_IMG}:${TAG} ${CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./config/default/manager_pull_policy.yaml"

.PHONY: docker-push-kubeadm-bootstrap
docker-push-kubeadm-bootstrap: ## Push the kubeadm bootstrap docker image
	docker push $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)-$(ARCH):$(TAG)

.PHONY: docker-push-manifest-kubeadm-bootstrap
docker-push-manifest-kubeadm-bootstrap: ## Push the multiarch manifest for the kubeadm bootstrap docker images
	docker manifest create --amend $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${KUBEADM_BOOTSTRAP_CONTROLLER_IMG}:${TAG} ${KUBEADM_BOOTSTRAP_CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(KUBEADM_BOOTSTRAP_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_BOOTSTRAP_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: docker-push-kubeadm-control-plane
docker-push-kubeadm-control-plane: ## Push the kubeadm control plane docker image
	docker push $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)-$(ARCH):$(TAG)

.PHONY: docker-push-manifest-kubeadm-control-plane
docker-push-manifest-kubeadm-control-plane: ## Push the multiarch manifest for the kubeadm control plane docker images
	docker manifest create --amend $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${KUBEADM_CONTROL_PLANE_CONTROLLER_IMG}:${TAG} ${KUBEADM_CONTROL_PLANE_CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(KUBEADM_CONTROL_PLANE_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./controlplane/kubeadm/config/default/manager_pull_policy.yaml"

.PHONY: docker-push-docker-infrastructure
docker-push-docker-infrastructure: ## Push the docker infrastructure provider image
	docker push $(CAPD_CONTROLLER_IMG)-$(ARCH):$(TAG)

.PHONY: docker-push-manifest-docker-infrastructure
docker-push-manifest-docker-infrastructure: ## Push the multiarch manifest for the docker infrastructure provider images
	docker manifest create --amend $(CAPD_CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CAPD_CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CAPD_CONTROLLER_IMG}:${TAG} ${CAPD_CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(CAPD_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CAPD_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="$(CAPD_DIR)/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="$(CAPD_DIR)/config/default/manager_pull_policy.yaml"

.PHONY: docker-push-in-memory-infrastructure
docker-push-in-memory-infrastructure: ## Push the in-memory infrastructure provider image
	docker push $(CAPIM_CONTROLLER_IMG)-$(ARCH):$(TAG)

.PHONY: docker-push-manifest-in-memory-infrastructure
docker-push-manifest-in-memory-infrastructure: ## Push the multiarch manifest for the in-memory infrastructure provider images
	docker manifest create --amend $(CAPIM_CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CAPIM_CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CAPIM_CONTROLLER_IMG}:${TAG} ${CAPIM_CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(CAPIM_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CAPIM_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="$(CAPIM_DIR)/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="$(CAPIM_DIR)/config/default/manager_pull_policy.yaml"

.PHONY: docker-push-test-extension
docker-push-test-extension: ## Push the test extension provider image
	docker push $(TEST_EXTENSION_IMG)-$(ARCH):$(TAG)

.PHONY: docker-push-manifest-test-extension
docker-push-manifest-test-extension: ## Push the multiarch manifest for the test extension provider images
	docker manifest create --amend $(TEST_EXTENSION_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(TEST_EXTENSION_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${TEST_EXTENSION_IMG}:${TAG} ${TEST_EXTENSION_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(TEST_EXTENSION_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(TEST_EXTENSION_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./test/extension/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./test/extension/config/default/manager_pull_policy.yaml"

.PHONY: docker-push-clusterctl
docker-push-clusterctl: ## Push the clusterctl image
	docker push $(CLUSTERCTL_IMG)-$(ARCH):$(TAG)

.PHONY: docker-push-manifest-clusterctl
docker-push-manifest-clusterctl: ## Push the multiarch manifest for the clusterctl images
	docker manifest create --amend $(CLUSTERCTL_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CLUSTERCTL_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CLUSTERCTL_IMG}:${TAG} ${CLUSTERCTL_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(CLUSTERCTL_IMG):$(TAG)

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
clean: ## Remove generated binaries, GitBook files, Helm charts, and Tilt build files
	$(MAKE) clean-bin
	$(MAKE) clean-book
	$(MAKE) clean-charts
	$(MAKE) clean-tilt

.PHONY: clean-kind
clean-kind: ## Cleans up the kind cluster with the name $CAPI_KIND_CLUSTER_NAME
	kind delete cluster --name="$(CAPI_KIND_CLUSTER_NAME)" || true

.PHONY: clean-bin
clean-bin: ## Remove all generated binaries
	rm -rf $(BIN_DIR)
	rm -rf $(TOOLS_BIN_DIR)

.PHONY: clean-tilt
clean-tilt: clean-charts clean-kind ## Remove all files generated by Tilt
	rm -rf ./.tiltbuild
	rm -rf ./controlplane/kubeadm/.tiltbuild
	rm -rf ./bootstrap/kubeadm/.tiltbuild
	rm -rf ./test/infrastructure/docker/.tiltbuild
	rm -rf ./test/infrastructure/inmemory/.tiltbuild

.PHONY: clean-charts
clean-charts: ## Remove all local copies of Helm charts in ./hack/observability
	(for path in "./hack/observability/*"; do rm -rf $$path/.charts ; done)

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

.PHONY: clean-generated-yaml
clean-generated-yaml: ## Remove files generated by conversion-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	(IFS=','; for i in $(SRC_DIRS); do find $$i -type f -name '*.yaml' -exec rm -f {} \;; done)

.PHONY: clean-generated-deepcopy
clean-generated-deepcopy: ## Remove files generated by conversion-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	(IFS=','; for i in $(SRC_DIRS); do find $$i -type f -name 'zz_generated.deepcopy*' -exec rm -f {} \;; done)

.PHONY: clean-generated-conversions
clean-generated-conversions: ## Remove files generated by conversion-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	(IFS=','; for i in $(SRC_DIRS); do find $$i -type f -name 'zz_generated.conversion*' -exec rm -f {} \;; done)

.PHONY: clean-generated-openapi-definitions
clean-generated-openapi-definitions: ## Remove files generated by openapi-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	(IFS=','; for i in $(SRC_DIRS); do find $$i -type f -name 'zz_generated.openapi*' -exec rm -f {} \;; done)

## --------------------------------------
## Hack / Tools
## --------------------------------------

##@ hack/tools:

.PHONY: $(CONTROLLER_GEN_BIN)
$(CONTROLLER_GEN_BIN): $(CONTROLLER_GEN) ## Build a local copy of controller-gen.

.PHONY: $(CONVERSION_GEN_BIN)
$(CONVERSION_GEN_BIN): $(CONVERSION_GEN) ## Build a local copy of conversion-gen.

.PHONY: $(OPENAPI_GEN_BIN)
$(OPENAPI_GEN_BIN): $(OPENAPI_GEN) ## Build a local copy of openapi-gen.

.PHONY: $(RUNTIME_OPENAPI_GEN_BIN)
$(RUNTIME_OPENAPI_GEN_BIN): $(RUNTIME_OPENAPI_GEN) ## Build a local copy of runtime-openapi-gen.

.PHONY: $(PROWJOB_GEN_BIN)
$(PROWJOB_GEN_BIN): $(PROWJOB_GEN) ## Build a local copy of prowjob-gen.

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

.PHONY: $(YQ_BIN)
$(YQ_BIN): $(YQ) ## Build a local copy of yq

.PHONY: $(TILT_PREPARE_BIN)
$(TILT_PREPARE_BIN): $(TILT_PREPARE) ## Build a local copy of tilt-prepare.

.PHONY: $(GINKGO_BIN)
$(GINKGO_BIN): $(GINKGO) ## Build a local copy of ginkgo.

.PHONY: $(GOLANGCI_LINT_BIN)
$(GOLANGCI_LINT_BIN): $(GOLANGCI_LINT) ## Build a local copy of golangci-lint.

.PHONY: $(GOVULNCHECK_BIN)
$(GOVULNCHECK_BIN): $(GOVULNCHECK) ## Build a local copy of govulncheck.

.PHONY: $(IMPORT_BOSS_BIN)
$(IMPORT_BOSS_BIN): $(IMPORT_BOSS)

$(CONTROLLER_GEN): # Build controller-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(CONTROLLER_GEN_PKG) $(CONTROLLER_GEN_BIN) $(CONTROLLER_GEN_VER)

## We are forcing a rebuilt of conversion-gen via PHONY so that we're always using an up-to-date version.
## We can't use a versioned name for the binary, because that would be reflected in generated files.
.PHONY: $(CONVERSION_GEN)
$(CONVERSION_GEN): # Build conversion-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(CONVERSION_GEN_PKG) $(CONVERSION_GEN_BIN) $(CONVERSION_GEN_VER)

$(CONVERSION_VERIFIER): $(TOOLS_DIR)/go.mod # Build conversion-verifier from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/$(CONVERSION_VERIFIER_BIN) sigs.k8s.io/cluster-api/hack/tools/conversion-verifier

.PHONY: $(OPENAPI_GEN)
$(OPENAPI_GEN): # Build openapi-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(OPENAPI_GEN_PKG) $(OPENAPI_GEN_BIN) $(OPENAPI_GEN_VER)

## We are forcing a rebuilt of runtime-openapi-gen via PHONY so that we're always using an up-to-date version.
.PHONY: $(RUNTIME_OPENAPI_GEN)
$(RUNTIME_OPENAPI_GEN): $(TOOLS_DIR)/go.mod # Build openapi-gen from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/$(RUNTIME_OPENAPI_GEN_BIN) sigs.k8s.io/cluster-api/hack/tools/runtime-openapi-gen

.PHONY: $(PROWJOB_GEN)
$(PROWJOB_GEN): $(TOOLS_DIR)/go.mod # Build prowjob-gen from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/$(PROWJOB_GEN_BIN) sigs.k8s.io/cluster-api/hack/tools/prowjob-gen

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
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/tilt-prepare sigs.k8s.io/cluster-api/hack/tools/internal/tilt-prepare

$(KPROMO):
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(KPROMO_PKG) $(KPROMO_BIN) ${KPROMO_VER}

$(YQ):
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(YQ_PKG) $(YQ_BIN) ${YQ_VER}

$(GINKGO): # Build ginkgo from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(GINKGO_PKG) $(GINKGO_BIN) $(GINKGO_VER)

$(GOLANGCI_LINT): # Build golangci-lint from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(GOLANGCI_LINT_PKG) $(GOLANGCI_LINT_BIN) $(GOLANGCI_LINT_VER)

$(GOVULNCHECK): # Build govulncheck.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(GOVULNCHECK_PKG) $(GOVULNCHECK_BIN) $(GOVULNCHECK_VER)

$(IMPORT_BOSS): # Build import-boss
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(IMPORT_BOSS_PKG) $(IMPORT_BOSS_BIN) $(IMPORT_BOSS_VER)

## --------------------------------------
## triage-party
## --------------------------------------

.PHONY: release-triage-party
release-triage-party: docker-build-triage-party docker-push-triage-party clean-triage-party

.PHONY: release-triage-party-local
release-triage-party-local: docker-build-triage-party clean-triage-party ## Release the triage party image for local use only

.PHONY: checkout-triage-party
checkout-triage-party:
	@if [ -z "${TRIAGE_PARTY_VERSION}" ]; then echo "TRIAGE_PARTY_VERSION is not set"; exit 1; fi
	@if [ -d "$(TRIAGE_PARTY_TMP_DIR)" ]; then \
		echo "$(TRIAGE_PARTY_TMP_DIR) exists, skipping clone"; \
	else \
		git clone "https://github.com/google/triage-party.git" "$(TRIAGE_PARTY_TMP_DIR)"; \
		cd "$(TRIAGE_PARTY_TMP_DIR)"; \
		git checkout "$(TRIAGE_PARTY_VERSION)"; \
		git apply "$(ROOT_DIR)/$(TRIAGE_PARTY_DIR)/triage-improvements.patch"; \
	fi
	@cd "$(ROOT_DIR)/$(TRIAGE_PARTY_TMP_DIR)"; \
	if [ "$$(git describe --tag 2> /dev/null)" != "$(TRIAGE_PARTY_VERSION)" ]; then \
		echo "ERROR: checked out version $$(git describe --tag 2> /dev/null) does not match expected version $(TRIAGE_PARTY_VERSION)"; \
		exit 1; \
	fi

.PHONY: docker-build-triage-party
docker-build-triage-party: checkout-triage-party
	@if [ -z "${TRIAGE_PARTY_VERSION}" ]; then echo "TRIAGE_PARTY_VERSION is not set"; exit 1; fi
	cd $(TRIAGE_PARTY_TMP_DIR) && \
	docker buildx build --platform linux/amd64 -t $(TRIAGE_PARTY_CONTROLLER_IMG):$(TRIAGE_PARTY_VERSION) .

.PHONY: docker-push-triage-party
docker-push-triage-party:
	@if [ -z "${TRIAGE_PARTY_VERSION}" ]; then echo "TRIAGE_PARTY_VERSION is not set"; exit 1; fi
	docker push $(TRIAGE_PARTY_CONTROLLER_IMG):$(TRIAGE_PARTY_VERSION)

.PHONY: clean-triage-party
clean-triage-party:
	rm -fr "$(TRIAGE_PARTY_TMP_DIR)"

.PHONY: triage-party
triage-party: ## Start a local instance of triage party
	@if [ -z "${GITHUB_TOKEN}" ]; then echo "GITHUB_TOKEN is not set"; exit 1; fi
	docker run --platform linux/amd64 --rm \
		-e GITHUB_TOKEN \
		-e "PERSIST_BACKEND=disk" \
		-e "PERSIST_PATH=/app/.cache" \
		-v "$(ROOT_DIR)/$(TRIAGE_PARTY_DIR)/.cache:/app/.cache" \
		-v "$(ROOT_DIR)/$(TRIAGE_PARTY_DIR)/config.yaml:/app/config/config.yaml" \
		-p 8080:8080 \
		$(TRIAGE_PARTY_CONTROLLER_IMG):$(TRIAGE_PARTY_VERSION)

## --------------------------------------
## Helpers
## --------------------------------------

##@ helpers:

go-version: ## Print the go version we use to compile our binaries and images
	@echo $(GO_VERSION)
