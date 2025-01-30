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


# This script installs a local Kind cluster with a local container registry and the correct files
# mounted for using CAPK to test Cluster API.
# The default Kind CNI is disabled because it doesn't work CAPK. Instead, Calico is used.
# MetalLB is used as a local layer 2 load balancer to support exposing the API servers of workload
# clusters on the local machine that's running this script.

set -o errexit
set -o nounset
set -o pipefail

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

CALICO_VERSION=${CAPI_CALICO_VERSION:-"v3.29.1"}
METALLB_VERSION=${CAPI_METALLB_VERSION:-""}
# Set manually for non-Docker runtimes - example: "172.20"
METALLB_IP_PREFIX=${CAPI_METALLB_IP_PREFIX:-""}
KUBEVIRT_VERSION=${CAPI_KUBEVIRT_VERSION:-""}
# Required to support pulling KubeVirt container disk images from private registries as well as
# avoid Docker Hub rate limiting
KIND_DOCKER_CONFIG_PATH=${CAPI_KIND_DOCKER_CONFIG_PATH:-"$HOME/.docker/config.json"}

# Deploy Kind cluster
KIND_CLUSTER_CONFIG="$(cat <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
nodes:
- role: control-plane
  extraMounts:
  - containerPath: /var/lib/kubelet/config.json
    hostPath: ${KIND_DOCKER_CONFIG_PATH}
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
EOF
)"
export KIND_CLUSTER_CONFIG

"$(dirname "${BASH_SOURCE[0]}")/kind-install.sh"

# Deploy Calico
kubectl apply -f \
  "https://raw.githubusercontent.com/projectcalico/calico/${CALICO_VERSION}/manifests/calico.yaml"

# Deploy MetalLB
if [[ -z "$METALLB_VERSION" ]]; then
  METALLB_VERSION=$(curl "https://api.github.com/repos/metallb/metallb/releases/latest" \
    | jq -r ".tag_name")
fi

kubectl apply -f \
  "https://raw.githubusercontent.com/metallb/metallb/${METALLB_VERSION}/config/manifests/metallb-native.yaml"

echo "Waiting for MetalLB controller pod to be created..."
kubectl wait -n metallb-system deployment controller --for condition=available --timeout 5m
echo "MetalLB controller pod created!"

echo "Waiting for all MetalLB pods to become ready..."
kubectl wait pods -n metallb-system -l app=metallb,component=controller --for condition=Ready --timeout 5m
kubectl wait pods -n metallb-system -l app=metallb,component=speaker --for condition=Ready --timeout 5m
echo "MetalLB pods ready!"

if [[ -z "$METALLB_IP_PREFIX" ]]; then
  SUBNET=$(docker network inspect \
    -f '{{range .IPAM.Config}}{{if .Gateway}}{{.Subnet}}{{end}}{{end}}' kind)
  METALLB_IP_PREFIX=$(echo "$SUBNET" | sed -E 's|^([0-9]+\.[0-9]+)\..*$|\1|g')
fi

cat <<EOF | kubectl apply -f -
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: capi-ip-pool
  namespace: metallb-system
spec:
  addresses:
  - ${METALLB_IP_PREFIX}.255.200-${METALLB_IP_PREFIX}.255.250
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: empty
  namespace: metallb-system
EOF

# Deploy KubeVirt
if [[ -z "$KUBEVIRT_VERSION" ]]; then
  KUBEVIRT_VERSION=$(curl "https://api.github.com/repos/kubevirt/kubevirt/releases/latest" \
    | jq -r ".tag_name")
fi
kubectl apply -f \
  "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-operator.yaml"
kubectl apply -f \
  "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-cr.yaml"
echo "Waiting for KubeVirt to become ready..."
kubectl wait -n kubevirt kv kubevirt --for=condition=Available --timeout=10m
echo "KubeVirt ready!"
