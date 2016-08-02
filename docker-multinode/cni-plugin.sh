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

# Utility functions for Kubernetes in docker setup and for cni network plugin.
kube::cni::ensure_docker_settings(){

  if kube::helpers::command_exists systemctl; then
    local restart=false
    DOCKER_CONF=$(systemctl cat docker | head -1 | awk '{print $2}')

    # Clear mtu and bip when previously started in docker-bootstrap mode
    if [[ ! -z $(grep "mtu=" ${DOCKER_CONF}) && ! -z $(grep "bip=" ${DOCKER_CONF}) ]]; then
      sed -i 's/--mtu=.* --bip=.*//g' ${DOCKER_CONF}
      restart=true
      kube::log::status "The mtu and bip parameters removed"
    fi

    # If we can find MountFlags but not MountFlags=shared, set MountFlags to shared
    if [[ ! -z $(grep "MountFlags" ${DOCKER_CONF}) && -z $(grep "MountFlags=shared" ${DOCKER_CONF}) ]]; then

      # Make a dropin file for shared mounts, as /usr/lib isn't always writeable
      mkdir -p /etc/systemd/system/docker.service.d
      cat > /etc/systemd/system/docker.service.d/shared-mounts.conf <<EOF
[Service]
MountFlags=
MountFlags=shared
EOF
      restart=true
      kube::log::status "systemd MountFlags option is now set to shared"
    fi

    # Check if restart needed
    if [[ ${restart} == true ]]; then
      systemctl daemon-reload
      systemctl restart docker
      kube::log::status "Restarted docker with service file modification"
    fi

  fi
}
