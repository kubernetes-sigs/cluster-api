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

# set REPO_PATH
readonly REPO_PATH=$(get_capd_root_path)
cd "${REPO_PATH}" || exit

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
if [[ "${VERIFY_BUILD:-true}" == "true" ]]; then
  echo "[*] Verifying build..."
  out=$(hack/verify-build.sh 2>&1)
  failure $? "verify-build.sh" "${out}"
  cd "${REPO_PATH}" || exit
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
