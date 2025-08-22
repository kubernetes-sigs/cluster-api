#!/usr/bin/env bash

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
set -o pipefail

REPO_ROOT=$(git rev-parse --show-toplevel)
cd "${REPO_ROOT}" || exit 1

# shellcheck source=./scripts/ci-e2e-lib.sh
source "${REPO_ROOT}/scripts/ci-e2e-lib.sh"

# Conf file is parsed to populate variables
export E2E_CONF_FILE="${REPO_ROOT}/test/e2e/config/docker.yaml"

# Prepare kindest/node images for all the required Kubernetes version; this implies
# 1. Kubernetes version labels (e.g. latest) to the corresponding version numbers.
# 2. Pre-pulling the corresponding kindest/node image if available; if not, building the image locally.
# Following variables are currently checked (if defined):
# - KUBERNETES_VERSION
# - KUBERNETES_VERSION_UPGRADE_TO
# - KUBERNETES_VERSION_UPGRADE_FROM
# - KUBERNETES_VERSION_LATEST_CI
# - KUBERNETES_VERSION_MANAGEMENT

k8s::prepareKindestImagesVariables
k8s::prepareKindestImages

# pre-pull all the images that will be used in the e2e, thus making the actual test run
# less sensible to the network speed. This includes:
# - cert-manager images
kind:prepullAdditionalImages
