# Copyright 2020 The Kubernetes Authors.
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

ROOT_DIR_RELATIVE := ../..
include $(ROOT_DIR_RELATIVE)/common.mk

UNAME := $(shell uname -s)

# Directories.
BIN_DIR := bin
SHARE_DIR := share

OS := $(shell go env GOOS)
RUST_TARGET := unknown-$(OS)-gnu
MDBOOK_EXTRACT_COMMAND := tar xfvz $(SHARE_DIR)/mdbook.tar.gz -C bin
MDBOOK_ARCHIVE_EXT := .tar.gz

ifeq ($(OS), windows)
	RUST_TARGET := pc-windows-msvc
	MDBOOK_ARCHIVE_EXT := .zip
	MDBOOK_EXTRACT_COMMAND := unzip -d /tmp
endif

ifeq ($(OS), darwin)
	RUST_TARGET := apple-darwin
endif

## --------------------------------------
## Tooling Binaries
## --------------------------------------

.PHONY: modules
modules: ## Runs go mod to ensure modules are up to date.
	go mod tidy

$(BIN_DIR):
	mkdir -p $@

$(SHARE_DIR):
	mkdir -p $@

CONTROLLER_GEN := $(BIN_DIR)/controller-gen
$(CONTROLLER_GEN): $(BIN_DIR) go.mod go.sum # Build controller-gen from tools folder.
	go build -tags=tools -o $@ sigs.k8s.io/controller-tools/cmd/controller-gen

CONVERSION_GEN := $(BIN_DIR)/conversion-gen
$(CONVERSION_GEN): $(BIN_DIR) go.mod go.sum
	go build -tags=tools -o $@ k8s.io/code-generator/cmd/conversion-gen

DEFAULTER_GEN := $(BIN_DIR)/defaulter-gen
$(DEFAULTER_GEN): $(BIN_DIR) go.mod go.sum
	go build -tags=tools -o $@ k8s.io/code-generator/cmd/defaulter-gen

ENVSUBST := $(BIN_DIR)/envsubst
$(ENVSUBST): $(BIN_DIR) go.mod go.sum # Build envsubst from tools folder.
	go build -tags=tools -o $@ github.com/a8m/envsubst/cmd/envsubst

GINKGO := $(BIN_DIR)/ginkgo
$(GINKGO): $(BIN_DIR) go.mod go.sum
	go build -tags=tools -o $@ github.com/onsi/ginkgo/ginkgo

GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
$(GOLANGCI_LINT): $(BIN_DIR) go.mod go.sum # Build golangci-lint from tools folder.
	go build -tags=tools -o $@ github.com/golangci/golangci-lint/cmd/golangci-lint

KIND := $(BIN_DIR)/kind
$(KIND): $(BIN_DIR) go.mod go.sum
	go build -tags tools -o $@ sigs.k8s.io/kind

KUSTOMIZE := $(BIN_DIR)/kustomize
$(KUSTOMIZE): $(BIN_DIR) go.mod go.sum # Build kustomize from tools folder.
	go build -tags=tools -o $@ sigs.k8s.io/kustomize/kustomize/v3

MDBOOK_EMBED := $(BIN_DIR)/mdbook-embed
$(MDBOOK_EMBED): $(BIN_DIR) go.mod go.sum
	go build -tags=tools -o $(BIN_DIR)/mdbook-embed ./mdbook/embed

MDBOOK_RELEASELINK := $(BIN_DIR)/mdbook-releaselink
$(MDBOOK_RELEASELINK): $(BIN_DIR) go.mod go.sum
	go build -tags=tools -o $(BIN_DIR)/mdbook-releaselink ./mdbook/releaselink

MDBOOK_TABULATE := $(BIN_DIR)/mdbook-tabulate
$(MDBOOK_TABULATE): $(BIN_DIR) go.mod go.sum
	go build -tags=tools -o $(BIN_DIR)/mdbook-tabulate ./mdbook/tabulate

MOCKGEN := $(BIN_DIR)/mockgen
$(MOCKGEN): $(BIN_DIR) go.mod go.sum # Build mockgen from tools folder.
	go build -tags=tools -o $@ github.com/golang/mock/mockgen

RELEASE_NOTES := $(BIN_DIR)/release-notes
$(RELEASE_NOTES): $(BIN_DIR) go.mod go.sum
	go build -tags tools -o $@ ./release

LINK_CHECKER := $(BIN_DIR)/link-checker
$(LINK_CHECKER): $(BIN_DIR) go.mod go.sum
	go build -tags tools -o $@ github.com/raviqqe/liche

GOBINDATA := $(BIN_DIR)/go-bindata
$(GOBINDATA): $(BIN_DIR) go.mod go.sum
	go build -tags tools -o $@ github.com/go-bindata/go-bindata/go-bindata

GOAPIDIFF := $(BIN_DIR)/go-apidiff
$(GOAPIDIFF): $(BIN_DIR) go.mod go.sum
	go build -tags tools -o $@ github.com/joelanford/go-apidiff
