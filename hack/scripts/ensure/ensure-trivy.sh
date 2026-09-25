#!/bin/bash

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

# This script downloads and ensures the trivy scanner is installed.

set -o errexit
set -o nounset
set -o pipefail

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

VERSION=${1}

GO_OS="$(go env GOOS)"
if [[ "${GO_OS}" == "linux" ]]; then
  TRIVY_OS="Linux"
elif [[ "${GO_OS}" == "darwin"* ]]; then
  TRIVY_OS="macOS"
fi

GO_ARCH="$(go env GOARCH)"
if [[ "${GO_ARCH}" == "amd" ]]; then
  TRIVY_ARCH="32bit"
elif [[ "${GO_ARCH}" == "amd64"* ]]; then
  TRIVY_ARCH="64bit"
elif [[ "${GO_ARCH}" == "arm" ]]; then
  TRIVY_ARCH="ARM"
elif [[ "${GO_ARCH}" == "arm64" ]]; then
  TRIVY_ARCH="ARM64"
fi

# shellcheck source=./hack/scripts/ensure/ensure-utils.sh
source "$(dirname "$0")/ensure-utils.sh"

TOOL_BIN=hack/tools/bin
mkdir -p ${TOOL_BIN}

TRIVY="${TOOL_BIN}/trivy/${VERSION}/trivy"

# Expected sha256 for each pinned version/OS/ARCH combination. When bumping
# TRIVY_VER in the Makefile, update these from the release's checksums.txt.
# Read via indirect expansion below, so shellcheck can't see the usage.
# shellcheck disable=SC2034
TRIVY_SHA256_Linux_32bit="2c6bf07edc5bdab2d7055a9b4d5dc6ac229476d545b1967288a6a3454d29f31b"
# shellcheck disable=SC2034
TRIVY_SHA256_Linux_64bit="affa59a1e37d86e4b8ab2cd02f0ab2e63d22f1bf9cf6a7aa326c884e25e26ce3"
# shellcheck disable=SC2034
TRIVY_SHA256_Linux_ARM="4f36230491a5724dd13b9e5891728f384e508c34df53c4514acf1877b7bcd281"
# shellcheck disable=SC2034
TRIVY_SHA256_Linux_ARM64="c73b97699c317b0d25532b3f188564b4e29d13d5472ce6f8eb078082546a6481"
# shellcheck disable=SC2034
TRIVY_SHA256_macOS_64bit="41f6eac3ebe3a00448a16f08038b55ce769fe2d5128cb0d64bdf282cdad4831a"
# shellcheck disable=SC2034
TRIVY_SHA256_macOS_ARM64="320c0e6af90b5733b9326da0834240e944c6f44091e50019abdf584237ff4d0c"

# Downloads trivy scanner
if [ ! -f "$TRIVY" ]; then
  TRIVY_SHA256_VAR="TRIVY_SHA256_${TRIVY_OS}_${TRIVY_ARCH}"
  TRIVY_SHA256="${!TRIVY_SHA256_VAR:?no known sha256 for trivy ${VERSION} on ${TRIVY_OS}/${TRIVY_ARCH}, add it to $0}"

  download_and_verify "https://github.com/aquasecurity/trivy/releases/download/v${VERSION}/trivy_${VERSION}_${TRIVY_OS}-${TRIVY_ARCH}.tar.gz" "${TRIVY_SHA256}" "${TOOL_BIN}/trivy.tar.gz"
  mkdir -p "${TOOL_BIN}/trivy/${VERSION}"
  tar -xf "${TOOL_BIN}/trivy.tar.gz" -C "${TOOL_BIN}/trivy/${VERSION}" trivy
  chmod +x "${TOOL_BIN}/trivy/${VERSION}/trivy"
  rm "${TOOL_BIN}/trivy.tar.gz"
fi
