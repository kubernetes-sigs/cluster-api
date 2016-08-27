#!/bin/bash

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

# Source common.sh
source $(dirname "${BASH_SOURCE}")/common.sh

# Make sure MASTER_IP is properly set
if [[ -z ${MASTER_IP} ]]; then
    echo "Please export MASTER_IP in your env"
    exit 1
fi

kube::multinode::main

kube::multinode::log_variables

kube::multinode::turndown

if [[ ${USE_CNI} == "true" ]]; then
  kube::cni::ensure_docker_settings

  kube::multinode::start_flannel
else
  kube::bootstrap::bootstrap_daemon

  kube::multinode::start_flannel

  kube::bootstrap::restart_docker
fi

kube::multinode::start_k8s_worker

# If under v1.4.0-alpha.3, run the proxy
if [[ $((VERSION_MINOR < 4)) == 1 || \
      ($((VERSION_MINOR <= 4)) == 1 && \
      ${VERSION_PRERELEASE} == "alpha" && \
      $((VERSION_PRERELEASE_REV < 3)) == 1) ]]; then

	kube::multinode::start_k8s_worker_proxy
fi

kube::log::status "Done. After about a minute the node should be ready."
