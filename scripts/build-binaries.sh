#!/usr/bin/env bash
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

set -o errexit
set -o xtrace

VERSION=${VERSION:-}
SHORT=${SHORT:-}

# shellcheck source=/dev/null
source "$(dirname "$0")/../hack/utils.sh"
readonly REPO_PATH=$(get_root_path)
readonly CAPDCTL_BIN=${REPO_PATH}/bin/capdctl
readonly CAPDCTL_SRC=${REPO_PATH}/cmd/capdctl

readonly CAPDMGR_BIN=${REPO_PATH}/bin/manager
readonly CAPDMGR_SRC=${REPO_PATH}/cmd/manager

readonly KIND_TEST_BIN=${REPO_PATH}/bin/kind-test
readonly KIND_TEST_SRC=${REPO_PATH}/cmd/kind-test

# this sets VERSION and SHORT if they are not already set
source "${REPO_PATH}/hack/set-workspace-status.sh"

LDFLAGS="-s -w \
-X sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo.version=${VERSION} \
-X sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo.shortHash=${SHORT}"

# build capdctl
go build -ldflags "${LDFLAGS}" -o ${CAPDCTL_BIN} ${CAPDCTL_SRC}
# build manager
go build -ldflags "${LDFLAGS}" -o ${CAPDMGR_BIN} ${CAPDMGR_SRC}
# build kind-test
go build -ldflags "${LDFLAGS}" -o ${KIND_TEST_BIN} ${KIND_TEST_SRC}
