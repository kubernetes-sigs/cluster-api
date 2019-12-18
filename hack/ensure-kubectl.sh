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

GOPATH_BIN="$(go env GOPATH)/bin/"
MINIMUM_KUBECTL_VERSION=v1.15.0

# Ensure the kubectl tool exists and is a viable version, or installs it
verify_kubectl_version() {

  # If kubectl is not available on the path, get it
  if ! [ -x "$(command -v kubectl)" ]; then
    if [[ "${OSTYPE}" == "linux-gnu" ]]; then
      if ! [ -d "${GOPATH_BIN}" ]; then
        mkdir -p "${GOPATH_BIN}"
      fi
      echo 'kubectl not found, installing'
      curl -sLo "${GOPATH_BIN}/kubectl" https://storage.googleapis.com/kubernetes-release/release/${MINIMUM_KUBECTL_VERSION}/bin/linux/amd64/kubectl
      chmod +x "${GOPATH_BIN}/kubectl"
    else
      echo "Missing required binary in path: kubectl"
      return 2
    fi
  fi

  local kubectl_version
  IFS=" " read -ra kubectl_version <<< "$(kubectl version --client --short)"
  if [[ "${MINIMUM_KUBECTL_VERSION}" != $(echo -e "${MINIMUM_KUBECTL_VERSION}\n${kubectl_version[2]}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]]; then
    cat <<EOF
Detected kubectl version: ${kubectl_version[2]}.
Requires ${MINIMUM_KUBECTL_VERSION} or greater.
Please install ${MINIMUM_KUBECTL_VERSION} or later.
EOF
    return 2
  fi
}

verify_kubectl_version
