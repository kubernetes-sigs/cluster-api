#!/usr/bin/env bash

# Copyright 2023 The Kubernetes Authors.
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

# This script checks import restrictions. The script looks for a file called
# `.import-restrictions` in each directory, then all imports of the package are
# checked against each "rule" in the file.
# Usage: `hack/verify-import-restrictions.sh`.

set -o errexit
set -o nounset
set -o pipefail

sub_packages=(
  "api"
  "exp/api"
  "bootstrap/kubeadm/api"
  "cmd/clusterctl/api"
  "controlplane/kubeadm/api"
  "exp/addons/api"
  "exp/ipam/api"
  "exp/runtime/api"
  "test/infrastructure/docker/api"
  "test/infrastructure/docker/exp/api"
  "test/infrastructure/inmemory/api"
)

packages=()
visit() {
  local count=0
  for file in "$1"/* ; do
    if [ -d "$file" ]; then
      visit "$file"
    elif [ -f "$file" ]; then
      ((count += 1))
    fi
  done
  if [ "$count" -gt 0 ]; then
    # import-boss may not accept directories without any sources
    packages+=("./$1")
  fi
}
for d in "${sub_packages[@]}"; do
  visit "$d"
done

INPUT_DIRS="$(IFS=, ; echo "${packages[*]}")"
echo "Enforcing imports in source codes under the following directories: ${INPUT_DIRS}"

# Make sure GOPATH is unset to avoid behavior inconsistency
# as import-boss will go through the sources
unset GOPATH
import-boss --include-test-files=true --verify-only --input-dirs "${INPUT_DIRS}"
