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

## Please note that this file needs to be escaped for envsubst to function

# shellcheck disable=SC1083,SC2034,SC2066,SC2193

set -o nounset
set -o pipefail
set -o errexit

function retry {
  attempt=0
  max_attempts=$${1}
  interval=$${2}
  shift; shift
  until [[ $${attempt} -ge "$${max_attempts}" ]] ; do
    attempt=$((attempt+1))
    set +e
    eval "$*" && return || echo "failed $${attempt} times: $*"
    set -e
    sleep "$${interval}"
  done
  echo "error: reached max attempts at retry($*)"
  return 1
}

[[ $(id -u) != 0 ]] && SUDO="sudo" || SUDO=""

USE_CI_ARTIFACTS=${USE_CI_ARTIFACTS:=false}

if [ ! "${USE_CI_ARTIFACTS}" = true ]; then
  echo "No CI Artifacts installation, exiting"
  exit 0
fi

# This test installs release packages or binaries that are a result of the CI and release builds.
# It runs '... --version' commands to verify that the binaries are correctly installed
# and finally uninstalls the packages.
# For the release packages it tests all versions in the support skew.
LINE_SEPARATOR="*************************************************"
echo "$${LINE_SEPARATOR}"

## Clusterctl set variables
##
# $${KUBERNETES_VERSION} will be replaced by clusterctl
KUBERNETES_VERSION=${KUBERNETES_VERSION}
##
## End clusterctl set variables

if [[ "$${KUBERNETES_VERSION}" != "" ]]; then
  CI_DIR=/tmp/k8s-ci
  mkdir -p "$${CI_DIR}"
  declare -a PACKAGES_TO_TEST=("kubectl" "kubelet" "kubeadm")
  {{- if .IsControlPlaneMachine }}
  declare -a CONTAINERS_TO_TEST=("kube-apiserver" "kube-controller-manager" "kube-proxy" "kube-scheduler")
  {{- else }}
  declare -a CONTAINERS_TO_TEST=("kube-proxy")
  {{- end }}
  CONTAINER_EXT="tar"
  echo "* testing CI version $${KUBERNETES_VERSION}"
  # Check for semver
  if [[ "$${KUBERNETES_VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    CI_URL="https://dl.k8s.io/release/$${KUBERNETES_VERSION}/bin/linux/amd64"
    VERSION_WITHOUT_PREFIX="$${KUBERNETES_VERSION#v}"
    export DEBIAN_FRONTEND=noninteractive
    # sometimes the network is not immediately available, so we have to retry the apt-get update
    retry 10 5 "apt-get update"
    apt-get install -y apt-transport-https ca-certificates curl
    curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
    echo 'deb https://apt.kubernetes.io/ kubernetes-xenial main' >/etc/apt/sources.list.d/kubernetes.list
    apt-get update
    # replace . with \.
    VERSION_REGEX="$${VERSION_WITHOUT_PREFIX//./\\.}"
    PACKAGE_VERSION="$(apt-cache madison kubelet | grep "$${VERSION_REGEX}-" | head -n1 | cut -d '|' -f 2 | tr -d '[:space:]')"
    for CI_PACKAGE in "$${PACKAGES_TO_TEST[@]}"; do
      echo "* installing package: $${CI_PACKAGE} $${PACKAGE_VERSION}"
      apt-get install -y "$${CI_PACKAGE}=$${PACKAGE_VERSION}"
    done
  else
    CI_URL="https://dl.k8s.io/ci/$${KUBERNETES_VERSION}/bin/linux/amd64"
    for CI_PACKAGE in "$${PACKAGES_TO_TEST[@]}"; do
      # Browser: https://console.cloud.google.com/storage/browser/k8s-release-dev?project=k8s-release-dev
      # e.g.: https://storage.googleapis.com/k8s-release-dev/ci/v1.21.0-beta.1.378+cf3374e43491c5/bin/linux/amd64/kubectl
      echo "* downloading binary: $${CI_URL}/$${CI_PACKAGE}"
      wget "$${CI_URL}/$${CI_PACKAGE}" -O "$${CI_DIR}/$${CI_PACKAGE}"
      chmod +x "$${CI_DIR}/$${CI_PACKAGE}"
      mv "$${CI_DIR}/$${CI_PACKAGE}" "/usr/bin/$${CI_PACKAGE}"
    done
    systemctl restart kubelet
  fi
  for CI_CONTAINER in "$${CONTAINERS_TO_TEST[@]}"; do
    # Redirect: https://dl.k8s.io/release/{path}
    # e.g. https://dl.k8s.io/release/v1.20.4/bin/linux/amd64/kube-apiserver.tar
    # Browser: https://gcsweb.k8s.io/gcs/kubernetes-release/
    # e.g. https://gcsweb.k8s.io/gcs/kubernetes-release/release/v1.20.4/bin/linux/amd64
    echo "* downloading package: $${CI_URL}/$${CI_CONTAINER}.$${CONTAINER_EXT}"
    wget "$${CI_URL}/$${CI_CONTAINER}.$${CONTAINER_EXT}" -O "$${CI_DIR}/$${CI_CONTAINER}.$${CONTAINER_EXT}"
    $${SUDO} ctr -n k8s.io images import "$${CI_DIR}/$${CI_CONTAINER}.$${CONTAINER_EXT}" || echo "* ignoring expected 'ctr images import' result"
    $${SUDO} ctr -n k8s.io images tag "k8s.gcr.io/$${CI_CONTAINER}-amd64:$${KUBERNETES_VERSION//+/_}" "k8s.gcr.io/$${CI_CONTAINER}:$${KUBERNETES_VERSION//+/_}"
    $${SUDO} ctr -n k8s.io images tag "k8s.gcr.io/$${CI_CONTAINER}-amd64:$${KUBERNETES_VERSION//+/_}" "gcr.io/k8s-staging-ci-images/$${CI_CONTAINER}:$${KUBERNETES_VERSION//+/_}"
  done
fi
echo "* checking binary versions"
echo "ctr version: " "$(ctr version)"
echo "kubeadm version: " "$(kubeadm version -o=short)"
echo "kubectl version: " "$(kubectl version --client=true --short=true)"
echo "kubelet version: " "$(kubelet --version)"
echo "$${LINE_SEPARATOR}"
