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

MAKE="make"
KUSTOMIZE="kustomize"
KUBECTL="kubectl"
CRD_YAML="crd.yaml"
BOOTSTRAP_CLUSTER_NAME="clusterapi-bootstrap"
CONTROLLER_REPO="controller-ci" # use arbitrary repo name since we don't need to publish it
EXAMPLE_PROVIDER_REPO="example-provider-ci"
INTEGRATION_TEST_DIR="./test/integration"

install_kustomize() {
   go get sigs.k8s.io/kustomize
}

install_kubectl() {
   wget https://storage.googleapis.com/kubernetes-release/release/v1.10.2/bin/linux/amd64/kubectl \
     --no-verbose -O /usr/local/bin/kubectl
   chmod +x /usr/local/bin/kubectl
}

build_containers() {
   VERSION="$(git describe --exact-match 2> /dev/null || git describe --match="$(git rev-parse --short=8 HEAD)" --always --dirty --abbrev=8)"
   CONTROLLER_IMG="${CONTROLLER_REPO}:${VERSION}"
   EXAMPLE_PROVIDER_IMG="${EXAMPLE_PROVIDER_REPO}:${VERSION}"
   export CONTROLLER_IMG="${CONTROLLER_IMG}"
   export EXAMPLE_PROVIDER_IMG="${EXAMPLE_PROVIDER_IMG}" 

   "${MAKE}" docker-build
   "${MAKE}" docker-build-ci
}

prepare_crd_yaml() {
   CLUSTER_API_CONFIG_PATH="./config"
   "${KUSTOMIZE}" build "${CLUSTER_API_CONFIG_PATH}/default/" > "${CRD_YAML}"
   echo "---" >> "${CRD_YAML}"
   "${KUSTOMIZE}" build "${CLUSTER_API_CONFIG_PATH}/ci/" >> "${CRD_YAML}"
}

create_bootstrap() {
   go get sigs.k8s.io/kind
   kind create cluster --name "${BOOTSTRAP_CLUSTER_NAME}"
   KUBECONFIG="$(kind get kubeconfig-path --name="${BOOTSTRAP_CLUSTER_NAME}")"
   export KUBECONFIG

   kind load docker-image "${CONTROLLER_IMG}" --name "${BOOTSTRAP_CLUSTER_NAME}"
   kind load docker-image "${EXAMPLE_PROVIDER_IMG}" --name "${BOOTSTRAP_CLUSTER_NAME}"
}

delete_bootstrap() {
   kind delete cluster --name "${BOOTSTRAP_CLUSTER_NAME}"
}

wait_pod_running() {
   retry=30
   INTERVAL=6
   until kubectl get pods "$1" -n "$2" --no-headers | awk -F" " '{s+=($3!="Running")} END {exit s}'
   do
      sleep ${INTERVAL};
      retry=$((retry - 1))
      if [ $retry -lt 0 ];
      then
         kubectl describe pod "$1" -n "$2"
         kubectl logs "$1" -n "$2"
         exit 1
      fi;
      kubectl get pods "$1" -n "$2" --no-headers;
   done;
}

ensure_docker_in_docker() {
   if [ -z "${PROW_JOB_ID}" ] ; then
      # start docker service in setup other than Prow
      service docker start
   fi
   # DinD is configured at Prow
}

main() {
   ensure_docker_in_docker
   build_containers

   install_kustomize
   prepare_crd_yaml

   install_kubectl
   create_bootstrap

   "${KUBECTL}" create -f "${CRD_YAML}"

   set +e
   wait_pod_running "cluster-api-controller-manager-0" "cluster-api-system"
   wait_pod_running "provider-controller-manager-0" "provider-system"
   set -e

   if [ -d "${INTEGRATION_TEST_DIR}" ] ; then
      go test -v "${INTEGRATION_TEST_DIR}"/...
   fi

   delete_bootstrap

   # TODO:
   # create a cluster object, verify from event, cluster reconcile been called.
   # delete a cluster object, verify from event, cluster delete been called.
   # create a machine object, verify from event, machine reconcile been called.
   # delete a machine object, verify from event, machine reconcile been deleted.
}

main
