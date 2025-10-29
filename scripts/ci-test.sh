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
# temp run on loop to try to catch flake

CAPI_TEST_ENV_LOG_LEVEL=10
while make test-junit; do :; done


#echo -e "\n*** Testing test/infrastructure folder ***\n"
#make test-infrastructure-junit
#
#echo -e "\n*** Testing Cluster API Runtime SDK test extension ***\n"
#make test-test-extension-junit
#
#echo -e "\n*** Testing Cluster API testing framework ***\n"
#make test-framework-junit
