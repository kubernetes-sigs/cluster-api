# Copyright 2019 The Kubernetes Authors.
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

.DEFAULT_GOAL:=help

# Use GOPROXY environment variable if set
GOPROXY := $(shell go env GOPROXY)
ifeq ($(GOPROXY),)
GOPROXY := https://proxy.golang.org
endif
export GOPROXY

REGISTRY ?= gcr.io/$(shell gcloud config get-value project)
# A release does not need to define this
MANAGER_IMAGE_NAME ?= cluster-api-bootstrap-provider-kubeadm
MANAGER_IMAGE_TAG ?= dev
PULL_POLICY ?= Always

# Define Docker related variables. Releases should modify and double check these vars.
REGISTRY ?= gcr.io/$(shell gcloud config get-value project)
STAGING_REGISTRY := gcr.io/k8s-staging-capi-kubeadm
PROD_REGISTRY := us.gcr.io/k8s-artifacts-prod/capi-kubeadm
IMAGE_NAME ?= cluster-api-kubeadm-controller
CONTROLLER_IMG ?= $(REGISTRY)/$(IMAGE_NAME)
TAG ?= dev
ARCH ?= amd64
ALL_ARCH = amd64 arm arm64 ppc64le s390x

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

TOOLS_DIR := hack/tools
CONTROLLER_GEN_BIN := bin/controller-gen
CONTROLLER_GEN := $(TOOLS_DIR)/$(CONTROLLER_GEN_BIN)
GOLANGCI_LINT_BIN := bin/golangci-lint
GOLANGCI_LINT := $(TOOLS_DIR)/$(GOLANGCI_LINT_BIN)

# Allow overriding manifest generation destination directory
MANIFEST_ROOT ?= "config"
CRD_ROOT ?= "$(MANIFEST_ROOT)/crd/bases"
WEBHOOK_ROOT ?= "$(MANIFEST_ROOT)/webhook"
RBAC_ROOT ?= "$(MANIFEST_ROOT)/rbac"

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE=on

.PHONY: all
all: manager

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: test
test: generate fmt vet ## Run tests
	go test ./... -coverprofile cover.out

.PHONY: manager
manager: generate fmt vet ## Build manager binary
	go build -o bin/manager main.go

.PHONY: run
run: generate fmt vet ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run ./main.go

.PHONY: install
install: generate ## Install CRDs into a cluster
	kubectl apply -f config/crd/bases

.PHONY: deploy
deploy: generate ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	kubectl apply -f config/crd/bases
	kubectl kustomize config/default | kubectl apply -f -

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Run golangci-lint against code
	$(GOLANGCI_LINT) run

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code
	$(MAKE) generate-manifests
	$(MAKE) generate-deepcopy

.PHONY: generate-deepcopy
generate-deepcopy: $(CONTROLLER_GEN) ## Generate deepcopy files
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate/boilerplate.generatego.txt paths=./api/...

.PHONY: generate-manifests
generate-manifests: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:dir=$(CRD_ROOT) output:webhook:dir=$(WEBHOOK_ROOT) output:rbac:dir=$(RBAC_ROOT)

# Build controller-gen
$(CONTROLLER_GEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR) && go build -o $(CONTROLLER_GEN_BIN) sigs.k8s.io/controller-tools/cmd/controller-gen
# Build golangci-lint
$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR) && go build -o $(GOLANGCI_LINT_BIN) github.com/golangci/golangci-lint/cmd/golangci-lint

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-build
docker-build: ## Build the docker image for controller-manager
	docker build --pull --build-arg ARCH=$(ARCH) . -t $(CONTROLLER_IMG)-$(ARCH):$(TAG)
	MANIFEST_IMG=$(CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) $(MAKE) set-manifest-image

.PHONY: docker-push
docker-push: ## Push the docker image
	docker push $(CONTROLLER_IMG)-$(ARCH):$(TAG)

## --------------------------------------
## Docker — All ARCH
## --------------------------------------

.PHONY: docker-build-all ## Build all the architecture docker images
docker-build-all: $(addprefix docker-build-,$(ALL_ARCH))

docker-build-%:
	$(MAKE) ARCH=$* docker-build

.PHONY: docker-push-all ## Push all the architecture docker images
docker-push-all: $(addprefix docker-push-,$(ALL_ARCH))
	$(MAKE) docker-push-manifest

docker-push-%:
	$(MAKE) ARCH=$* docker-push

.PHONY: docker-push-manifest
docker-push-manifest: ## Push the fat manifest docker image
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CONTROLLER_IMG}:${TAG} ${CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge ${CONTROLLER_IMG}:${TAG}
	MANIFEST_IMG=$(CONTROLLER_IMG) MANIFEST_TAG=$(TAG) $(MAKE) set-manifest-image

.PHONY: set-manifest-image
set-manifest-image:
	$(info Updating kustomize image patch file for manager resource)
	sed -i'' -e 's@image: .*@image: '"${MANIFEST_IMG}:$(MANIFEST_TAG)"'@' ./config/default/manager_image_patch.yaml

## --------------------------------------
## Release
## --------------------------------------

RELEASE_TAG := $(shell git describe --abbrev=0 2>/dev/null)

.PHONY: release
release:  ## Builds and push container images using the latest git tag for the commit
	@if [ -z "${RELEASE_TAG}" ]; then echo "RELEASE_TAG is not set"; exit 1; fi
	# Push the release image to the staging bucket first.
	REGISTRY=$(STAGING_REGISTRY) TAG=$(RELEASE_TAG) \
		$(MAKE) docker-build-all docker-push-all
	# Set the manifest image to the production bucket.
	MANIFEST_IMG=$(PROD_REGISTRY)/$(IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		$(MAKE) set-manifest-image
	# Generate release artifacts.
	mkdir -p out/
	kustomize build config/default > out/bootstrap-components.yaml

.PHONY: release-staging-latest
release-staging-latest: ## Builds and push container images to the staging bucket using "latest" tag
	REGISTRY=$(STAGING_REGISTRY) TAG=latest \
		$(MAKE) docker-build-all docker-push-all
