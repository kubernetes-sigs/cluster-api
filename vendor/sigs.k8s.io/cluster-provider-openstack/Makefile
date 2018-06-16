
GIT_HOST = sigs.k8s.io
PWD := $(shell pwd)
BASE_DIR := $(shell basename $(PWD))
# Keep an existing GOPATH, make a private one if it is undefined
GOPATH_DEFAULT := $(PWD)/.go
export GOPATH ?= $(GOPATH_DEFAULT)
GOBIN_DEFAULT := $(GOPATH)/bin
export GOBIN ?= $(GOBIN_DEFAULT)
TESTARGS_DEFAULT := "-v"
export TESTARGS ?= $(TESTARGS_DEFAULT)
PKG := $(shell awk  -F "\"" '/^ignored = / { print $$2 }' Gopkg.toml)
DEST := $(GOPATH)/src/$(GIT_HOST)/$(BASE_DIR)
SOURCES := $(shell find $(DEST) -name '*.go')

HAS_MERCURIAL := $(shell command -v hg;)
HAS_DEP := $(shell command -v dep;)
HAS_LINT := $(shell command -v golint;)
HAS_GOX := $(shell command -v gox;)
GOX_PARALLEL ?= 3
TARGETS ?= darwin/amd64 linux/amd64 linux/386 linux/arm linux/arm64 linux/ppc64le
DIST_DIRS         = find * -type d -exec

GOOS ?= $(shell go env GOOS)
VERSION ?= $(shell git describe --exact-match 2> /dev/null || \
                 git describe --match=$(git rev-parse --short=8 HEAD) --always --dirty --abbrev=8)
GOFLAGS   :=
TAGS      :=
LDFLAGS   := "-w -s -X 'main.version=${VERSION}'"
REGISTRY ?= k8sclusterprovider

ifneq ("$(DEST)", "$(PWD)")
    $(error Please run 'make' from $(DEST). Current directory is $(PWD))
endif

# CTI targets

$(GOBIN):
	echo "create gobin"
	mkdir -p $(GOBIN)

work: $(GOBIN)

depend: work
ifndef HAS_DEP
	curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
endif
	dep ensure

depend-update: work
	dep ensure -update

build: openstack-machine-controller

openstack-machine-controller: depend $(SOURCES)
	CGO_ENABLED=0 GOOS=$(GOOS) go build \
		-ldflags $(LDFLAGS) \
		-o openstack-machine-controller \
		cmd/openstack-machine-controller/main.go

test: unit functional

check: depend fmt vet lint

unit: depend
	go test -tags=unit $(shell go list ./...) $(TESTARGS)

functional:
	@echo "$@ not yet implemented"

fmt:
	hack/verify-gofmt.sh

lint:
ifndef HAS_LINT
		go get -u github.com/golang/lint/golint
		echo "installing lint"
endif
	hack/verify-golint.sh

vet:
	go vet ./...

cover: depend
	go test -tags=unit $(shell go list ./...) -cover

docs:
	@echo "$@ not yet implemented"

godoc:
	@echo "$@ not yet implemented"

releasenotes:
	@echo "Reno not yet implemented for this repo"

translation:
	@echo "$@ not yet implemented"

# Do the work here

# Set up the development environment
env:
	@echo "PWD: $(PWD)"
	@echo "BASE_DIR: $(BASE_DIR)"
	@echo "GOPATH: $(GOPATH)"
	@echo "GOROOT: $(GOROOT)"
	@echo "DEST: $(DEST)"
	@echo "PKG: $(PKG)"
	go version
	go env

clean:
	rm -rf _dist openstack-machine-controller

realclean: clean
	rm -rf vendor
	if [ "$(GOPATH)" = "$(GOPATH_DEFAULT)" ]; then \
		rm -rf $(GOPATH); \
	fi

shell:
	$(SHELL) -i

images: image-machine-controller

image-machine-controller: depend openstack-machine-controller
ifeq ($(GOOS),linux)
	cp openstack-machine-controller images/machine-controller
	docker build -t $(REGISTRY)/openstack-machine-controller:$(VERSION) images/machine-controller
	rm images/machine-controller/openstack-machine-controller
else
	$(error Please set GOOS=linux for building the image)
endif

upload-images: images
	@echo "push images to $(REGISTRY)"
	docker login -u="$(DOCKER_USERNAME)" -p="$(DOCKER_PASSWORD)";
	docker push $(REGISTRY)/openstack-machine-controller:$(VERSION)

version:
	@echo ${VERSION}

.PHONY: build-cross
build-cross: LDFLAGS += -extldflags "-static"
build-cross: depend
ifndef HAS_GOX
	go get -u github.com/mitchellh/gox
endif
	CGO_ENABLED=0 gox -parallel=$(GOX_PARALLEL) -output="_dist/{{.OS}}-{{.Arch}}/{{.Dir}}" -osarch='$(TARGETS)' $(GOFLAGS) $(if $(TAGS),-tags '$(TAGS)',) -ldflags '$(LDFLAGS)' $(GIT_HOST)/$(BASE_DIR)/cmd/openstack-machine-controller/

.PHONY: dist
dist: build-cross
	( \
		cd _dist && \
		$(DIST_DIRS) cp ../LICENSE {} \; && \
		$(DIST_DIRS) cp ../README.md {} \; && \
		$(DIST_DIRS) tar -zcf cluster-provider-openstack-$(VERSION)-{}.tar.gz {} \; && \
		$(DIST_DIRS) zip -r cluster-provider-openstack-$(VERSION)-{}.zip {} \; \
	)

.PHONY: build clean cover depend docs fmt functional lint realclean \
	relnotes test translation version build-cross dist
