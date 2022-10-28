#!/usr/bin/env bash

# Copyright 2021 The Kubernetes Authors.
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

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

YQ_BIN=yq
YQ_PATH=hack/tools/bin/${YQ_BIN}

cd "${REPO_ROOT}" && make ${YQ_BIN} >/dev/null

KEYS=()
while IFS='' read -r line; do KEYS+=("$line"); done < <(${YQ_PATH} e '.aliases["cluster-api-maintainers"][]' OWNERS_ALIASES)
echo "${KEYS[@]/#/@}"
