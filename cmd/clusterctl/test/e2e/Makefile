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
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Generate / Manifests
## --------------------------------------

.PHONY: modules
modules: ## Runs go mod to ensure modules are up to date.
	go mod tidy

## --------------------------------------
## Testing
## --------------------------------------

GINKGO_FOCUS  ?= ""
E2E_CONF_FILE  ?= ${REPO_ROOT}/cmd/clusterctl/test/config/docker-dev.conf
ARTIFACTS_FOLDER ?= ${REPO_ROOT}/_artifacts
SKIP_RESOURCE_CLEANUP ?= false

.PHONY: test-e2e
test-e2e: ## Run the end-to-end tests
	go test . -v -count=1 -timeout=40m -tags=e2e \
	    -ginkgo.v \
	    -ginkgo.trace \
	    -ginkgo.progress \
	    -ginkgo.focus=$(GINKGO_FOCUS) \
	    -e2e.config-base-path="$(REPO_ROOT)" \
	    -e2e.config="$(E2E_CONF_FILE)" \
	    -e2e.artifacts-folder="$(ARTIFACTS_FOLDER)" \
	    -e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP)
