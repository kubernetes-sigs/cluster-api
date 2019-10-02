# If you update this file, please follow:
# https://suva.sh/posts/well-documented-makefiles/

.DEFAULT_GOAL:=help

# Use GOPROXY environment variable if set
GOPROXY := $(shell go env GOPROXY)
GOPROXY ?= https://proxy.golang.org
export GOPROXY

REGISTRY ?= gcr.io/$(shell gcloud config get-value project)
# Image URL to use all building/pushing image targets
IMAGE_NAME ?= manager
IMAGE_TAG ?= dev
IMG ?= $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

TOOLS_DIR := hack/tools
CONTROLLER_GEN_BIN := bin/controller-gen
CONTROLLER_GEN := $(TOOLS_DIR)/$(CONTROLLER_GEN_BIN)
RELEASE_NOTES_BIN := bin/release-notes
RELEASE_NOTES := $(TOOLS_DIR)/$(RELEASE_NOTES_BIN)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE=on

all: manager

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

test: generate fmt vet manifests ## Run tests
	go test ./api/... ./controllers/... -coverprofile cover.out

manager: generate fmt vet ## Build manager binary
	go build -o bin/manager main.go

run: generate fmt vet ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run main.go

install: manifests ## Install CRDs into a cluster
	kubectl apply -f config/crd/bases

deploy: manifests ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	kubectl apply -f config/crd/bases
	kustomize build config/default | kubectl apply -f -

manifests: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

fmt: ## Run go fmt against code
	go fmt ./...

vet: ## Run go vet against code
	go vet ./...

generate: $(CONTROLLER_GEN) ## Generate code
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...

docker-build: test ## Build the docker image
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

docker-push: ## Push the docker image
	docker push ${IMG}

# Build controller-gen
$(CONTROLLER_GEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR) && go build -o $(CONTROLLER_GEN_BIN) sigs.k8s.io/controller-tools/cmd/controller-gen

$(RELEASE_NOTES) : $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR) && go build -o $(RELEASE_NOTES_BIN) -tags tools sigs.k8s.io/cluster-api/hack/tools/release

.PHONY: release-notes
release-notes: $(RELEASE_NOTES)
	$(RELEASE_NOTES)
