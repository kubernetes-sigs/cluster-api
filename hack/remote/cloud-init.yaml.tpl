#cloud-config
runcmd:
- /root/setup.sh
final_message: "The system is finally up, after $UPTIME seconds"
users:
- name: cloud
  lock_passwd: true
  sudo: ALL=(ALL) NOPASSWD:ALL
  ssh_authorized_keys:
  - ${SSH_PUBLIC_KEY}
# Infrastructure packages required:
#   python3 - required by sshuttle
#   jq - for convenience
packages:
- python3
- jq
write_files:
- path: /etc/systemd/system/docker.service.d/override.conf
  permissions: 0644
  content: |
    # Disable flags to dockerd, all settings are done in /etc/docker/daemon.json
    [Service]
    ExecStart=
    ExecStart=/usr/bin/dockerd
- path: /etc/docker/daemon.json
  permissions: 0755
  # Note: We had to disable command line flags in the Docker systemd unit with the override file,
  # because otherwise the hosts flag would have been set twice and Docker fails to start up.
  # Because we entirely disable flags in the default Docker systemd unit we also have to set
  # "containerd" in daemon.json
  content: |
    {
      "hosts": ["tcp://${SERVER_PRIVATE_IP}:2375", "unix:///var/run/docker.sock"],
      "tls": false
    }
  # FIXME:(sbueringer) just a test
- path: /usr/local/bin/kind-install-for-capd.sh
  permissions: 0755
  content: |
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


    # This script installs a local kind cluster with a local container registry and the correct files mounted for using CAPD
    # to test Cluster API.
    # This script is a customized version of the kind_with_local_registry script supplied by the kind maintainers at
    # https://kind.sigs.k8s.io/docs/user/local-registry/
    # The modifications mount the docker socket inside the kind cluster so that CAPD can be used to
    # created docker containers.

    set -o errexit
    set -o nounset
    set -o pipefail

    if [[ "${TRACE-0}" == "1" ]]; then
        set -o xtrace
    fi

    KIND_CLUSTER_NAME=${CAPI_KIND_CLUSTER_NAME:-"capi-test"}

    if [[ "$(kind get clusters)" =~ .*"${KIND_CLUSTER_NAME}".* ]]; then
      echo "kind cluster already exists, moving on"
      exit 0
    fi

    # create registry container unless it already exists
    reg_name='kind-registry'
    reg_port='5000'
    running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
    if [ "${running}" != 'true' ]; then
      docker run \
        -d --restart=always -p "10.0.3.15:${reg_port}:5000" --name "${reg_name}" \
        registry:2
    fi

    # create a cluster with the local registry enabled in containerd
    cat <<EOF | kind create cluster --name="$KIND_CLUSTER_NAME"  --config=-
    kind: Cluster
    apiVersion: kind.x-k8s.io/v1alpha4
    networking:
      apiServerAddress: "10.0.3.15"
    nodes:
    - role: control-plane
      extraMounts:
        - hostPath: /var/run/docker.sock
          containerPath: /var/run/docker.sock
    containerdConfigPatches:
    - |-
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
        endpoint = ["http://${reg_name}:${reg_port}"]
    EOF

    # connect the registry to the cluster network
    # (the network may already be connected)
    docker network connect "kind" "${reg_name}" || true

    # Document the local registry
    # https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
    cat <<EOF | kubectl apply -f -
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: local-registry-hosting
      namespace: kube-public
    data:
      localRegistryHosting.v1: |
        host: "10.0.3.15:${reg_port}"
        help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
    EOF
- path: /root/setup.sh
  permissions: 0755
  content: |
    #!/bin/bash

    set -o -x -o errexit -o nounset -o pipefail

    # Install Docker
    curl -fsSL https://get.docker.com | sh

    # Add cloud user to docker group
    # Note: this allows accessing docker via:
    # export DOCKER_HOST=ssh://cloud@3.76.248.105
    sudo usermod -aG docker cloud

    # Ensure we don't get too many open files
    # Check via inotify-consumers (https://github.com/fatso83/dotfiles/blob/master/utils/scripts/inotify-consumers)
    sysctl fs.inotify.max_user_watches=1048576
    sysctl fs.inotify.max_user_instances=8192

    # Create the kind network
    docker network create -d=bridge -o com.docker.network.bridge.enable_ip_masquerade=true -o com.docker.network.driver.mtu=1500 --ipv6 --subnet=fc00:f853:ccd:e793::/64 --subnet=${SERVER_KIND_SUBNET} --gateway=${SERVER_KIND_GATEWAY} kind

    # Install kubectl
    curl -L https://dl.k8s.io/release/v1.26.0/bin/linux/amd64/kubectl -o /tmp/kubectl
    sudo install -o root -g root -m 0755 /tmp/kubectl /usr/local/bin/kubectl
    kubectl version --client -o yaml

    # Install kind
    # Note: The kind binary is not strictly needed but useful for e.g.:
    # kind delete clusters --all
    curl -L https://kind.sigs.k8s.io/dl/v0.17.0/kind-linux-amd64 -o /tmp/kind
    sudo install -o root -g root -m 0755 /tmp/kind /usr/local/bin/kind
    kind version

    # Store config for kind cluster with mounts for CAPD.
    cat <<EOF > /root/kind.yaml
    kind: Cluster
    apiVersion: kind.x-k8s.io/v1alpha4
    networking:
      apiServerAddress: "10.0.3.15"
    nodes:
    - role: control-plane
      extraMounts:
        - hostPath: /var/run/docker.sock
          containerPath: /var/run/docker.sock
    EOF
