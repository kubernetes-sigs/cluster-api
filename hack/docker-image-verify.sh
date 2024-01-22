#!/usr/bin/env bash
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

_tmp="$(pwd)/_tmp"
cleanup() {
  rm -rf "${_tmp}"
}
trap "cleanup" EXIT SIGINT
cleanup
mkdir -p "${_tmp}"

function TESTIMAGE() {
  IMAGE="${1}"
  ARCH="${2}"
  GREPGOPATH="${3}"
  BINARYPATH="${4:-manager}"

  echo "> Testing $IMAGE"

  docker save "${IMAGE}" -o "${_tmp}"/img.tar
  mkdir -p "${_tmp}"/extracted-img "${_tmp}"/extracted
  tar -xf "${_tmp}"/img.tar -C "${_tmp}"/extracted-img/
  while IFS= read -r -d '' layer
  do
    tar -xf "${layer}" -C "${_tmp}"/extracted
  done <   <(find "${_tmp}"/extracted-img/ -name "*.tar" -print0)

  go version -m "${_tmp}"/extracted/"${BINARYPATH}" | grep -E $'\tpath' | grep -E -q -e "${GREPGOPATH}" || (echo "FAILED ${IMAGE} expected value for path: \"${GREPGOPATH}\""; go version -m "${_tmp}"/extracted/"${BINARYPATH}" | grep -E $'\tpath'; exit 1)
  go version -m "${_tmp}"/extracted/"${BINARYPATH}" | grep -q -E "GOARCH=${ARCH}$" || (echo "Failed ${IMAGE} expected GOARCH to be \"$ARCH\""; go version -m "${_tmp}"/extracted/"${BINARYPATH}" | grep "GOARCH="; exit 1)

  rm -rf "${_tmp}"/img.tar "${_tmp}"/extracted-img "${_tmp}"/extracted
}

for arch in ${ALL_ARCH}; do
  TESTIMAGE "${REGISTRY}/cluster-api-controller-${arch}:${TAG}" "${arch}" "sigs.k8s.io/cluster-api$"
  TESTIMAGE "${REGISTRY}/kubeadm-bootstrap-controller-${arch}:${TAG}" "${arch}" "sigs.k8s.io/cluster-api/bootstrap/kubeadm$"
  TESTIMAGE "${REGISTRY}/kubeadm-control-plane-controller-${arch}:${TAG}" "${arch}" "sigs.k8s.io/cluster-api/controlplane/kubeadm$"
  TESTIMAGE "${REGISTRY}/capd-manager-${arch}:${TAG}" "${arch}" "sigs.k8s.io/cluster-api/test/infrastructure/docker$"
  TESTIMAGE "${REGISTRY}/capim-manager-${arch}:${TAG}" "${arch}" "sigs.k8s.io/cluster-api/test/infrastructure/inmemory$"
  TESTIMAGE "${REGISTRY}/test-extension-${arch}:${TAG}" "${arch}" "sigs.k8s.io/cluster-api/test/extension$"
  TESTIMAGE "${REGISTRY}/clusterctl-${arch}:${TAG}" "${arch}" "sigs.k8s.io/cluster-api/cmd/clusterctl$" "clusterctl"
done
