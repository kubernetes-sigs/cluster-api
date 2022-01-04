# Copyright 2020 The Kubernetes Authors.
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

# If you update this file, please follow:
# https://suva.sh/posts/well-documented-makefiles/

# Use GOPROXY environment variable if set

.DEFAULT_GOAL:=help

GOPROXY := $(shell go env GOPROXY)
ifeq ($(GOPROXY),)
GOPROXY := https://proxy.golang.org
endif
export GOPROXY

REPO_ROOT := $(shell git rev-parse --show-toplevel)

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Binaries
## --------------------------------------

TOOLS_DIR := $(REPO_ROOT)/hack/tools
BIN_DIR := bin
TOOLS_BIN_DIR := $(TOOLS_DIR)/$(BIN_DIR)
GINKGO := $(TOOLS_BIN_DIR)/ginkgo
KUSTOMIZE := $(TOOLS_BIN_DIR)/kustomize

$(GINKGO): # Build ginkgo from tools folder.
	cd $(TOOLS_DIR) && go build -tags=tools -o $(BIN_DIR)/ginkgo github.com/onsi/ginkgo/ginkgo

$(KUSTOMIZE): # Build kustomize from tools folder.
	$(REPO_ROOT)/hack/ensure-kustomize.sh

## --------------------------------------
## Templates
## --------------------------------------

DOCKER_TEMPLATES := $(REPO_ROOT)/test/e2e/data/infrastructure-docker

.PHONY: cluster-templates
cluster-templates: $(KUSTOMIZE) cluster-templates-v1alpha3 cluster-templates-v1alpha4 cluster-templates-v1beta1 ## Generate cluster templates for all versions

cluster-templates-v1alpha3: $(KUSTOMIZE) ## Generate cluster templates for v1alpha3
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha3/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha3/cluster-template.yaml

cluster-templates-v1alpha4: $(KUSTOMIZE) ## Generate cluster templates for v1alpha4
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha4/cluster-template.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-md-remediation --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-md-remediation.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-kcp-remediation --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-kcp-remediation.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-kcp-adoption/step1 --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-kcp-adoption.yaml
	echo "---" >> $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-kcp-adoption.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-kcp-adoption/step2 --load-restrictor LoadRestrictionsNone >> $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-kcp-adoption.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-machine-pool --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-machine-pool.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-node-drain --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-node-drain.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-upgrades --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-upgrades.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-kcp-scale-in --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-kcp-scale-in.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-ipv6 --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1alpha4/cluster-template-ipv6.yaml

cluster-templates-v1beta1: $(KUSTOMIZE) ## Generate cluster templates for v1beta1
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-md-remediation --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-md-remediation.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-kcp-remediation --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-kcp-remediation.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-kcp-adoption/step1 --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-kcp-adoption.yaml
	echo "---" >> $(DOCKER_TEMPLATES)/v1beta1/cluster-template-kcp-adoption.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-kcp-adoption/step2 --load-restrictor LoadRestrictionsNone >> $(DOCKER_TEMPLATES)/v1beta1/cluster-template-kcp-adoption.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-machine-pool --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-machine-pool.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-node-drain --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-node-drain.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-upgrades --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-upgrades.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-kcp-scale-in --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-kcp-scale-in.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-ipv6 --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-ipv6.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-topology --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-topology.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/v1beta1/cluster-template-ignition --load-restrictor LoadRestrictionsNone > $(DOCKER_TEMPLATES)/v1beta1/cluster-template-ignition.yaml

## --------------------------------------
## Testing
## --------------------------------------

GINKGO_FOCUS  ?=
GINKGO_SKIP ?=
GINKGO_NODES  ?= 1
E2E_CONF_FILE  ?= ${REPO_ROOT}/test/e2e/config/docker.yaml
ARTIFACTS ?= ${REPO_ROOT}/_artifacts
SKIP_RESOURCE_CLEANUP ?= false
USE_EXISTING_CLUSTER ?= false
GINKGO_NOCOLOR ?= false

# to set multiple ginkgo skip flags, if any
ifneq ($(strip $(GINKGO_SKIP)),)
_SKIP_ARGS := $(foreach arg,$(strip $(GINKGO_SKIP)),-skip="$(arg)")
endif

.PHONY: run
run: $(GINKGO) cluster-templates ## Run the end-to-end tests
	$(GINKGO) -v -trace -tags=e2e -focus="$(GINKGO_FOCUS)" $(_SKIP_ARGS) -nodes=$(GINKGO_NODES) --noColor=$(GINKGO_NOCOLOR) $(GINKGO_ARGS) . -- \
	    -e2e.artifacts-folder="$(ARTIFACTS)" \
	    -e2e.config="$(E2E_CONF_FILE)" \
	    -e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) -e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)
