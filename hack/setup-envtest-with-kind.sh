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

os="unknown"
if [[ "${OSTYPE}" == "linux"* ]]; then
  os="linux"
elif [[ "${OSTYPE}" == "darwin"* ]]; then
  os="darwin"
fi

if [[ "$os" == "unknown" ]]; then
  echo "OS '$OSTYPE' not supported. Aborting." >&2
  exit 1
fi

echo "Execute the following commands to prepare your environment to run the tests with kind:"
echo ""
echo "kind create cluster"
echo "export USE_EXISTING_CLUSTER=true"

if [[ "${os}" == "linux" ]]; then
  echo "gateway_ip=\$(docker inspect kind-control-plane | jq '.[0].NetworkSettings.Networks.kind.Gateway' -r)"
  echo "docker exec kind-control-plane bash -c \"echo \\\"\${gateway_ip} linux.localhost\\\" >> /etc/hosts\""
  echo "export CAPI_WEBHOOK_HOSTNAME=\"linux.localhost\""
elif [[ "${os}" == "darwin" ]]; then
  echo "export CAPI_WEBHOOK_HOSTNAME=\"docker.for.mac.localhost\""
fi

echo ""
echo "Now you can run the tests repeatedly, e.g. via:"
echo ""
echo "go test ./controllers/..."
echo ""
echo "NOTE: You cannot run tests of multiple packages at the same time, because the webhooks would overwrite each other."
