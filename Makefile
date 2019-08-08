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

# Used in docker-* targets.
MANAGER_IMAGE ?= $(REGISTRY)/$(MANAGER_IMAGE_NAME):$(MANAGER_IMAGE_TAG)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

TOOLS_DIR := hack/tools
CONTROLLER_GEN_BIN := bin/controller-gen
CONTROLLER_GEN := $(TOOLS_DIR)/$(CONTROLLER_GEN_BIN)

# Allow overriding manifest generation destination directory
MANIFEST_ROOT ?= "config"
CRD_ROOT ?= "$(MANIFEST_ROOT)/crd/bases"
WEBHOOK_ROOT ?= "$(MANIFEST_ROOT)/webhook"
RBAC_ROOT ?= "$(MANIFEST_ROOT)/rbac"

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE=on

.PHONY: all
all: manager

# Run tests
.PHONY: test
test: generate fmt vet manifests
	go test ./api/... ./controllers/... -coverprofile cover.out

# Build manager binary
.PHONY: manager
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run: generate fmt vet
	go run ./main.go

# Install CRDs into a cluster
.PHONY: install
install: manifests
	kubectl apply -f config/crd/bases

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy: manifests
	kubectl apply -f config/crd/bases
	kubectl kustomize config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:dir=$(CRD_ROOT) output:webhook:dir=$(WEBHOOK_ROOT) output:rbac:dir=$(RBAC_ROOT)

# Run go fmt against code
.PHONY: fmt
fmt:
	go fmt ./...

# Run go vet against code
.PHONY: vet
vet:
	go vet ./...

# Generate code
.PHONY: generate
generate: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...

# Build the docker image
.PHONY: docker-build
docker-build: test
	docker build . -t ${MANAGER_IMAGE}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${MANAGER_IMAGE}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
.PHONY: docker-push
docker-push:
	docker push ${MANAGER_IMAGE}

# Build controller-gen
$(CONTROLLER_GEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR) && go build -o $(CONTROLLER_GEN_BIN) sigs.k8s.io/controller-tools/cmd/controller-gen
