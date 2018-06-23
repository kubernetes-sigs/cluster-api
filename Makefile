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

generate: genapi genconversion genclientset gendeepcopy

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
	  -i ./pkg/apis/cluster/,./pkg/apis/cluster/v1alpha1/,./cloud/google/gceproviderconfig/v1alpha1,./cloud/google/gceproviderconfig,./cloud/vsphere/vsphereproviderconfig/v1alpha1,./cloud/vsphere/vsphereproviderconfig \
	  -O zz_generated.deepcopy \
	  -h boilerplate.go.txt

build: depend
	CGO_ENABLED=0 go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cloud/google/cmd/gce-controller
	CGO_ENABLED=0 go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cloud/vsphere/cmd/vsphere-machine-controller
	CGO_ENABLED=0 go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cmd/apiserver
	CGO_ENABLED=0 go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cmd/controller-manager

images: depend
	$(MAKE) -C cloud/google/cmd/gce-controller image
	$(MAKE) -C cloud/vsphere/cmd/vsphere-machine-controller image
	$(MAKE) -C cmd/apiserver image
	$(MAKE) -C cmd/controller-manager image

verify:
	./hack/verify_boilerplate.py
