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

# shellcheck source=/dev/null
source "$(dirname "$0")/utils.sh"

# set REPO_PATH
REPO_PATH=$(get_root_path)
cd "${REPO_PATH}"

failure() {
    if [[ "${1}" != 0 ]]; then
        res=1
        failed+=("${2}")
        outputs+=("${3}")
    fi
}

# exit code, if a script fails we'll set this to 1
res=0
failed=()
outputs=()

# run all verify scripts, optionally skipping any of them

if [[ "${VERIFY_MANIFESTS:-true}" == "true" ]]; then
  echo "[*] Verifying manifests..."
  out=$(hack/verify-manifests.sh 2>&1)
  failure $? "verify-manifests.sh" "${out}"
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_BOILERPLATE:-true}" == "true" ]]; then
  echo "[*] Verifying boilerplate..."
  out=$(hack/verify-boilerplate.sh 2>&1)
  failure $? "verify-boilerplate.sh" "${out}"
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_DEPS:-true}" == "true" ]]; then
  echo "[*] Verifying deps..."
  out=$(hack/verify-deps.sh 2>&1)
  failure $? "verify-deps.sh" "${out}"
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_GOLANGCI_LINT:-true}" == "true" ]]; then
  echo "[*] Verifying golangci-lint..."
  out=$(hack/verify-golangci-lint.sh 2>&1)
  failure $? "hack/verify-golangci-lint.sh " "${out}"
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_GOTEST:-true}" == "true" ]]; then
  echo "[*] Verifying gotest..."
  out=$(hack/verify-gotest.sh 2>&1)
  failure $? "verify-gotest.sh" "${out}"
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_BUILD:-true}" == "true" ]]; then
  echo "[*] Verifying build..."
  out=$(hack/verify-build.sh 2>&1)
  failure $? "verify-build.sh" "${out}"
  cd "${REPO_PATH}"
fi

if [[ "${VERIFY_DOCKER_BUILD:-true}" == "true" ]]; then
  echo "[*] Verifying docker build..."
  out=$(hack/verify-docker-build.sh 2>&1)
  failure $? "verify-docker-build.sh" "${out}"
  cd "${REPO_PATH}"
fi

# exit based on verify scripts
if [[ "${res}" = 0 ]]; then
  echo ""
  echo "All verify checks passed, congrats!"
else
  echo ""
  echo "Some of the verify scripts failed:"
  for i in "${!failed[@]}"; do
      echo "- ${failed[$i]}:"
      echo "${outputs[$i]}"
      echo
  done
fi
exit "${res}"
