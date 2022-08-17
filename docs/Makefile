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

SHELL:=/usr/bin/env bash

.DEFAULT_GOAL:=help

DIAGRAM_SRCS := $(call rwildcard,.,*.md,*.plantuml)

# Hosts running SELinux need :z added to volume mounts
SELINUX_ENABLED := $(shell cat /sys/fs/selinux/enforce 2> /dev/null || echo 0)

ifeq ($(SELINUX_ENABLED),1)
  DOCKER_VOL_OPTS?=:z
endif

##@ PlantUML diagrams

.PHONY: diagrams
diagrams: plantuml-builder  diagrams-book diagrams-proposals ## Make all diagrams

.PHONY: diagrams-book
diagrams-book: plantuml-builder ## Make all book diagrams
	-docker run -u $(UID):$(GID) -v $(abspath .):/docs$(DOCKER_VOL_OPTS) plantuml-builder /docs/book/**/*.md
	-docker run -u $(UID):$(GID) -v $(abspath .):/docs$(DOCKER_VOL_OPTS) plantuml-builder /docs/book/**/*.plantuml

.PHONY: diagrams-proposals
diagrams-proposals: plantuml-builder ## Make all proposals diagrams
	-docker run -u $(UID):$(GID) -v $(abspath .):/docs$(DOCKER_VOL_OPTS) plantuml-builder /docs/proposals/**/*.md
	-docker run -u $(UID):$(GID) -v $(abspath .):/docs$(DOCKER_VOL_OPTS) plantuml-builder /docs/proposals/**/*.plantuml

.PHONY: plantuml-builder
plantuml-builder: Dockerfile  ## Make diagram build container
	docker build -f Dockerfile -t "plantuml-builder" .

##@ general

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
