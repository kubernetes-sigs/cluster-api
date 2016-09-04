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

# Utility functions for Kubernetes in docker setup and bootstrap mode

# Start a docker bootstrap for running etcd and flannel
kube::bootstrap::bootstrap_daemon() {

  kube::log::status "Launching docker bootstrap..."

  docker daemon \
    -H ${BOOTSTRAP_DOCKER_SOCK} \
    -p /var/run/docker-bootstrap.pid \
    --iptables=false \
    --ip-masq=false \
    --bridge=none \
    --graph=/var/lib/docker-bootstrap \
    --exec-root=/var/run/docker-bootstrap \
      2> /var/log/docker-bootstrap.log \
      1> /dev/null &

  # Wait for docker bootstrap to start by "docker ps"-ing every second
  local SECONDS=0
  while [[ $(docker -H ${BOOTSTRAP_DOCKER_SOCK} ps 2>&1 1>/dev/null; echo $?) != 0 ]]; do
    ((SECONDS++))
    if [[ ${SECONDS} == ${TIMEOUT_FOR_SERVICES} ]]; then
      kube::log::fatal "docker bootstrap failed to start. Exiting..."
    fi
    sleep 1
  done
}

# Configure docker net settings, then restart it
kube::bootstrap::restart_docker(){

  kube::log::status "Restarting main docker daemon..."

  if kube::helpers::command_exists systemctl; then
    kube::bootstrap::restart_docker_systemd
  elif kube::helpers::command_exists yum; then
    DOCKER_CONF="/etc/sysconfig/docker"
    kube::helpers::backup_file ${DOCKER_CONF}

    # Is there an uncommented OPTIONS line at all?
    if [[ -z $(grep "OPTIONS" ${DOCKER_CONF} | grep -v "#") ]]; then
      echo "OPTIONS=\"--mtu=${FLANNEL_MTU} --bip=${FLANNEL_SUBNET} \"" >> ${DOCKER_CONF}
    else
      kube::helpers::replace_mtu_bip ${DOCKER_CONF} "OPTIONS"
    fi

    kube::multinode::delete_bridge docker0
    systemctl restart docker
  elif kube::helpers::command_exists apt-get; then
    DOCKER_CONF="/etc/default/docker"
    kube::helpers::backup_file ${DOCKER_CONF}
        
    # Is there an uncommented DOCKER_OPTS line at all?
    if [[ -z $(grep "DOCKER_OPTS" $DOCKER_CONF | grep -v "#") ]]; then
      echo "DOCKER_OPTS=\"--mtu=${FLANNEL_MTU} --bip=${FLANNEL_SUBNET} \"" >> ${DOCKER_CONF}
    else
      kube::helpers::replace_mtu_bip ${DOCKER_CONF} "DOCKER_OPTS"
    fi

    kube::multinode::delete_bridge docker0
    service docker stop
    while [[ $(ps aux | grep $(which docker) | grep -v grep | wc -l) -gt 0 ]]; do
      kube::log::status "Waiting for docker to terminate"
      sleep 1
    done
    service docker start
  else
    kube::log::fatal "Error: docker-bootstrap currently only supports ubuntu|debian|amzn|centos|systemd."
  fi

  kube::log::status "Restarted docker with the new flannel settings"
}

# Replace --mtu and --bip in systemd's docker.service file and restart
kube::bootstrap::restart_docker_systemd(){

  DOCKER_CONF=$(systemctl cat docker | head -1 | awk '{print $2}')
  kube::helpers::backup_file ${DOCKER_CONF}
  kube::helpers::replace_mtu_bip ${DOCKER_CONF} $(which docker)

  # The docker0 bridge HAVE TO be deleted in between
  kube::multinode::delete_bridge docker0
  kube::multinode::delete_bridge docker0

  sed -i.bak 's/^\(MountFlags=\).*/\1shared/' ${DOCKER_CONF}
  systemctl daemon-reload
  systemctl daemon-reload
  systemctl restart docker
}

kube::helpers::replace_mtu_bip(){
  local DOCKER_CONF=$1
  local SEARCH_FOR=$2

  # Assuming is a $SEARCH_FOR statement already, and we should append the options if they do not exist
  if [[ -z $(grep -- "--mtu=" $DOCKER_CONF) ]]; then
    sed -e "s@$(grep "$SEARCH_FOR" $DOCKER_CONF)@$(grep "$SEARCH_FOR" $DOCKER_CONF) --mtu=${FLANNEL_MTU}@g" -i $DOCKER_CONF
  fi
  if [[ -z $(grep -- "--bip=" $DOCKER_CONF) ]]; then
    sed -e "s@$(grep "$SEARCH_FOR" $DOCKER_CONF)@$(grep "$SEARCH_FOR" $DOCKER_CONF) --bip=${FLANNEL_SUBNET}@g" -i $DOCKER_CONF
  fi

  # Finds "--mtu=????" and replaces with "--mtu=${FLANNEL_MTU}"
  # Also finds "--bip=??.??.??.??" and replaces with "--bip=${FLANNEL_SUBNET}"
  # NOTE: This method replaces a whole 'mtu' or 'bip' expression. If it ends with a punctuation mark it will be truncated.
  # Please add additional space before the punctuation mark to prevent this. For example: "--mtu=${FLANNEL_MTU} --bip=${FLANNEL_SUBNET} ".
  sed -e "s@$(grep -o -- "--mtu=[[:graph:]]*" $DOCKER_CONF)@--mtu=${FLANNEL_MTU}@g;s@$(grep -o -- "--bip=[[:graph:]]*" $DOCKER_CONF)@--bip=${FLANNEL_SUBNET}@g" -i $DOCKER_CONF
}
