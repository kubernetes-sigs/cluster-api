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

TOOL_BIN=hack/tools/bin
mkdir -p ${TOOL_BIN}

TRIVY="${TOOL_BIN}/trivy/${VERSION}/trivy"

# Downloads trivy scanner
if [ ! -f "$TRIVY" ]; then
  curl -L -o ${TOOL_BIN}/trivy.tar.gz "https://github.com/aquasecurity/trivy/releases/download/v${VERSION}/trivy_${VERSION}_${TRIVY_OS}-${TRIVY_ARCH}.tar.gz"
  mkdir -p "$(dirname "$0")/tools/bin/trivy/${VERSION}"
  tar -xf "${TOOL_BIN}/trivy.tar.gz" -C "${TOOL_BIN}/trivy/${VERSION}" trivy
  chmod +x "${TOOL_BIN}/trivy/${VERSION}/trivy"
  rm "${TOOL_BIN}/trivy.tar.gz"
fi
