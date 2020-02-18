#!/bin/bash

# Copyright 2020 The Kubernetes Authors.
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

export CAPD_VERSION=v0.3.0

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/../../..
cd "${REPO_ROOT}" || exit 1
REPO_ROOT_ABS=${PWD}
ARTIFACTS="${ARTIFACTS:-${REPO_ROOT_ABS}/_artifacts}"
mkdir -p "$ARTIFACTS/logs/"

if [ -z "${SKIP_DOCKER_BUILD-}" ]; then
	REGISTRY=gcr.io/"$(gcloud config get-value project)"
	export REGISTRY
	export DOCKER_MANAGER_IMAGE=docker-provider-manager
	export TAG=dev
	export ARCH=amd64
	# This will load the capd image into the kind mgmt cluster.
	export MANAGER_IMAGE=${REGISTRY}/${DOCKER_MANAGER_IMAGE}-${ARCH}:${TAG}
	export PULL_POLICY=IfNotPresent
	cd "${REPO_ROOT_ABS}/test/infrastructure/docker"
	CONTROLLER_IMG=${REGISTRY}/${DOCKER_MANAGER_IMAGE} make docker-build
fi

cat <<EOF > "clusterctl-settings.json"
{
   "providers": [ "cluster-api", "kubeadm-bootstrap", "kubeadm-control-plane", "docker"]
}
EOF

# Create a local filesystem repository for the docker provider and update clusterctl.yaml
LOCAL_CAPD_REPO_PATH="${ARTIFACTS}/testdata/docker"
mkdir -p "${LOCAL_CAPD_REPO_PATH}"
cp -r "${REPO_ROOT_ABS}/cmd/clusterctl/test/testdata/docker/${CAPD_VERSION}" "${LOCAL_CAPD_REPO_PATH}"
# We build the infrastructure-components.yaml from the capd folder and put in local repo folder
kustomize build "${REPO_ROOT_ABS}/test/infrastructure/docker/config/default/" > "${LOCAL_CAPD_REPO_PATH}/${CAPD_VERSION}/infrastructure-components.yaml"
export CLUSTERCTL_CONFIG="${ARTIFACTS}/testdata/clusterctl.yaml" 
cat <<EOF > "${CLUSTERCTL_CONFIG}"
providers:
- name: docker
  url:  ${LOCAL_CAPD_REPO_PATH}/${CAPD_VERSION}/infrastructure-components.yaml
  type: InfrastructureProvider

DOCKER_SERVICE_DOMAIN: "cluster.local"
DOCKER_SERVICE_CIDRS: "10.128.0.0/12" 
DOCKER_POD_CIDRS: "192.168.0.0/16" 
EOF

export KIND_CONFIG_FILE="${ARTIFACTS}/kind-cluster-with-extramounts.yaml"
cat <<EOF > "${KIND_CONFIG_FILE}"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraMounts:
      - hostPath: /var/lib/docker
        containerPath: /var/lib/docker
      - hostPath: /var/run/docker.sock
        containerPath: /var/run/docker.sock
EOF

GINKGO_FOCUS=${GINKGO_FOCUS:-""}

cd "${REPO_ROOT_ABS}/cmd/clusterctl/test/e2e"
ARTIFACTS="${ARTIFACTS}" go test -v -tags=e2e -timeout=20m . -args -ginkgo.v -ginkgo.trace --ginkgo.focus="${GINKGO_FOCUS}"
