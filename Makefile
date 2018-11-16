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

# Default timeout for starting/stopping the Kubebuilder test controlplane
export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?=60s
export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?=60s

# Image URL to use all building/pushing image targets
IMG ?= gcr.io/k8s-cluster-api/cluster-api-controller:latest

all: test manager clusterctl

# Run tests
test: generate fmt vet manifests verify
	go test -v -tags=integration ./pkg/... ./cmd/...
#	go test -v -tags=integration ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager sigs.k8s.io/cluster-api/cmd/manager

# Build clusterctl binary
clusterctl: generate fmt vet
	go build -o bin/clusterctl sigs.k8s.io/cluster-api/cmd/clusterctl

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
	@# Kubebuilder CRD generation can't handle intstr.IntOrString properly:
	@# https://github.com/kubernetes-sigs/kubebuilder/issues/442
	sed -i -e 's/maxSurge:/maxSurge: {}/g' -e '/maxSurge:/{n;d};' config/crds/cluster_v1alpha1_machinedeployment.yaml
	sed -i -e 's/maxUnavailable:/maxUnavailable: {}/g' -e '/maxUnavailable:/{n;d};' config/crds/cluster_v1alpha1_machinedeployment.yaml

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate: clientset
	go generate ./pkg/... ./cmd/...

# Generate a typed clientset
clientset:
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

# Build the docker image
docker-build: generate fmt vet manifests
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

verify:
	./hack/verify_boilerplate.py
	./hack/verify_clientset.sh

# Push the docker image
docker-push:
	docker push ${IMG}
