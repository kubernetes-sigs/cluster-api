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

.PHONY: genapi genconversion genclientset gendeepcopy

all: generate build images

depend:
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep ensure -v

	# go libraries often ship BUILD and BUILD.bazel files, but they often don't work.
	# We delete them and regenerate them
	find vendor -name "BUILD" -delete
	find vendor -name "BUILD.bazel" -delete

	bazel run //:gazelle

generate: genapi genconversion genclientset gendeepcopy genopenapi

genapi: depend
	go build -o $$GOPATH/bin/apiregister-gen sigs.k8s.io/cluster-api/vendor/github.com/kubernetes-incubator/apiserver-builder/cmd/apiregister-gen
	apiregister-gen -i ./pkg/apis,./pkg/apis/cluster,./pkg/apis/cluster/v1alpha1

genconversion: depend
	go build -o $$GOPATH/bin/conversion-gen sigs.k8s.io/cluster-api/vendor/k8s.io/code-generator/cmd/conversion-gen
	conversion-gen -i ./pkg/apis/cluster/v1alpha1/ -O zz_generated.conversion --go-header-file boilerplate.go.txt

genclientset: depend
	go build -o $$GOPATH/bin/client-gen sigs.k8s.io/cluster-api/vendor/k8s.io/code-generator/cmd/client-gen
	client-gen \
	  --input="cluster/v1alpha1" \
		--clientset-name="clientset" \
		--input-base="sigs.k8s.io/cluster-api/pkg/apis" \
		--output-package "sigs.k8s.io/cluster-api/pkg/client/clientset_generated" \
		--go-header-file boilerplate.go.txt \
		--clientset-path sigs.k8s.io/cluster-api/pkg/client/clientset_generated

gendeepcopy:
	go build -o $$GOPATH/bin/deepcopy-gen sigs.k8s.io/cluster-api/vendor/k8s.io/code-generator/cmd/deepcopy-gen
	deepcopy-gen \
	  -i ./pkg/apis/cluster/,./pkg/apis/cluster/v1alpha1/ \
	  -O zz_generated.deepcopy \
	  -h boilerplate.go.txt

STATIC_API_DIRS = k8s.io/apimachinery/pkg/apis/meta/v1
STATIC_API_DIRS += k8s.io/apimachinery/pkg/api/resource
STATIC_API_DIRS += k8s.io/apimachinery/pkg/version
STATIC_API_DIRS += k8s.io/apimachinery/pkg/runtime
STATIC_API_DIRS += k8s.io/apimachinery/pkg/util/intstr
STATIC_API_DIRS += k8s.io/api/core/v1

# Automatically extract vendored apis under vendor/k8s.io/api.
VENDOR_API_DIRS := $(shell find vendor/k8s.io/api -type d | grep -E 'v[[:digit:]]+(alpha[[:digit:]]+|beta[[:digit:]]+)*' | sed -e 's/^vendor\///')

empty:=
comma:=,
space:=$(empty) $(empty)

genopenapi: static_apis = $(subst $(space),$(comma),$(STATIC_API_DIRS))
genopenapi: vendor_apis = $(subst $(space),$(comma),$(VENDOR_API_DIRS))

genopenapi:
	go build -o $$GOPATH/bin/openapi-gen sigs.k8s.io/cluster-api/vendor/k8s.io/code-generator/cmd/openapi-gen
	openapi-gen \
	  --input-dirs $(static_apis) \
	  --input-dirs $(vendor_apis) \
	  --input-dirs ./pkg/apis/cluster/,./pkg/apis/cluster/v1alpha1/ \
	  --output-package "sigs.k8s.io/cluster-api/pkg/openapi" \
	  --go-header-file boilerplate.go.txt

build: depend
	CGO_ENABLED=0 go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cmd/apiserver
	CGO_ENABLED=0 go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cmd/controller-manager

images: depend
	$(MAKE) -C cmd/apiserver image
	$(MAKE) -C cmd/controller-manager image

verify:
	./hack/verify_boilerplate.py
