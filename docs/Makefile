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

ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

SOURCES := $(shell find ${ROOT_DIR} -name \*.plantuml)
DIAGRAMS := $(SOURCES:%.plantuml=%.png)

# Hosts running SELinux need :z added to volume mounts
SELINUX_ENABLED := $(shell cat /sys/fs/selinux/enforce 2> /dev/null || echo 0)

ifeq ($(SELINUX_ENABLED),1)
  DOCKER_VOL_OPTS?=:z
endif

.PHONY: diagrams
diagrams: $(DIAGRAMS)

%.png: %.plantuml
	docker run \
		--rm \
		--volume ${ROOT_DIR}:/workdir$(DOCKER_VOL_OPTS) \
		--user $(shell id -u):$(shell id -g) \
		us.gcr.io/k8s-artifacts-prod/cluster-api/plantuml:1.2019.6 \
		-v /workdir/$(shell echo '$^' | sed -e 's,.*docs/,,g' )
