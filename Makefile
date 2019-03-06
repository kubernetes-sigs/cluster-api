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

# Image URL to use all building/pushing image targets
export CONTROLLER_IMG ?= gcr.io/k8s-cluster-api/cluster-api-controller:latest
export EXAMPLE_PROVIDER_IMG ?= gcr.io/k8s-cluster-api/example-provider-controller:latest

all: test manager clusterctl

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: gazelle
gazelle: ## Run Bazel Gazelle
	(which bazel && ./hack/update-bazel.sh) || true

.PHONY: dep-ensure
dep-ensure: ## Runs dep-ensure and rebuilds Bazel gazelle files.
	find vendor -name 'BUILD.bazel' -delete
	(which dep && dep ensure -v) || true
	$(MAKE) gazelle

.PHONY: test
test: gazelle verify generate fmt vet manifests ## Run tests
	go test -v -tags=integration ./pkg/... ./cmd/...

.PHONY: manager
manager: generate fmt vet ## Build manager binary
	go build -o bin/manager sigs.k8s.io/cluster-api/cmd/manager

.PHONY: clusterctl
clusterctl: generate fmt vet ## Build clusterctl binary
	go build -o bin/clusterctl sigs.k8s.io/cluster-api/cmd/clusterctl

.PHONY: run
run: generate fmt vet ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run ./cmd/manager/main.go

.PHONY: deploy
deploy: manifests ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	kustomize build config/default | kubectl apply -f -

.PHONY: manifests
manifests: ## Generate manifests e.g. CRD, RBAC etc.
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
	cp -f ./config/rbac/rbac*.yaml ./config/ci/rbac/
	cp -f ./config/manager/manager*.yaml ./config/ci/manager/

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./pkg/... ./cmd/...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./pkg/... ./cmd/...

.PHONY: lint
lint: dep-ensure ## Lint codebase
	bazel run //:lint $(BAZEL_ARGS)

lint-full: dep-ensure ## Run slower linters to detect possible issues
	bazel run //:lint-full $(BAZEL_ARGS)

.PHONY: generate
generate: clientset dep-ensure ## Generate code
	go generate ./pkg/... ./cmd/...
	$(MAKE) gazelle

.PHONY: clientset
clientset: ## Generate a typed clientset
	rm -rf pkg/client
	cd ./vendor/k8s.io/code-generator/cmd && go install ./client-gen ./lister-gen ./informer-gen
	$$GOPATH/bin/client-gen --clientset-name clientset --input-base sigs.k8s.io/cluster-api/pkg/apis \
		--input cluster/v1alpha1 --output-package sigs.k8s.io/cluster-api/pkg/client/clientset_generated \
		--go-header-file=./hack/boilerplate.go.txt
	$$GOPATH/bin/lister-gen --input-dirs sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1 \
		--output-package sigs.k8s.io/cluster-api/pkg/client/listers_generated \
		--go-header-file=./hack/boilerplate.go.txt
	$$GOPATH/bin/informer-gen --input-dirs sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1 \
		--versioned-clientset-package sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset \
		--listers-package sigs.k8s.io/cluster-api/pkg/client/listers_generated \
		--output-package sigs.k8s.io/cluster-api/pkg/client/informers_generated \
		--go-header-file=./hack/boilerplate.go.txt
	$(MAKE) gazelle

.PHONY: clean
clean: ## Remove all generated files
	rm -f bazel-*

.PHONY: docker-build
docker-build: generate fmt vet manifests ## Build the docker image for controller-manager
	docker build . -t ${CONTROLLER_IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i.tmp -e 's@image: .*@image: '"${CONTROLLER_IMG}"'@' ./config/default/manager_image_patch.yaml

.PHONY: docker-push
docker-push: docker-build ## Push the docker image
	docker push "$(CONTROLLER_IMG)"

.PHONY: docker-build-ci
docker-build-ci: generate fmt vet manifests ## Build the docker image for example provider
	docker build . -f ./pkg/provider/example/container/Dockerfile -t ${EXAMPLE_PROVIDER_IMG}
	@echo "updating kustomize image patch file for ci"
	sed -i.tmp -e 's@image: .*@image: '"${EXAMPLE_PROVIDER_IMG}"'@' ./config/ci/manager_image_patch.yaml

.PHONY: docker-push-ci
docker-push-ci: docker-build-ci  ## Build the docker image for ci
	docker push "$(EXAMPLE_PROVIDER_IMG)"

.PHONY: verify
verify:
	./hack/verify_boilerplate.py
	./hack/verify_clientset.sh
	./hack/verify-bazel.sh
