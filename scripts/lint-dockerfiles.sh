#!/bin/bash

# Copyright 2022 The Kubernetes Authors.
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

HADOLINT_VER=${1:-latest}
HADOLINT_FAILURE_THRESHOLD=${2:-warning}

FILES=$(find -- * -name Dockerfile)
while read -r file; do
  echo "Linting: ${file}"
  # Configure the linter to fail for warnings and errors. Can be set to: error | warning | info | style | ignore | none
  docker run --rm -i ghcr.io/hadolint/hadolint:"${HADOLINT_VER}" hadolint --failure-threshold "${HADOLINT_FAILURE_THRESHOLD}" - < "${file}"
done <<< "${FILES}"
