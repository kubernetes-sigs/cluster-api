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

# capi:buildDockerImages builds all the CAPI (and CAPD) docker images, if not already present locally.
capi:buildDockerImages () {
  # Configure provider images generation;
  # please ensure the generated image name matches image names used in the E2E_CONF_FILE
  export REGISTRY=gcr.io/k8s-staging-cluster-api
  export TAG=dev
  export ARCH=amd64
  export PULL_POLICY=IfNotPresent

  ## Build all Cluster API provider images, if missing
  if [[ "$(docker images -q $REGISTRY/cluster-api-controller-amd64:$TAG 2> /dev/null)" == "" ]]; then
    echo "+ Building CAPI images"
    make docker-build
  else
    echo "+ CAPI images already present in the system, skipping make"
  fi

  ## Build CAPD provider images, if missing
  if [[ "$(docker images -q $REGISTRY/capd-manager-amd64:$TAG 2> /dev/null)" == "" ]]; then
    echo "+ Building CAPD images"
    make -C test/infrastructure/docker docker-build
  else
    echo "+ CAPD images already present in the system, skipping make"
  fi
}

# k8s::resolveAllVersions checks all the e2e test variables representing a Kubernetes version,
# and resolves kubernetes version labels (e.g. latest) to the corresponding version numbers.
k8s::resolveAllVersions() {
  if [ -n "${KUBERNETES_VERSION:-}" ]; then
    k8s::resolveVersion "KUBERNETES_VERSION" "$KUBERNETES_VERSION"
    export KUBERNETES_VERSION=$resolveVersion
  fi

  if [ -n "${KUBERNETES_VERSION_UPGRADE_TO:-}" ]; then
    k8s::resolveVersion "KUBERNETES_VERSION_UPGRADE_TO" "$KUBERNETES_VERSION_UPGRADE_TO"
    export KUBERNETES_VERSION_UPGRADE_TO=$resolveVersion
  fi

  if [ -n "${KUBERNETES_VERSION_UPGRADE_FROM:-}" ]; then
    k8s::resolveVersion "KUBERNETES_VERSION_UPGRADE_FROM" "$KUBERNETES_VERSION_UPGRADE_FROM"
    export KUBERNETES_VERSION_UPGRADE_FROM=$resolveVersion
  fi

  if [ -n "${BUILD_NODE_IMAGE_TAG:-}" ]; then
    k8s::resolveVersion "BUILD_NODE_IMAGE_TAG" "$BUILD_NODE_IMAGE_TAG"
    export BUILD_NODE_IMAGE_TAG=$resolveVersion
  fi
}

# k8s::resolveVersion resolves kubernetes version labels (e.g. latest) to the corresponding version numbers.
# The result will be available in the resolveVersion variable which is accessible from the caller.
#
# NOTE: this can't be used for kindest/node images pulled from docker hub, given that there are not guarantees that
# such images are generated in sync with the Kubernetes release process.
k8s::resolveVersion() {
  local variableName=$1
  local version=$2

  resolveVersion=$version
  if [[ "$version" =~ ^v ]]; then
    return
  fi

  if [[ "$version" =~ ^ci/ ]]; then
    resolveVersion=$(curl -LsS "http://gcsweb.k8s.io/gcs/kubernetes-release-dev/ci/${version#ci/}.txt")
  else
    resolveVersion=$(curl -LsS "http://gcsweb.k8s.io/gcs/kubernetes-release/release/${version}.txt")
  fi
  echo "+ $variableName=\"$version\" resolved to \"$resolveVersion\""
}

# k8s::setBuildVersion sets the build version that will be applied by the Kubernetes build command.
# the func expect an input parameter defining the version to be used.
k8s::setBuildVersion() {
  local version=$1
  echo "+ Setting version for Kubernetes build to $version"

  local major
  local minor
  major=$(echo "${version#v}" | awk '{split($0,a,"."); print a[1]}')
  minor=$(echo "${version#v}" | awk '{split($0,a,"."); print a[2]}')

  cat > build-version << EOL
export KUBE_GIT_MAJOR=$major
export KUBE_GIT_MINOR=$minor
export KUBE_GIT_VERSION=$version
export KUBE_GIT_TREE_STATE=clean
export KUBE_GIT_COMMIT=d34db33f
EOL

  export KUBE_GIT_VERSION_FILE=$PWD/build-version
}

# kind::buildNodeImage builds a kindest/node images starting from Kubernetes sources.
# the func expect an input parameter defining the image tag to be used.
kind::buildNodeImage() {
  local version=$1
  version="${version//+/_}"

  # return early if the image already exists
  if [[ "$(docker images -q kindest/node:"$version" 2> /dev/null)" != "" ]]; then
    echo "+ image kindest/node:$version already present in the system, skipping build"
    return
  fi

  # sets the build version that will be applied by the Kubernetes build command called during .
  k8s::setBuildVersion "$1"

  # build the node image
  echo "+ Building kindest/node:$version"
  kind build node-image --type docker --image "kindest/node:$version"
}

# kind:prepullImages pre-pull all the images that will be used in the e2e, thus making
# the actual test run less sensible to the network speed.
kind:prepullImages () {
  # Pulling cert manager images so we can pre-load in kind nodes
  kind::prepullImage "quay.io/jetstack/cert-manager-cainjector:v1.1.0"
  kind::prepullImage "quay.io/jetstack/cert-manager-webhook:v1.1.0"
  kind::prepullImage "quay.io/jetstack/cert-manager-controller:v1.1.0"

  # Pulling kindest/node images used by tests
  # NB. some of those versions might be the same
  if [ -n "${KUBERNETES_VERSION:-}" ]; then
    kind::prepullImage "kindest/node:$KUBERNETES_VERSION"
  fi

  if [ -n "${KUBERNETES_VERSION_UPGRADE_TO:-}" ]; then
    kind::prepullImage "kindest/node:$KUBERNETES_VERSION_UPGRADE_TO"
  fi

  if [ -n "${KUBERNETES_VERSION_UPGRADE_FROM:-}" ]; then
    kind::prepullImage "kindest/node:$KUBERNETES_VERSION_UPGRADE_FROM"
  fi

  if [ -n "${BUILD_NODE_IMAGE_TAG:-}" ]; then
    kind::prepullImage "kindest/node:$BUILD_NODE_IMAGE_TAG"
  fi
}

# kind:prepullImage pre-pull a docker image if no already present locally.
kind::prepullImage () {
  local image=$1
  image="${image//+/_}"

  if [[ "$(docker images -q "$image" 2> /dev/null)" == "" ]]; then
    echo "+ Pulling $image"
    docker pull "$image"
  else
    echo "+ image $image already present in the system, skipping pre-pull"
  fi
}