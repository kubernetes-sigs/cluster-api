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
	cd $(TOOLS_DIR) && go build -tags=tools -o $(BIN_DIR)/kustomize sigs.k8s.io/kustomize/kustomize/v3

## --------------------------------------
## Templates
## --------------------------------------

DOCKER_TEMPLATES := $(REPO_ROOT)/test/e2e/data/infrastructure-docker

.PHONY: cluster-templates
cluster-templates: $(KUSTOMIZE) ## Generate cluster templates
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/cluster-template --load_restrictor none > $(DOCKER_TEMPLATES)/cluster-template.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/cluster-template-mhc --load_restrictor none > $(DOCKER_TEMPLATES)/cluster-template-mhc.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/cluster-template-kcp-adoption/step1 --load_restrictor none > $(DOCKER_TEMPLATES)/cluster-template-kcp-adoption.yaml
	echo "---" >> $(DOCKER_TEMPLATES)/cluster-template-kcp-adoption.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/cluster-template-kcp-adoption/step2 --load_restrictor none >> $(DOCKER_TEMPLATES)/cluster-template-kcp-adoption.yaml
	$(KUSTOMIZE) build $(DOCKER_TEMPLATES)/cluster-template-machine-pool --load_restrictor none > $(DOCKER_TEMPLATES)/cluster-template-machine-pool.yaml

## --------------------------------------
## Testing
## --------------------------------------

GINKGO_FOCUS  ?=
GINKGO_NODES  ?= 1
E2E_CONF_FILE  ?= ${REPO_ROOT}/test/e2e/config/docker.yaml
ARTIFACTS ?= ${REPO_ROOT}/_artifacts
SKIP_RESOURCE_CLEANUP ?= false
USE_EXISTING_CLUSTER ?= false
GINKGO_NOCOLOR ?= false

.PHONY: run
run: $(GINKGO) cluster-templates ## Run the end-to-end tests
	$(GINKGO) -v -trace -tags=e2e -focus="$(GINKGO_FOCUS)" -nodes=$(GINKGO_NODES) --noColor=$(GINKGO_NOCOLOR) $(GINKGO_ARGS) . -- \
	    -e2e.artifacts-folder="$(ARTIFACTS)" \
	    -e2e.config="$(E2E_CONF_FILE)" \
	    -e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) -e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)
