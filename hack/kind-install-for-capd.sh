#!/usr/bin/env bash

# Copyright 2025 The Kubernetes Authors.
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


# This script installs a local kind cluster with a local container registry and the correct files mounted for using CAPD
# to test Cluster API.
# This script is a customized version of the kind_with_local_registry script supplied by the kind maintainers at
# https://kind.sigs.k8s.io/docs/user/local-registry/
# The modifications mount the docker socket inside the kind cluster so that CAPD can be used to
# created docker containers.

set -o errexit
set -o nounset
set -o pipefail

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

# See: https://kind.sigs.k8s.io/docs/user/configuration/#ip-family
KIND_NETWORK_IPFAMILY=${CAPI_KIND_NETWORK_IPFAMILY:-"dual"}

KIND_CLUSTER_CONFIG="$(cat <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  ipFamily: ${KIND_NETWORK_IPFAMILY}
nodes:
- role: control-plane
  extraMounts:
    - hostPath: /var/run/docker.sock
      containerPath: /var/run/docker.sock
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
EOF
)"
export KIND_CLUSTER_CONFIG

"$(dirname "${BASH_SOURCE[0]}")/kind-install.sh"
