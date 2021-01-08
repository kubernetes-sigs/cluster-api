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

KUBE_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
BIN_ROOT="${KUBE_ROOT}/hack/tools/bin"

kustomize_version=3.9.1

goarch=amd64
goos="unknown"
if [[ "${OSTYPE}" == "linux"* ]]; then
  goos="linux"
elif [[ "${OSTYPE}" == "darwin"* ]]; then
  goos="darwin"
fi

if [[ "$goos" == "unknown" ]]; then
  echo "OS '$OSTYPE' not supported. Aborting." >&2
  exit 1
fi

# Ensure the kustomize tool exists and is a viable version, or installs it
verify_kustomize_version() {
  if ! [ -x "$(command -v "${BIN_ROOT}/kustomize")" ]; then
    echo "fetching kustomize@${kustomize_version}"
    if ! [ -d "${BIN_ROOT}" ]; then
      mkdir -p "${BIN_ROOT}"
    fi
    archive_name="kustomize-v${kustomize_version}.tar.gz"
    curl -sLo "${BIN_ROOT}/${archive_name}" https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${kustomize_version}/kustomize_v${kustomize_version}_${goos}_${goarch}.tar.gz
    tar -zvxf "${BIN_ROOT}/${archive_name}" -C "${BIN_ROOT}/"
    chmod +x "${BIN_ROOT}/kustomize"
    rm "${BIN_ROOT}/${archive_name}"
  fi
}

verify_kustomize_version
