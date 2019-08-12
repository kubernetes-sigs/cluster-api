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
# shellcheck source=./ensure-go.sh
source "${KUBE_ROOT}/hack/ensure-go.sh"

GOPROXY=$(go env GOPROXY)
export GOPROXY="${GOPROXY:-https://proxy.golang.org}"
go mod tidy
go mod vendor

# Copy full dependencies if needed.
while IFS= read -r dep; do
    src="$(go mod download -json "${dep}" | jq -r .Dir)"
    dst="${KUBE_ROOT}/vendor/${dep}"
    cp -af "${src}/" "${dst}"
    chmod -R +w "${dst}"
done < "${KUBE_ROOT}/go.vendor"

go mod verify
