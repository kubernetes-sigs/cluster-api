#!/bin/bash

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

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
# shellcheck source=../hack/ensure-go.sh
source "${REPO_ROOT}/hack/ensure-go.sh"

MAKE="make"
KIND_VERSION="v0.5.0"
KUBECTL_VERSION="v1.15.3"
KUSTOMIZE_VERSION="3.1.0"
CRD_YAML="crd.yaml"
BOOTSTRAP_CLUSTER_NAME="clusterapi-bootstrap"
CONTROLLER_REPO="controller-ci" # use arbitrary repo name since we don't need to publish it
EXAMPLE_PROVIDER_REPO="example-provider-ci"

GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)

install_kind() {
   wget "https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/kind-${GOOS}-${GOARCH}" \
     --no-verbose -O /usr/local/bin/kind
   chmod +x /usr/local/bin/kind
}

install_kubectl() {
   wget "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/${GOOS}/${GOARCH}/kubectl" \
     --no-verbose -O /usr/local/bin/kubectl
   chmod +x /usr/local/bin/kubectl
}

install_kustomize() {
  wget https://github.com/kubernetes-sigs/kustomize/releases/download/v${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_${GOOS}_${GOARCH} \
    --no-verbose -O /usr/local/bin/kustomize
    chmod +x /usr/local/bin/kustomize
}

build_containers() {
   VERSION="$(git describe --exact-match 2> /dev/null || git describe --match="$(git rev-parse --short=8 HEAD)" --always --dirty --abbrev=8)"
   export CONTROLLER_IMG="${CONTROLLER_REPO}"
   export EXAMPLE_PROVIDER_IMG="${EXAMPLE_PROVIDER_REPO}"

   "${MAKE}" docker-build TAG="${VERSION}" ARCH="${GOARCH}"
   "${MAKE}" docker-build-example-provider TAG="${VERSION}" ARCH="${GOARCH}"
}

prepare_crd_yaml() {
   CLUSTER_API_CONFIG_PATH="./config"
   kustomize build "${CLUSTER_API_CONFIG_PATH}/default/" > "${CRD_YAML}"
   echo "---" >> "${CRD_YAML}"
   kustomize build "${CLUSTER_API_CONFIG_PATH}/ci/" >> "${CRD_YAML}"
}

create_bootstrap() {
   kind create cluster --name "${BOOTSTRAP_CLUSTER_NAME}"
   KUBECONFIG="$(kind get kubeconfig-path --name="${BOOTSTRAP_CLUSTER_NAME}")"
   export KUBECONFIG

   kind load docker-image "${CONTROLLER_IMG}-${GOARCH}:${VERSION}" --name "${BOOTSTRAP_CLUSTER_NAME}"
   kind load docker-image "${EXAMPLE_PROVIDER_IMG}-${GOARCH}:${VERSION}" --name "${BOOTSTRAP_CLUSTER_NAME}"
}

delete_bootstrap() {
   kind delete cluster --name "${BOOTSTRAP_CLUSTER_NAME}"
}

wait_deployment_available() {
   retry=30
   INTERVAL=6
   until kubectl describe deployment "$1" -n "$2" | grep "1 available"
   do
      sleep ${INTERVAL};
      retry=$((retry - 1))
      if [[ $retry -lt 0 ]];
      then
         kubectl describe deployment "$1" -n "$2"
         exit 1
      fi;
   done;
}

wait_pod_running() {
   retry=30
   INTERVAL=6
   until kubectl get pods "$1" -n "$2" --no-headers | awk -F" " '{s+=($3!="Running")} END {exit s}'
   do
      sleep ${INTERVAL};
      retry=$((retry - 1))
      if [[ $retry -lt 0 ]];
      then
         kubectl describe pod "$1" -n "$2"
         kubectl logs "$1" -n "$2"
         exit 1
      fi;
      kubectl get pods "$1" -n "$2" --no-headers;
   done;
}

ensure_docker_in_docker() {
   if [[ -z "${PROW_JOB_ID}" ]] ; then
      # start docker service in setup other than Prow
      service docker start
   fi
   # DinD is configured at Prow
}

main() {
   ensure_docker_in_docker
   build_containers

   install_kubectl
   install_kind
   install_kustomize
   prepare_crd_yaml
   create_bootstrap

   kubectl create -f "${CRD_YAML}"

   set +e
   wait_deployment_available "capi-controller-manager" "capi-system"
   wait_deployment_available "provider-controller-manager" "provider-system"
   set -e

   make test-integration

   delete_bootstrap

   # TODO:
   # create a cluster object, verify from event, cluster reconcile been called.
   # delete a cluster object, verify from event, cluster delete been called.
   # create a machine object, verify from event, machine reconcile been called.
   # delete a machine object, verify from event, machine reconcile been deleted.
}

main
