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

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
cd "${REPO_ROOT}" || exit 1

# shellcheck source=./hack/ensure-go.sh
source "${REPO_ROOT}/hack/ensure-go.sh"
# shellcheck source=./hack/ensure-kind.sh
source "${REPO_ROOT}/hack/ensure-kind.sh"
# shellcheck source=./hack/ensure-kubectl.sh
source "${REPO_ROOT}/hack/ensure-kubectl.sh"
# shellcheck source=./hack/ensure-kustomize.sh
source "${REPO_ROOT}/hack/ensure-kustomize.sh"

# Set a registry that is not a valid image registry.
# This makes it very clear the images being used on the cluster are built locally.
export REGISTRY=ci-registry
export CORE_IMAGE_NAME=core-manager
export KUBEADM_BOOTSTRAP_IMAGE_NAME=kubeadm-bootstrap-manager
export KUBEADM_CONTROL_PLANE_IMAGE_NAME=kubeadm-control-plane-manager
export DOCKER_MANAGER_IMAGE=docker-provider-manager
export TAG=ci
export ARCH=amd64

# Build the managers from local files.
IMAGE_NAME=${CORE_IMAGE_NAME} make docker-build

ARTIFACTS="${ARTIFACTS:-${PWD}/_artifacts}"
mkdir -p "$ARTIFACTS/logs/"

# Update the images used in the e2e tests.
# This will load the images built above into the kind cluster that runs the managers.
export CAPI_IMAGE=${REGISTRY}/${CORE_IMAGE_NAME}-${ARCH}:${TAG}
export CAPI_KUBEADM_BOOTSTRAP_IMAGE=${REGISTRY}/${KUBEADM_BOOTSTRAP_IMAGE_NAME}-${ARCH}:${TAG}
export CAPI_KUBEADM_CONTROL_PLANE_IMAGE=${REGISTRY}/${KUBEADM_CONTROL_PLANE_IMAGE_NAME}-${ARCH}:${TAG}
export MANAGER_IMAGE=${REGISTRY}/${DOCKER_MANAGER_IMAGE}-${ARCH}:${TAG}

cd "${REPO_ROOT}/test/infrastructure/docker"

# The values above must match the values found in the configuration file below
export E2E_CONF_FILE=e2e/ci-e2e.conf

echo "*** Testing Cluster API Provider Docker e2es ***"
CONTROLLER_IMG=${REGISTRY}/${DOCKER_MANAGER_IMAGE} make docker-build
ARTIFACTS="${ARTIFACTS}" make run-e2e
