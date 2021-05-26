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

VERSION="v0.7.0"

OS="unknown"
if [[ "${OSTYPE}" == "linux"* ]]; then
  OS="linux"
elif [[ "${OSTYPE}" == "darwin"* ]]; then
  OS="darwin"
fi

# shellcheck source=./hack/utils.sh
source "$(dirname "$0")/utils.sh"
ROOT_PATH=$(get_root_path)

# create a temporary directory
TMP_DIR=$(mktemp -d)
OUT="${TMP_DIR}/out.log"

# cleanup on exit
cleanup() {
  ret=0
  if [[ -s "${OUT}" ]]; then
    echo "Found errors:"
    cat "${OUT}"
    ret=1
  fi
  echo "Cleaning up..."
  rm -rf "${TMP_DIR}"
  exit ${ret}
}
trap cleanup EXIT


SHELLCHECK="./$(dirname "$0")/tools/bin/shellcheck/${VERSION}/shellcheck"

if [ ! -f "$SHELLCHECK" ]; then
  # install buildifier
  cd "${TMP_DIR}" || exit
  DOWNLOAD_FILE="shellcheck-${VERSION}.${OS}.x86_64.tar.xz"
  curl -L "https://github.com/koalaman/shellcheck/releases/download/${VERSION}/${DOWNLOAD_FILE}" -o "${TMP_DIR}/shellcheck.tar.xz"
  tar xf "${TMP_DIR}/shellcheck.tar.xz"
  cd "${ROOT_PATH}"
  mkdir -p "$(dirname "$0")/tools/bin/shellcheck/${VERSION}"
  mv "${TMP_DIR}/shellcheck-${VERSION}/shellcheck" "$SHELLCHECK"
fi

echo "Running shellcheck..."
cd "${ROOT_PATH}" || exit
IGNORE_FILES=$(find . -name "*.sh" | grep "third_party\|tilt_modules")
echo "Ignoring shellcheck on ${IGNORE_FILES}"
FILES=$(find . -name "*.sh" -not -path "./tilt_modules/*" -not -path "*third_party*")
while read -r file; do
    "$SHELLCHECK" -x "$file" >> "${OUT}" 2>&1
done <<< "$FILES"
