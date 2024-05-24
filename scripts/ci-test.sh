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

echo "*** Testing Cluster API ***"
make test-junit

echo -e "\n*** Testing Cluster API Provider Docker ***\n"
# Docker provider
make test-docker-infrastructure-junit

echo -e "\n*** Testing Cluster API Provider In-Memory ***\n"
# Docker provider
make test-in-memory-infrastructure-junit

echo -e "\n*** Testing Cluster API Runtime SDK test extension ***\n"
# Test Extension
make test-test-extension-junit
