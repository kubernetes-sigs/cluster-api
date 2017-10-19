ifndef VERBOSE
	MAKEFLAGS += --silent
endif

PKGS=$(shell go list ./... | grep -v /vendor)
CI_PKGS=$(shell go list ./... | grep -v /vendor | grep -v test)
FMT_PKGS=$(shell go list -f {{.Dir}} ./... | grep -v vendor | grep -v test | tail -n +2)
SHELL_IMAGE=golang:1.8.3
GIT_SHA=$(shell git rev-parse --verify HEAD)
VERSION=$(shell cat VERSION)
PWD=$(shell pwd)

default: authorsfile compile ## Parse Bootstrap scripts and create kubicorn executable in the ./bin directory and the AUTHORS file.

all: default install

compile: ## Create the kubicorn executable in the ./bin directory.
	go build -o bin/kubicorn -ldflags "-X github.com/kris-nova/kubicorn/cmd.GitSha=${GIT_SHA} -X github.com/kris-nova/kubicorn/cmd.Version=${VERSION}" main.go

install: ## Create the kubicorn executable in $GOPATH/bin directory.
	install -m 0755 bin/kubicorn ${GOPATH}/bin/kubicorn

bindata: ## Generate the bindata for the bootstrap scripts that are built into the binary
	which go-bindata > /dev/null || go get -u github.com/jteeuwen/go-bindata/...
	rm -rf bootstrap/bootstrap.go
	go-bindata -pkg bootstrap -o bootstrap/bootstrap.go bootstrap/ bootstrap/vpn

build: authors clean build-linux-amd64 build-darwin-amd64 build-freebsd-amd64 build-windows-amd64

authorsfile: ## Update the AUTHORS file from the git logs
	git log --all --format='%aN <%cE>' | sort -u | egrep -v "noreply|mailchimp|@Kris" > AUTHORS

clean: ## Clean the project tree from binary files, and any bootstrap files.
	rm -rf bin/*
	rm -rf bootstrap/bootstrap.go

gofmt: install-tools
	echo "Fixing format of go files..."; \
	for package in $(FMT_PKGS); \
	do \
		gofmt -w $$package ; \
		goimports -l -w $$package ; \
	done

# Because of https://github.com/golang/go/issues/6376 We actually have to build this in a container
build-linux-amd64: ## Create the kubicorn executable for Linux 64-bit OS in the ./bin directory. Requires Docker.
	mkdir -p bin
	docker run \
	-it \
	-w /go/src/github.com/kris-nova/kubicorn \
	-v ${PWD}:/go/src/github.com/kris-nova/kubicorn \
	-e GOPATH=/go \
	--rm golang:1.8.1 make docker-build-linux-amd64

docker-build-linux-amd64:
	go build -v -o bin/linux-amd64

build-darwin-amd64: ## Create the kubicorn executable for Darwin (osX) 64-bit OS in the ./bin directory. Requires Docker.
	GOOS=darwin GOARCH=amd64 go build -v -o bin/darwin-amd64 &

build-freebsd-amd64: ## Create the kubicorn executable for FreeBSD 64-bit OS in the ./bin directory. Requires Docker.
	GOOS=freebsd GOARCH=amd64 go build -v -o bin/freebsd-amd64 &

build-windows-amd64: ## Create the kubicorn executable for Windows 64-bit OS in the ./bin directory. Requires Docker.
	GOOS=windows GOARCH=amd64 go build -v -o bin/windows-amd64 &

linux: shell
shell: ## Exec into a container with the kubicorn source mounted inside
	docker run \
	-i -t \
	-w /go/src/github.com/kris-nova/kubicorn \
	-v ${PWD}:/go/src/github.com/kris-nova/kubicorn \
	--rm ${SHELL_IMAGE} /bin/bash

lint: install-tools ## check for style mistakes all Go files using golint
	golint $(PKGS)

bashlint:
	if which shellcheck > /dev/null; then \
	find . -path ./vendor -prune -o -name '*.sh' -print0 | xargs -n1 -0 shellcheck -s bash -e SC2001; \
	else \
	 echo shellcheck not installed. try something like: apt-get install shellcheck; \
	fi

# versioning
bump-major:
	./scripts/bump-version.sh major

bump-minor:
	./scripts/bump-version.sh minor

bump-patch:
	./scripts/bump-version.sh patch

.PHONY: test
test: ## Run the INTEGRATION TESTS. This will create cloud resources and potentially cost money.
	go test -timeout 20m -v $(PKGS)

.PHONY: ci
ci: ## Run the CI TESTS. This will never cost money, and will never communicate with a cloud API.
	go test -timeout 20m -v $(CI_PKGS)

.PHONY: check-code
check-code: install-tools ## Run code checks
	PKGS="${FMT_PKGS}" GOFMT="gofmt" GOLINT="golint" ./scripts/ci-checks.sh

vet: ## apply go vet to all the Go files
	@go vet $(PKGS)

check-headers: ## Check if the headers are valid. This is ran in CI.
	./scripts/check-header.sh

update-headers: ## Update the headers in the repository. Required for all new files.
	./scripts/headers.sh

.PHONY: apimachinery
apimachinery:
	go get k8s.io/kubernetes/cmd/libs/go2idl/conversion-gen
	go get k8s.io/kubernetes/cmd/libs/go2idl/defaulter-gen
	${GOPATH}/bin/conversion-gen --skip-unsafe=true --input-dirs github.com/kris-nova/kubicorn/apis/cluster/v1alpha1 --v=0  --output-file-base=zz_generated.conversion
	${GOPATH}/bin/conversion-gen --skip-unsafe=true --input-dirs github.com/kris-nova/kubicorn/apis/cluster/v1alpha1 --v=0  --output-file-base=zz_generated.conversion
	${GOPATH}/bin/defaulter-gen --input-dirs github.com/kris-nova/kubicorn/apis/cluster/v1alpha1 --v=0  --output-file-base=zz_generated.defaults
	${GOPATH}/bin/defaulter-gen --input-dirs github.com/kris-nova/kubicorn/apis/cluster/v1alpha1 --v=0  --output-file-base=zz_generated.defaults

.PHONY: install-tools
install-tools:
	GOIMPORTS_CMD=$(shell command -v goimports 2> /dev/null)
ifndef GOIMPORTS_CMD
	go get golang.org/x/tools/cmd/goimports
endif

	GOLINT_CMD=$(shell command -v golint 2> /dev/null)
ifndef GOLINT_CMD
	go get github.com/golang/lint/golint
endif

.PHONY: help
help:  ## Show help messages for make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}'
