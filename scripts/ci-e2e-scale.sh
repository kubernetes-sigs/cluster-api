#!/bin/bash

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

REPO_ROOT=$(git rev-parse --show-toplevel)
cd "${REPO_ROOT}" || exit 1

# shellcheck source=./scripts/ci-e2e-lib.sh
source "${REPO_ROOT}/scripts/ci-e2e-lib.sh"

# shellcheck source=./hack/ensure-go.sh
source "${REPO_ROOT}/hack/ensure-go.sh"
# shellcheck source=./hack/ensure-kubectl.sh
source "${REPO_ROOT}/hack/ensure-kubectl.sh"
# shellcheck source=./hack/ensure-kind.sh
source "${REPO_ROOT}/hack/ensure-kind.sh"

# Make sure the tools binaries are on the path.
export PATH="${REPO_ROOT}/hack/tools/bin:${PATH}"

# Build envsubst. This is later required to template the cloud-init file.
make envsubst

# Builds CAPI (and CAPD) images.
capi:buildDockerImages

# Prepare kindest/node images for all the required Kubernetes version; this implies
# 1. Kubernetes version labels (e.g. latest) to the corresponding version numbers.
# 2. Pre-pulling the corresponding kindest/node image if available; if not, building the image locally.
# Following variables are currently checked (if defined):
# - KUBERNETES_VERSION
# - KUBERNETES_VERSION_UPGRADE_TO
# - KUBERNETES_VERSION_UPGRADE_FROM
k8s::prepareKindestImages
# FIXME(sbueringer): This is only useful if we import them in the remote docker engine
# TBD if we want to use any local docker host or run everything remote

# pre-pull all the images that will be used in the e2e, thus making the actual test run
# less sensible to the network speed. This includes:
# - cert-manager images
kind:prepullAdditionalImages

# RESOURCE_TYPE defines in which cloud we run the e2e test.
# FIXME(sbueringer): as of today boskos.py only supports "gce-project".
# "aws-account" could be supported by extending boskos.py (cf. with boskos.py in the CAPA repo).
export RESOURCE_TYPE="${RESOURCE_TYPE:-"gce-project"}"

# Configure e2e tests
export GINKGO_NODES=3
export GINKGO_NOCOLOR=true
export GINKGO_ARGS="--fail-fast" # Other ginkgo args that need to be appended to the command.
export E2E_CONF_FILE="${REPO_ROOT}/test/e2e/config/docker.yaml"
export ARTIFACTS="${ARTIFACTS:-${REPO_ROOT}/_artifacts}"
export SKIP_RESOURCE_CLEANUP=false
export USE_EXISTING_CLUSTER=false

# Setup local output directory
ARTIFACTS_LOCAL="${ARTIFACTS}/localhost"
mkdir -p "${ARTIFACTS_LOCAL}"
echo "This folder contains logs from the local host where the tests ran." > "${ARTIFACTS_LOCAL}/README.md"

# Configure the containerd socket, otherwise 'ctr' would not work
export CONTAINERD_ADDRESS=/var/run/docker/containerd/containerd.sock

# ensure we retrieve additional info for debugging when we leave the script
cleanup() {
  # shellcheck disable=SC2046
  kill $(pgrep -f 'docker events') || true
  # shellcheck disable=SC2046
  kill $(pgrep -f 'ctr -n moby events') || true

  cp /var/log/docker.log "${ARTIFACTS_LOCAL}/docker.log" || true
  docker ps -a > "${ARTIFACTS_LOCAL}/docker-ps.txt" || true
  docker images > "${ARTIFACTS_LOCAL}/docker-images.txt" || true
  docker info > "${ARTIFACTS_LOCAL}/docker-info.txt" || true
  docker system df > "${ARTIFACTS_LOCAL}/docker-system-df.txt" || true
  docker version > "${ARTIFACTS_LOCAL}/docker-version.txt" || true

  ctr namespaces list > "${ARTIFACTS_LOCAL}/containerd-namespaces.txt" || true
  ctr -n moby tasks list > "${ARTIFACTS_LOCAL}/containerd-tasks.txt" || true
  ctr -n moby containers list > "${ARTIFACTS_LOCAL}/containerd-containers.txt" || true
  ctr -n moby images list > "${ARTIFACTS_LOCAL}/containerd-images.txt" || true
  ctr -n moby version > "${ARTIFACTS_LOCAL}/containerd-version.txt" || true

  # Stop boskos heartbeat
  [[ -z ${HEART_BEAT_PID:-} ]] || kill -9 "${HEART_BEAT_PID}"

  # Stop sshuttle which was used to tunnel to the remote Docker host.
  pkill sshuttle

  # Verify that no containers are running at this time
  # Note: This verifies that all our tests clean up clusters correctly.
  if [[ ! "$(docker ps -q | wc -l)" -eq "0" ]]
  then
     echo "ERROR: Found unexpected running containers:"
     echo ""
     docker ps
     exit 1
  fi
}
trap "cleanup" EXIT SIGINT

# Stream docker and containerd events.
docker events > "${ARTIFACTS_LOCAL}/docker-events.txt" 2>&1 &
ctr -n moby events > "${ARTIFACTS_LOCAL}/containerd-events.txt" 2>&1 &

# Ensure that python3-pip is installed.
apt update
apt install -y python3-pip
rm -rf /var/lib/apt/lists/*

# Install/upgrade pip and requests module explicitly for HTTP calls.
python3 -m pip install --upgrade pip requests

# If BOSKOS_HOST is set then acquire a resource of type ${RESOURCE_TYPE} from Boskos.
if [ -n "${BOSKOS_HOST:-}" ]; then
  # Check out the account from Boskos and store the produced environment
  # variables in a temporary file.
  account_env_var_file="$(mktemp)"
  python3 hack/boskos.py --get --resource-type="${RESOURCE_TYPE}" 1>"${account_env_var_file}"
  checkout_account_status="${?}"

  # If the checkout process was a success then load the account's
  # environment variables into this process.
  # shellcheck disable=SC1090
  [ "${checkout_account_status}" = "0" ] && . "${account_env_var_file}"

  # Always remove the account environment variable file. It contains
  # sensitive information.
  rm -f "${account_env_var_file}"

  if [ ! "${checkout_account_status}" = "0" ]; then
    echo "error getting account from boskos" 1>&2
    exit "${checkout_account_status}"
  fi

  # run the heart beat process to tell boskos that we are still
  # using the checked out account periodically
  python3 -u hack/boskos.py --heartbeat >> "$ARTIFACTS/logs/boskos.log" 2>&1 &
  HEART_BEAT_PID=$!
fi

"hack/remote/setup-docker-on-${RESOURCE_TYPE}.sh"

# Use remote Docker host in CAPD.
export CAPD_DOCKER_HOST=tcp://10.0.3.15:2375

make test-e2e

test_status="${?}"

cleanup

# If Boskos is being used then release the resource back to Boskos.
[ -z "${BOSKOS_HOST:-}" ] || python3 hack/boskos.py --release >> "$ARTIFACTS/logs/boskos.log" 2>&1

exit "${test_status}"
