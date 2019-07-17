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

# shellcheck source=/dev/null
source "$(dirname "$0")/../hack/utils.sh"
readonly REPO_PATH=$(get_root_path)
readonly CAPDCTL_BIN=${REPO_PATH}/bin/capdctl
readonly CAPDCTL_SRC=${REPO_PATH}/cmd/capdctl

readonly CAPDMGR_BIN=${REPO_PATH}/bin/capd-manager
readonly CAPDMGR_SRC=${REPO_PATH}/cmd/capd-manager

readonly KIND_TEST_BIN=${REPO_PATH}/bin/kind-test
readonly KIND_TEST_SRC=${REPO_PATH}/cmd/kind-test

source "${REPO_PATH}/hack/set-workspace-status.sh"

LDFLAGS="-X sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo.GitBranch=${GIT_BRANCH} \
-X sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo.GitReleaseTag=${GIT_RELEASE_TAG} \
-X sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo.GitReleaseCommit=${GIT_RELEASE_COMMIT} \
-X sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo.GitTreeState=${GIT_TREE_STATE} \
-X sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo.GitCommit=${GIT_COMMIT} \
-X sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo.GitMajor=${GIT_MAJOR} \
-X sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo.GitMinor=${GIT_MINOR}"

# build capdctl
go build -ldflags "${LDFLAGS}" -o ${CAPDCTL_BIN} ${CAPDCTL_SRC}
# build capd-manager
go build -ldflags "${LDFLAGS}" -o ${CAPDMGR_BIN} ${CAPDMGR_SRC}
# build kind-test
go build -ldflags "${LDFLAGS}" -o ${KIND_TEST_BIN} ${KIND_TEST_SRC}
