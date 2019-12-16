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

set -o nounset
set -o pipefail

# shellcheck source=./hack/utils.sh
source "$(git rev-parse --show-toplevel)/hack/utils.sh"
# cd to the root path
cd_capd_root_path

# cleanup on exit
cleanup() {
  echo "Cleaning up..."
  mv go.mod.old go.mod
  mv go.sum.old go.sum
}
trap cleanup EXIT

echo "Verifying..."
# temporary copy the go mod and sum files
cp go.mod go.mod.old || exit
cp go.sum go.sum.old || exit

# run update-deps.sh
export GO111MODULE="on"
./hack/update-deps.sh

# compare the old and new files
DIFF0=$(diff -u go.mod go.mod.old)
DIFF1=$(diff -u go.sum go.sum.old)

if [[ -n "${DIFF0}" ]] || [[ -n "${DIFF1}" ]]; then
  echo "${DIFF0}"
  echo "${DIFF1}"
  echo "Check failed. Please run ./hack/update-deps.sh"
  exit 1
fi
