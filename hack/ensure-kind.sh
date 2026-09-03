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
MINIMUM_KIND_VERSION=v0.31.0


# install_kind downloads and installs the required kind version into GOPATH_BIN.
install_kind() {
  if [ "$goos" == "linux" ] || [ "$goos" == "darwin" ]; then
    echo "Installing kind ${MINIMUM_KIND_VERSION}"
    if ! [ -d "${GOPATH_BIN}" ]; then
      mkdir -p "${GOPATH_BIN}"
    fi
    curl --retry 5 --retry-all-errors -sLo "${GOPATH_BIN}/kind" "https://github.com/kubernetes-sigs/kind/releases/download/${MINIMUM_KIND_VERSION}/kind-${goos}-${goarch}"
    chmod +x "${GOPATH_BIN}/kind"
    verify_gopath_bin
  else
    echo "Unsupported OS: cannot install kind on ${goos}"
    return 2
  fi
}

# verify_kind_installed checks that the kind binary resolved via PATH meets MINIMUM_KIND_VERSION.
verify_kind_installed() {
  local kind_version
  kind_version="v$(kind version -q)"
  if [[ "${MINIMUM_KIND_VERSION}" != $(echo -e "${MINIMUM_KIND_VERSION}\n${kind_version}" | sort -s -t. -k 1,1n -k 2,2n -k 3,3n | head -n1) ]]; then
    echo "error: 'kind' in PATH resolved to ${kind_version} after install; expected >= ${MINIMUM_KIND_VERSION}"
    return 2
  fi
}

# Ensure the kind tool exists and is a viable version, or installs it
verify_kind_version() {

  # If kind is not available on the path, get it
  if ! [ -x "$(command -v kind)" ]; then
    echo 'kind not found, installing'
    install_kind
    verify_kind_installed
    return
  fi

  local kind_version
  kind_version="v$(kind version -q)"
  if [[ "${MINIMUM_KIND_VERSION}" != $(echo -e "${MINIMUM_KIND_VERSION}\n${kind_version}" | sort -s -t. -k 1,1n -k 2,2n -k 3,3n | head -n1) ]]; then
    cat <<EOF
Detected kind version: ${kind_version}.
Requires ${MINIMUM_KIND_VERSION} or greater.
Installing ${MINIMUM_KIND_VERSION}.
EOF
    install_kind
    verify_kind_installed
  fi
}

verify_kind_version
