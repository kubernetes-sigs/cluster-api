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

# This script ensures the kind is installed, and if already present, it is a viable version.

set -o errexit
set -o nounset
set -o pipefail

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

# shellcheck source=./hack/scripts/ensure/ensure-utils.sh
source "$(dirname "${BASH_SOURCE[0]}")/ensure-utils.sh"

GOPATH_BIN="$(go env GOPATH)/bin"
goarch="$(go env GOARCH)"
goos="$(go env GOOS)"

# Note: When updating the MINIMUM_KIND_VERSION new shas MUST be added in `preBuiltMappings` at `test/infrastructure/kind/mapper.go`
MINIMUM_KIND_VERSION=v0.33.0

# Expected sha256 for each pinned version/OS/ARCH combination. Read via indirect
# expansion below, so shellcheck can't see the usage. Update these whenever
# MINIMUM_KIND_VERSION is bumped, using the sha256sum published alongside each
# release binary.
# shellcheck disable=SC2034
KIND_SHA256_linux_amd64="aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d"
# shellcheck disable=SC2034
KIND_SHA256_linux_arm64="20022bee6cfcd5086cb7234d218e3454e6090022f2a8f55d1fa7fcf42c3867a2"
# shellcheck disable=SC2034
KIND_SHA256_darwin_amd64="5a99f26f57246dc9319dd294803313197a0f34d33c525b3ea8b655db5916ece0"
# shellcheck disable=SC2034
KIND_SHA256_darwin_arm64="0c8c7dbe5e23594a198b786c4bc13dacc101fa6196b0cb0b23a1ca44e61f4b4f"


# install_kind downloads and installs the required kind version into GOPATH_BIN.
install_kind() {
  if [ "$goos" == "linux" ] || [ "$goos" == "darwin" ]; then
    echo "Installing kind ${MINIMUM_KIND_VERSION}"
    if ! [ -d "${GOPATH_BIN}" ]; then
      mkdir -p "${GOPATH_BIN}"
    fi
    KIND_SHA256_VAR="KIND_SHA256_${goos}_${goarch}"
    KIND_SHA256="${!KIND_SHA256_VAR:?no known sha256 for kind ${MINIMUM_KIND_VERSION} on ${goos}/${goarch}, add it to $0}"
    download_and_verify "https://github.com/kubernetes-sigs/kind/releases/download/${MINIMUM_KIND_VERSION}/kind-${goos}-${goarch}" "${KIND_SHA256}" "${GOPATH_BIN}/kind"
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
