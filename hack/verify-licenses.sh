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

# This list is from https://github.com/cncf/foundation/blob/main/allowed-third-party-license-policy.md
CNCF_LICENSE_ALLOWLIST=Apache-2.0,MIT,BSD-2-Clause,SD-2-Clause-FreeBSD,BSD-3-Clause,ISC,Python-2.0,PostgreSQL,X11,Zlib

VERSION=${1}

REPO_ROOT=$(git rev-parse --show-toplevel)
"${REPO_ROOT}/hack/ensure-trivy.sh" "${VERSION}"


TRIVY="${REPO_ROOT}/hack/tools/bin/trivy/${VERSION}/trivy"
$TRIVY filesystem . --license-full --ignored-licenses ${CNCF_LICENSE_ALLOWLIST} --scanners license --severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL -f json | \
# Specifically ignore 'github.com/hashicorp/hcl'. This is a known indirect dependency that we should remove where possible.
# This query ensures we only skip github.com/hashicorp/hcl for as long as the license remains MPL-2.0
jq  '.Results[] | select( .Licenses[]?.PkgName == "github.com/hashicorp/hcl" and .Licenses[]?.Name == "MPL-2.0" | not) | if . == {} then .  else error(.) end'



