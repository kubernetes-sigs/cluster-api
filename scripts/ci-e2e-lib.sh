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
  ARCH="$(go env GOARCH)"
  export REGISTRY=gcr.io/k8s-staging-cluster-api
  export TAG=dev
  export ARCH
  export PULL_POLICY=IfNotPresent

  ## Build all Cluster API provider images, if missing
  if [[ "$(docker images -q "$REGISTRY/cluster-api-controller-$ARCH:$TAG" 2> /dev/null)" == "" ]]; then
    echo "+ Building CAPI images"
    make docker-build
  else
    echo "+ CAPI images already present in the system, skipping make"
  fi

  ## Build CAPD provider images, if missing
  if [[ "$(docker images -q "$REGISTRY/capd-manager-$ARCH:$TAG" 2> /dev/null)" == "" ]]; then
    echo "+ Building CAPD images"
    make -C test/infrastructure/docker docker-build
  else
    echo "+ CAPD images already present in the system, skipping make"
  fi
}

# k8s::prepareKindestImages checks all the e2e test variables representing a Kubernetes version,
# and makes sure a corresponding kindest/node image is available locally.
k8s::prepareKindestImages() {
  if [ -n "${KUBERNETES_VERSION_MANAGEMENT:-}" ]; then
    k8s::resolveVersion "KUBERNETES_VERSION_MANAGEMENT" "$KUBERNETES_VERSION_MANAGEMENT"
    export KUBERNETES_VERSION_MANAGEMENT=$resolveVersion

    kind::prepareKindestImage "$resolveVersion"
  fi

  if [ -n "${KUBERNETES_VERSION:-}" ]; then
    k8s::resolveVersion "KUBERNETES_VERSION" "$KUBERNETES_VERSION"
    export KUBERNETES_VERSION=$resolveVersion

    kind::prepareKindestImage "$resolveVersion"
  fi

  if [ -n "${KUBERNETES_VERSION_UPGRADE_TO:-}" ]; then
    k8s::resolveVersion "KUBERNETES_VERSION_UPGRADE_TO" "$KUBERNETES_VERSION_UPGRADE_TO"
    export KUBERNETES_VERSION_UPGRADE_TO=$resolveVersion

    kind::prepareKindestImage "$resolveVersion"
  fi

  if [ -n "${KUBERNETES_VERSION_UPGRADE_FROM:-}" ]; then
    k8s::resolveVersion "KUBERNETES_VERSION_UPGRADE_FROM" "$KUBERNETES_VERSION_UPGRADE_FROM"
    export KUBERNETES_VERSION_UPGRADE_FROM=$resolveVersion

    kind::prepareKindestImage "$resolveVersion"
  fi

  if [ -n "${BUILD_NODE_IMAGE_TAG:-}" ]; then
    k8s::resolveVersion "BUILD_NODE_IMAGE_TAG" "$BUILD_NODE_IMAGE_TAG"
    export BUILD_NODE_IMAGE_TAG=$resolveVersion

    kind::prepareKindestImage "$resolveVersion"
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
    resolveVersion=$(curl -LsS "http://dl.k8s.io/ci/${version#ci/}.txt")
  else
    resolveVersion=$(curl -LsS "http://dl.k8s.io/release/${version}.txt")
  fi
  echo "+ $variableName=\"$version\" resolved to \"$resolveVersion\""
}

# kind::prepareKindestImage check if a kindest/image exist, and if yes, pre-pull it; otherwise it builds
# the kindest image locally
kind::prepareKindestImage() {
  local version=$1

  # Try to pre-pull the image
  kind::prepullImage "kindest/node:$version"

  # if pre-pull failed, falling back to local build
  if [[ "$retVal" != 0 ]]; then
    echo "+ image for Kuberentes $version is not available in docker hub, trying local build"
    kind::buildNodeImage "$version"
  fi
}

# kind::buildNodeImage builds a kindest/node images starting from Kubernetes sources.
# the func expect an input parameter defining the image tag to be used.
kind::buildNodeImage() {
  local version=$1

  # move to the Kubernetes repository.
  echo "KUBE_ROOT $GOPATH/src/k8s.io/kubernetes"
  cd "$GOPATH/src/k8s.io/kubernetes" || exit

  # checkouts the Kubernetes branch for the given version.
  k8s::checkoutBranch "$version"

  # sets the build version that will be applied by the Kubernetes build command called during kind build node-image.
  k8s::setBuildVersion "$version"

  # build the node image
  version="${version//+/_}"
  echo "+ Building kindest/node:$version"
  kind build node-image --image "kindest/node:$version"

  # move back to Cluster API
  cd "$REPO_ROOT" || exit
}

# k8s::checkoutBranch checkouts the Kubernetes branch for the given version.
k8s::checkoutBranch() {
  local version=$1
  echo "+ Checkout branch for Kubernetes $version"

  # checkout the required tag/branch.
  local buildMetadata
  buildMetadata=$(echo "${version#v}" | awk '{split($0,a,"+"); print a[2]}')
  if [[ "$buildMetadata" == "" ]]; then
    # if there are no release metadata, it means we are looking for a Kubernetes version that
    # should be already been tagged.
    echo "+ checkout tag $version"
    git fetch --all --tags
    git checkout "tags/$version" -B "$version-branch"
  else
    # otherwise we are requiring a Kubernetes version that should be built from HEAD
    # of one of the existing branches
    echo "+ checking for existing branches"
    git fetch --all

    local major
    local minor
    major=$(echo "${version#v}" | awk '{split($0,a,"."); print a[1]}')
    minor=$(echo "${version#v}" | awk '{split($0,a,"."); print a[2]}')

    local releaseBranch
    releaseBranch="$(git branch -r | grep "release-$major.$minor$" || true)"
    if [[ "$releaseBranch" != "" ]]; then
      # if there is already a release branch for the required Kubernetes branch, use it
      echo "+ checkout $releaseBranch branch"
      git checkout "$releaseBranch" -B "release-$major.$minor"
    else
      # otherwise, we should build from the main branch, which is the branch for the next release
      # TODO(sbueringer) we can only change this to main after k/k has migrated to main
      echo "+ checkout master branch"
      git checkout master
    fi
  fi
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

# kind:prepullAdditionalImages pre-pull all the additional (not Kindest/node) images that will be used in the e2e, thus making
# the actual test run less sensible to the network speed.
kind:prepullAdditionalImages () {
  # Pulling cert manager images so we can pre-load in kind nodes
  kind::prepullImage "quay.io/jetstack/cert-manager-cainjector:v1.5.3"
  kind::prepullImage "quay.io/jetstack/cert-manager-webhook:v1.5.3"
  kind::prepullImage "quay.io/jetstack/cert-manager-controller:v1.5.3"
}

# kind:prepullImage pre-pull a docker image if no already present locally.
# The result will be available in the retVal value which is accessible from the caller.
kind::prepullImage () {
  local image=$1
  image="${image//+/_}"

  retVal=0
  if [[ "$(docker images -q "$image" 2> /dev/null)" == "" ]]; then
    echo "+ Pulling $image"
    docker pull "$image" || retVal=$?
  else
    echo "+ image $image already present in the system, skipping pre-pull"
  fi
}
