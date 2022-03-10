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

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
ROOT_PATH="$(cd "${SCRIPT_DIR}"/.. && pwd)"

VERSION="0.29.0"

MODE="check"

if [[ "$*" == "fix" ]]; then
  MODE="fix"
fi

if [[ "${OSTYPE}" == "linux"* ]]; then
  BINARY="buildifier"
elif [[ "${OSTYPE}" == "darwin"* ]]; then
  BINARY="buildifier.mac"
fi

# create a temporary directory
TMP_DIR=$(mktemp -d)
OUT="${TMP_DIR}/out.log"

# cleanup on exit
cleanup() {
  ret=0
  if [[ -s "${OUT}" ]]; then
    echo "Found errors:"
    cat "${OUT}"
    echo ""
    echo "run make tiltfile-fix to fix the errors"
    ret=1
  fi
  echo "Cleaning up..."
  rm -rf "${TMP_DIR}"
  exit ${ret}
}
trap cleanup EXIT

BUILDIFIER="${SCRIPT_DIR}/tools/bin/buildifier/${VERSION}/buildifier"

if [ ! -f "$BUILDIFIER" ]; then
  # install buildifier
  cd "${TMP_DIR}" || exit
  curl -L "https://github.com/bazelbuild/buildtools/releases/download/${VERSION}/${BINARY}" -o "${TMP_DIR}/buildifier"
  chmod +x "${TMP_DIR}/buildifier"
  cd "${ROOT_PATH}"
  mkdir -p "$(dirname "$0")/tools/bin/buildifier/${VERSION}"
  mv "${TMP_DIR}/buildifier" "$BUILDIFIER"
fi

echo "Running buildifier..."
cd "${ROOT_PATH}" || exit
"${BUILDIFIER}" -mode=${MODE} Tiltfile >> "${OUT}" 2>&1

