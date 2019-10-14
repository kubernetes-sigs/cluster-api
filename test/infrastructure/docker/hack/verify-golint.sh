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

# CI script to run go lint over our code
set -o errexit
set -o nounset
set -o pipefail

# shellcheck source=/dev/null
source "$(dirname "$0")/utils.sh"

# cd to the root path
REPO_PATH=$(get_root_path)

# create a temporary directory
TMP_DIR=$(mktemp -d)

# cleanup
exitHandler() (
  echo "Cleaning up..."
  rm -rf "${TMP_DIR}"
)
trap exitHandler EXIT

# pull the source code and build the binary
cd "${TMP_DIR}"
URL="http://github.com/golang/lint"
echo "Cloning ${URL} in ${TMP_DIR}..."
git clone --quiet --depth=1 "${URL}" .
echo "Building golint..."
export GO111MODULE=on
go build -o ./golint/golint ./golint

# run the binary
cd "${REPO_PATH}"
echo "Running golint..."
git ls-files | grep "\.go$" | \
  grep -v "\\/vendor\\/" | \
  xargs -L1 "${TMP_DIR}/golint/golint" -set_exit_status
