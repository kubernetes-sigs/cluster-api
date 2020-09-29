# Copyright 2019 The Kubernetes Authors.
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

# Directories.
ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
TOOLS_DIR := $(realpath ../../hack/tools)
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin
BIN_DIR := bin
CRATE_INSTALL := $(realpath ../../third_party/japaric-trust/crate_install.sh)

export PATH := $(TOOLS_BIN_DIR):$(PATH)

# Binaries.
TABULATE := $(TOOLS_BIN_DIR)/mdbook-tabulate
$(TABULATE): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/mdbook-tabulate ./mdbook/tabulate

EMBED := $(TOOLS_BIN_DIR)/mdbook-embed
$(EMBED): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/mdbook-embed ./mdbook/embed

RELEASELINK := $(TOOLS_BIN_DIR)/mdbook-releaselink
$(RELEASELINK): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/mdbook-releaselink ./mdbook/releaselink

MDBOOK := $(TOOLS_BIN_DIR)/mdbook
$(MDBOOK):
	$(CRATE_INSTALL) --git rust-lang/mdBook --tag v0.4.3 --to $(TOOLS_BIN_DIR) --force

MDBOOK_LINKCHECK := $(TOOLS_BIN_DIR)/mdbook-linkcheck
$(MDBOOK_LINKCHECK):
	$(CRATE_INSTALL) --git Michael-F-Bryan/mdbook-linkcheck --tag v0.7.0 --to $(TOOLS_BIN_DIR) --force

.PHONY: serve
serve: $(MDBOOK) $(TABULATE) $(RELEASELINK) clean-linkcheck
	$(MDBOOK) serve

.PHONY: build
build: $(MDBOOK) $(TABULATE) $(RELEASELINK) clean-linkcheck
	$(MDBOOK) build

.PHONY: verify
verify: $(MDBOOK) $(TABULATE) $(RELEASELINK) $(MDBOOK_LINKCHECK)
	$(MDBOOK) build

# We remove the linkcheck binary when not running `verify` because mdbook
# always run optional plugins if they are installed, but will only print a
# warning if they're not. This is the best solution we could figure out for how
# to integrate it into the mdbook plugin model while not gating all mdbook
# tasks on verification.
.PHONY: clean-linkcheck
clean-linkcheck:
	rm -f $(MDBOOK_LINKCHECK)
