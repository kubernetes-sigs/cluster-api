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

set -x

GOPATH_BIN="$(go env GOPATH)/bin/"
MINIMUM_KIND_VERSION=v0.9.0

# Ensure the kind tool exists and is a viable version, or installs it
verify_kind_version() {

  # If kind is not available on the path, get it
  if ! [ -x "$(command -v kind)" ]; then
    if [[ "${OSTYPE}" == "linux-gnu" ]]; then
      echo 'kind not found, installing'
      if ! [ -d "${GOPATH_BIN}" ]; then
        mkdir -p "${GOPATH_BIN}"
      fi
      curl -sLo "${GOPATH_BIN}/kind" https://github.com/kubernetes-sigs/kind/releases/download/${MINIMUM_KIND_VERSION}/kind-linux-amd64
      chmod +x "${GOPATH_BIN}/kind"
    else
      echo "Missing required binary in path: kind"
      return 2
    fi
  fi

  local kind_version
  kind_version=$(kind version)
  if ! [[ "${kind_version}" =~ ${MINIMUM_KIND_VERSION} ]]; then
    cat <<EOF
Detected kind version: ${kind_version}.
Requires ${MINIMUM_KIND_VERSION} or greater.
Please install ${MINIMUM_KIND_VERSION} or later.
EOF
    return 2
  fi
}

verify_kind_version
