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

# This script runs shellcheck against all shell scripts in the repo.

set -o errexit
set -o nounset
set -o pipefail

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

if [ $# -ne 1 ]; then
  echo 1>&2 "$0: usage: ./verify-shellcheck.sh <version>"
  exit 2
fi

VERSION=${1}

OS="unknown"
if [[ "${OSTYPE}" == "linux"* ]]; then
  OS="linux"
elif [[ "${OSTYPE}" == "darwin"* ]]; then
  OS="darwin"
fi

# shellcheck source=./hack/scripts/ensure/ensure-utils.sh
source "$(dirname "$0")/../ensure/ensure-utils.sh"
ROOT_PATH=$(get_root_path)

# Expected sha256 for each pinned version/OS combination. When bumping
# SHELLCHECK_VER in the Makefile, update these from the release assets.
# Read via indirect expansion below, so shellcheck can't see the usage.
# shellcheck disable=SC2034
SHELLCHECK_SHA256_linux="6c881ab0698e4e6ea235245f22832860544f17ba386442fe7e9d629f8cbedf87"
# shellcheck disable=SC2034
SHELLCHECK_SHA256_darwin="ef27684f23279d112d8ad84e0823642e43f838993bbb8c0963db9b58a90464c2"

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


SHELLCHECK="${ROOT_PATH}/hack/tools/bin/shellcheck/${VERSION}/shellcheck"

if [ ! -f "$SHELLCHECK" ]; then
  # install shellcheck
  DOWNLOAD_FILE="shellcheck-${VERSION}.${OS}.x86_64.tar.xz"
  SHELLCHECK_SHA256_VAR="SHELLCHECK_SHA256_${OS}"
  SHELLCHECK_SHA256="${!SHELLCHECK_SHA256_VAR:?no known sha256 for shellcheck ${VERSION} on ${OS}, add it to $0}"

  download_and_verify "https://github.com/koalaman/shellcheck/releases/download/${VERSION}/${DOWNLOAD_FILE}" "${SHELLCHECK_SHA256}" "${TMP_DIR}/shellcheck.tar.xz"
  cd "${TMP_DIR}" || exit
  tar xf "${TMP_DIR}/shellcheck.tar.xz"
  cd "${ROOT_PATH}"
  mkdir -p "${ROOT_PATH}/hack/tools/bin/shellcheck/${VERSION}"
  mv "${TMP_DIR}/shellcheck-${VERSION}/shellcheck" "$SHELLCHECK"
fi

echo "Running shellcheck..."
cd "${ROOT_PATH}" || exit
FILES=$(find . -name "*.sh")
while read -r file; do
    "$SHELLCHECK" -x "$file" >> "${OUT}" 2>&1
done <<< "$FILES"
