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
set -o nounset
set -o pipefail

# shellcheck source=/dev/null
source "$(dirname "$0")/utils.sh"

# set REPO_PATH
REPO_PATH=$(get_root_path)
cd "${REPO_PATH}"

# exit code, if a script fails we'll set this to 1
res=0

# run all verify scripts, optionally skipping any of them

if [[ "${VERIFY_WHITESPACE:-true}" == "true" ]]; then
  echo "[*] Verifying whitespace..."
  hack/verify-whitespace.sh || res=1
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_SPELLING:-true}" == "true" ]]; then
  echo "[*] Verifying spelling..."
  hack/verify-spelling.sh || res=1
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_BOILERPLATE:-true}" == "true" ]]; then
  echo "[*] Verifying boilerplate..."
  hack/verify-boilerplate.sh || res=1
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_GOFMT:-true}" == "true" ]]; then
  echo "[*] Verifying gofmt..."
  hack/verify-gofmt.sh || res=1
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_GOLINT:-true}" == "true" ]]; then
  echo "[*] Verifying golint..."
  hack/verify-golint.sh || res=1
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_GOVET:-true}" == "true" ]]; then
  echo "[*] Verifying govet..."
  hack/verify-govet.sh || res=1
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_DEPS:-true}" == "true" ]]; then
  echo "[*] Verifying deps..."
  hack/verify-deps.sh || res=1
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_GOTEST:-true}" == "true" ]]; then
  echo "[*] Verifying gotest..."
  hack/verify-gotest.sh || res=1
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_BUILD:-true}" == "true" ]]; then
  echo "[*] Verifying build..."
  hack/verify-build.sh || res=1
  cd "${REPO_PATH}"
fi

# exit based on verify scripts
if [[ "${res}" = 0 ]]; then
  echo ""
  echo "All verify checks passed, congrats!"
else
  echo ""
  echo "One or more verify checks failed! See output above..."
fi
exit "${res}"
