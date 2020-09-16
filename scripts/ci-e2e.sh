#!/bin/bash

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

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(git rev-parse --show-toplevel)
cd "${REPO_ROOT}" || exit 1

# shellcheck source=./hack/ensure-go.sh
source "${REPO_ROOT}/hack/ensure-go.sh"
# shellcheck source=./hack/ensure-kubectl.sh
source "${REPO_ROOT}/hack/ensure-kubectl.sh"
# shellcheck source=./hack/ensure-kustomize.sh
source "${REPO_ROOT}/hack/ensure-kustomize.sh"

# Configure provider images generation;
# please ensure the generated image name matches image names used in the E2E_CONF_FILE
export REGISTRY=gcr.io/k8s-staging-cluster-api
export TAG=dev
export ARCH=amd64
export PULL_POLICY=IfNotPresent

## Rebuild all Cluster API provider images
make docker-build

## Rebuild CAPD provider images
make -C test/infrastructure/docker docker-build

## Pulling cert manager images so we can pre-load in kind nodes
docker pull quay.io/jetstack/cert-manager-cainjector:v0.16.1
docker pull quay.io/jetstack/cert-manager-webhook:v0.16.1
docker pull quay.io/jetstack/cert-manager-controller:v0.16.1

## Pulling kind images used by tests
docker pull kindest/node:v1.18.2
docker pull kindest/node:v1.17.2

# Configure e2e tests
export GINKGO_NODES=3
export GINKGO_NOCOLOR=true
export GINKGO_ARGS="--failFast" # Other ginkgo args that need to be appended to the command.
export E2E_CONF_FILE="${REPO_ROOT}/test/e2e/config/docker.yaml"
export ARTIFACTS="${ARTIFACTS:-${REPO_ROOT}/_artifacts}"
export SKIP_RESOURCE_CLEANUP=false
export USE_EXISTING_CLUSTER=false

# Run e2e tests
mkdir -p "$ARTIFACTS"
make -C test/e2e/ run
