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

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

# shellcheck source=./hack/utils.sh
source "$(dirname "${BASH_SOURCE[0]}")/utils.sh"

GOPATH_BIN="$(go env GOPATH)/bin"
goarch="$(go env GOARCH)"
goos="$(go env GOOS)"

# Note: When updating the MINIMUM_KIND_VERSION new shas MUST be added in `preBuiltMappings` at `test/infrastructure/kind/mapper.go`
MINIMUM_KIND_VERSION=v0.29.0


# Ensure the kind tool exists and is a viable version, or installs it
verify_kind_version() {

  # If kind is not available on the path, get it
  if ! [ -x "$(command -v kind)" ]; then
    if [ "$goos" == "linux" ] || [ "$goos" == "darwin" ]; then
      echo 'kind not found, installing'
      if ! [ -d "${GOPATH_BIN}" ]; then
        mkdir -p "${GOPATH_BIN}"
      fi
      
      # Download the kind binary with retry logic.
      # Retries up to $max_retries times in case of network or server errors.
      # The '-f' flag in curl ensures it fails on HTTP errors.
      max_retries=5
      retry_count=0
      until curl -sfLo "${GOPATH_BIN}/kind" "https://github.com/kubernetes-sigs/kind/releases/download/${MINIMUM_KIND_VERSION}/kind-${goos}-${goarch}"; do
        retry_count=$((retry_count + 1))
        if [ "${retry_count}" -ge "${max_retries}" ]; then
          echo "Failed to download kind after ${max_retries} attempts."
          return 2
        fi
        echo "Download of kind failed, retrying (${retry_count}/${max_retries})..."
        sleep 3
      done

      if [ ! -s "${GOPATH_BIN}/kind" ]; then
        echo "Downloaded kind binary is missing or empty!"
        return 2
      fi

      chmod +x "${GOPATH_BIN}/kind"
      verify_gopath_bin
      else
        echo "Missing required binary in path: kind"
        return 2
    fi
  fi

  local kind_version
  kind_version="v$(kind version -q)"
  if [[ "${MINIMUM_KIND_VERSION}" != $(echo -e "${MINIMUM_KIND_VERSION}\n${kind_version}" | sort -s -t. -k 1,1n -k 2,2n -k 3,3n | head -n1) ]]; then
    cat <<EOF
Detected kind version: ${kind_version}.
Requires ${MINIMUM_KIND_VERSION} or greater.
Please install ${MINIMUM_KIND_VERSION} or later.
EOF
    return 2
  fi
}

verify_kind_version
