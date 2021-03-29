#!/usr/bin/env bash

# Copyright 2021 The Kubernetes Authors.
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

# Helper script to tilt up an environment for working with CAPD.

# Make sure we run from the root of the repo
cd "$(git rev-parse --show-toplevel)" || ( \
    echo "!! Unable to find repo root, make sure to run from within cluster-api repo."
    exit 1)

# Verify we have all the expected prerequisites
echo "++ Checking prerequisites..."
PREREQS=( docker kind kustomize tilt envsubst kubectl )
for i in "${PREREQS[@]}"; do
    if [[ -z "$(command -v "$i")" ]]; then
        echo "!! $i was not found on path. Please install and try again."
        exit 1
    fi
done

# Check if we have a usable kind cluster running
echo "++ Validating kind cluster setup..."
if [[ ! $(kubectl cluster-info --context kind-kind) ]]; then
    echo "-- No usable kind cluster found."

    # Let's create one
    export KIND_EXPERIMENTAL_DOCKER_NETWORK=bridge
    kind create cluster --config test/infrastructure/docker/kind-cluster-with-extramounts.yaml

else
    # A kind cluster is running, let's make sure if was started correctly
    if ! (docker inspect kind-control-plane | grep -q "/var/run/docker.sock:/var/run/docker.sock"); then
        echo "!! Kind cluster is running, but does not appear to mount /var/run/docker.sock"
        echo "!! Access to the docker daemon is required for CAPD operation."
        echo "!! Recreate the kind cluster, or you may delete it and this script"
        echo "!! will attempt to create it for you."
        exit 1
    fi
fi

# Check tilt-settings
if [[ -f "tilt-settings.json" ]]; then
    # Settings file exists, make sure it has what we need
    if ! (grep -q docker tilt-settings.json); then
        echo "!! tilt-settings.json file exists in repo root, but does not"
        echo "!! appear to configure CAPD. Please update or remove this file."
        exit 1
    fi
else
    # No settings file present, create one we can use
    cat > tilt-settings.json << EOF
{
  "default_registry": "gcr.io/k8s-staging-cluster-api",
  "enable_providers": ["docker", "kubeadm-bootstrap", "kubeadm-control-plane"]
}
EOF
fi

# Away we go
echo "++"
echo "++ Starting tilt, press Ctrl-C to exit..."
echo "++"
tilt up

