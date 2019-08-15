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

.DEFAULT_GOAL:=help

# Default timeout for starting/stopping the Kubebuilder test control plane
export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?=60s
export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?=60s

# This option is for running docker manifest command
export DOCKER_CLI_EXPERIMENTAL := enabled

# Image URL to use all building/pushing image targets
REGISTRY ?= gcr.io/$(shell gcloud config get-value project)
CONTROLLER_IMG ?= $(REGISTRY)/cluster-api-controller
EXAMPLE_PROVIDER_IMG ?= $(REGISTRY)/example-provider-controller
TAG ?= dev

ARCH?=amd64
ALL_ARCH = amd64 arm arm64 ppc64le s390x
GOOS?=linux

all: test manager clusterctl

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Testing
## --------------------------------------

.PHONY: test
test: generate lint ## Run tests
	$(MAKE) test-go

.PHONY: test-go
test-go: ## Run tests
	go test -v ./...

## --------------------------------------
## Binaries
## --------------------------------------

.PHONY: manager
manager: lint-full ## Build manager binary
	go build -o bin/manager sigs.k8s.io/cluster-api/cmd/manager

.PHONY: docker-build-manager
docker-build-manager: lint-full ## Build manager binary in docker
	docker run --rm \
	-v "${PWD}:/go/src/sigs.k8s.io/cluster-api" \
	-v "${PWD}/bin:/go/bin" \
	-w "/go/src/sigs.k8s.io/cluster-api" \
	-e CGO_ENABLED=0 -e GOOS=${GOOS} -e GOARCH=${ARCH} -e GO111MODULE=on -e GOFLAGS="-mod=vendor" \
	golang:1.12.6 \
	go build -a -ldflags '-extldflags "-static"' -o /go/bin/manager sigs.k8s.io/cluster-api/cmd/manager

.PHONY: clusterctl
clusterctl: lint-full ## Build clusterctl binary
	go build -o bin/clusterctl sigs.k8s.io/cluster-api/cmd/clusterctl

.PHONY: run
run: lint ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run ./cmd/manager/main.go

## --------------------------------------
## Linting
## --------------------------------------

.PHONY: lint
lint: ## Lint codebase
	bazel run //:lint $(BAZEL_ARGS)

lint-full: ## Run slower linters to detect possible issues
	bazel run //:lint-full $(BAZEL_ARGS)

## --------------------------------------
## Generate / Manifests
## --------------------------------------

.PHONY: generate
generate: ## Generate code
	$(MAKE) generate-manifests
	$(MAKE) generate-deepcopy
	$(MAKE) gazelle

.PHONY: generate-deepcopy
generate-deepcopy: ## Runs controller-gen to generate deepcopy files
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./api/...

.PHONY: generate-manifests
generate-manifests: ## Generate manifests e.g. CRD, RBAC etc.
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go \
		paths=./api/... \
		paths=./pkg/controller/... \
		crd:trivialVersions=true \
		rbac:roleName=manager-role \
		output:crd:dir=./config/crd/bases
	## Copy files in CI folders.
	cp -f ./config/rbac/role*.yaml ./config/ci/rbac/
	cp -f ./config/manager/manager*.yaml ./config/ci/manager/

.PHONY: gazelle
gazelle: ## Run Bazel Gazelle
	(which bazel && ./hack/update-bazel.sh) || true

.PHONY: vendor
vendor: ## Runs go mod to ensure proper vendoring.
	./hack/update-vendor.sh
	$(MAKE) gazelle

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-build
docker-build: generate lint-full ## Build the docker image for controller-manager
	docker build --pull --build-arg ARCH=$(ARCH) . -t $(CONTROLLER_IMG)-$(ARCH):$(TAG)
	@echo "updating kustomize image patch file for manager resource"
	hack/sed.sh -i.tmp -e 's@image: .*@image: '"${CONTROLLER_IMG}-$(ARCH):$(TAG)"'@' ./config/default/manager_image_patch.yaml

.PHONY: docker-push
docker-push: docker-build ## Push the docker image
	docker push $(CONTROLLER_IMG)-$(ARCH):$(TAG)

.PHONY: all-docker-build
all-docker-build: $(addprefix sub-docker-build-,$(ALL_ARCH))
	@echo "updating kustomize image patch file for manager resource"
	hack/sed.sh -i.tmp -e 's@image: .*@image: '"$(CONTROLLER_IMG):$(TAG)"'@' ./config/default/manager_image_patch.yaml

sub-docker-build-%:
	$(MAKE) ARCH=$* docker-build

.PHONY:all-push ## Push all the architecture docker images and fat manifest docker image
all-push: all-docker-push docker-push-manifest

.PHONY:all-docker-push ## Push all the architecture docker images
all-docker-push: $(addprefix sub-docker-push-,$(ALL_ARCH))

sub-docker-push-%:
	$(MAKE) ARCH=$* docker-push

.PHONY: docker-build-ci
docker-build-ci: generate lint-full ## Build the docker image for example provider
	docker build --pull --build-arg ARCH=$(ARCH) . -f ./cmd/example-provider/Dockerfile -t $(EXAMPLE_PROVIDER_IMG)-$(ARCH):$(TAG)
	@echo "updating kustomize image patch file for ci"
	hack/sed.sh -i.tmp -e 's@image: .*@image: '"${EXAMPLE_PROVIDER_IMG}-$(ARCH):$(TAG)"'@' ./config/ci/manager_image_patch.yaml

.PHONY: docker-push-ci
docker-push-ci: docker-build-ci  ## Build the docker image for ci
	docker push "$(EXAMPLE_PROVIDER_IMG)-$(ARCH):$(TAG)"

.PHONY: docker-push-manifest
docker-push-manifest: ## Push the fat manifest docker image. TODO: Update bazel build to push manifest once https://github.com/bazelbuild/rules_docker/issues/300 get merged
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images
	docker manifest create --amend $(CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CONTROLLER_IMG}:${TAG} ${CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge ${CONTROLLER_IMG}:${TAG}

## --------------------------------------
## Cleanup / Verification
## --------------------------------------

.PHONY: clean
clean: ## Remove all generated files
	$(MAKE) clean-bazel
	$(MAKE) clean-bin
	$(MAKE) clean-book

.PHONY: clean-bazel
clean-bazel: ## Remove all generated bazel symlinks
	bazel clean

.PHONY: clean-bin
clean-bin: ## Remove all generated binaries
	rm -rf bin

.PHONY: clean-clientset
clean-clientset: ## Remove all generated clientset files
	rm -rf pkg/client

.PHONY: verify
verify:
	./hack/verify-boilerplate.sh
	./hack/verify-bazel.sh
	./hack/verify-doctoc.sh
	./hack/verify-generated-files.sh

.PHONY: clean-book
clean-book: ## Remove all generated GitBook files
	rm -rf ./docs/book/_book

## --------------------------------------
## Others / Utilities
## --------------------------------------

.PHONY: diagrams
diagrams: ## Build proposal diagrams
	$(MAKE) -C docs/proposals/images

.PHONY: book
book: ## Build the GitBook
	./hack/build-gitbook.sh

.PHONY: serve-book
serve-book: ## Build and serve the GitBook with live-reloading enabled
	(cd ./docs/book && gitbook serve --live --open)
