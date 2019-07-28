
REGISTRY ?= gcr.io/$(shell gcloud config get-value project)
# A release does not need to define this
MANAGER_IMAGE_NAME ?= cluster-api-bootstrap-provider-kubeadm
MANAGER_IMAGE_TAG ?= dev
PULL_POLICY ?= Always

# Used in docker-* targets.
MANAGER_IMAGE ?= $(REGISTRY)/$(MANAGER_IMAGE_NAME):$(MANAGER_IMAGE_TAG)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

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
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

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
generate: controller-gen
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

# find or download controller-gen
# download controller-gen if necessary
.PHONY: controller-gen
controller-gen:
ifeq (, $(shell which controller-gen))
	./hack/install-controller-runtime.sh
CONTROLLER_GEN=$(shell go env GOPATH)/bin/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif
