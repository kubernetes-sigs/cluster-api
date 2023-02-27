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

export ARTIFACTS="${ARTIFACTS:-${REPO_ROOT}/_artifacts}"

# Setup output directory for the docker server.
ARTIFACTS_DOCKER_SERVER="${ARTIFACTS}/docker-server"
mkdir -p "${ARTIFACTS_DOCKER_SERVER}"
echo "This folder contains files for the docker server." > "${ARTIFACTS_DOCKER_SERVER}/README.md"

SSHUTTLE_PIDFILE="${ARTIFACTS_DOCKER_SERVER}/sshuttle.pid"

# retry retries a command $1 times with $2 sleep in between
# Example: retry 10 30 echo test
function retry {
  local attempt=0
  local max_attempts=${1}
  local interval=${2}
  shift; shift
  until [[ "$attempt" -ge "$max_attempts" ]] ; do
    attempt=$((attempt+1))
    set +e
    eval "$*" && return || echo "failed $attempt times: $*"
    set -e
    sleep "$interval"
  done
  echo "error: reached max attempts at retry($*)"
  return 1
}

# get_ssh_cmd calculates the ssh cmd command based on
# SSH_PRIVATE_KEY_FILE and SSH_PUBLIC_KEY_FILE.
# Example: get_ssh_cmd $private_key_file $public_key_file
function get_ssh_cmd {
  local private_key_file=$1 && shift
  local public_key_file=$1 && shift

  local key_file=${private_key_file}
  if [ -z "$key_file" ]; then
    # If there's no private key file use the public key instead
    # This allows us to specify a private key which is held only on a
    # hardware device and therefore has no key file
    key_file=${public_key_file}
  fi

  # Note: LogLevel=ERROR hides warnings.
  echo "ssh -i ${key_file} -l cloud " \
         "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes -o PasswordAuthentication=no -o LogLevel=ERROR "
}

# wait_for_ssh waits until ssh is available on IP $1
# Example: wait_for_ssh $ip $ssh_cmd
function wait_for_ssh_available {
  echo -e "# Wait until ssh is available \n"

  local ip=$1 && shift
  local ssh_cmd=$1 && shift

  retry 10 30 "${ssh_cmd} ${ip} -- true"
  echo ""
}

# start_sshuttle starts sshuttle
# Note: If necessary it also install sshuttle.
# Example: start_sshuttle $public_ip $private_network_cir $server_kind_subnet $ssh_cmd
function start_sshuttle {
  echo -e "# Start sshuttle \n"

  local public_ip=$1 && shift
  local private_network_cir=$1 && shift
  local server_kind_subnet=$1 && shift
  local ssh_cmd=$1 && shift

  # Install sshuttle if it isn't already installed.
  if ! command -v sshuttle > /dev/null;
  then
    echo -e "Install sshuttle\n"
    pip3 install sshuttle
  fi

  # Kill sshuttle if it is already running
  # Note: This depends on ${SSHUTTLE_PIDFILE}.
  stop_sshuttle

  # Wait until ssh is available.
  wait_for_ssh_available "${public_ip}" "${ssh_cmd}"

  # Open tunnel.
  echo "Opening tunnel for CIDRs: ${private_network_cir} and ${server_kind_subnet} (via ${public_ip})"
  # sshuttle won't succeed until ssh is up and python is installed on the destination
  retry 30 20 sshuttle -r "${public_ip}" \
                          "${private_network_cir}" \
                          "${server_kind_subnet}" \
                          --ssh-cmd=\""${ssh_cmd}"\" \
                          -l 0.0.0.0 -D \
                          --pidfile "${SSHUTTLE_PIDFILE}"

  # Give sshuttle a few seconds to be fully up
  sleep 5
  echo ""
}

# stop_sshuttle kills sshuttle
# Note: This depends on ${SSHUTTLE_PIDFILE}.
# Example: stop_sshuttle
function stop_sshuttle {
  echo -e "# Stop sshuttle (if running)\n"

  if [ -f "${SSHUTTLE_PIDFILE}" ]; then
    local sshuttle_pid
    sshuttle_pid=$(cat "${SSHUTTLE_PIDFILE}")
    echo "Stopping sshuttle with PID ${sshuttle_pid}"
    kill "${sshuttle_pid}"
    while [ -d "/proc/${sshuttle_pid}" ]; do
      echo "Waiting for sshuttle to stop"
      sleep 1
    done
  else
    echo "PID file ${SSHUTTLE_PIDFILE} does not exist, skip stopping sshuttle"
  fi

  echo ""
}

# wait_for_cloud_init waits until cloud init is completed and retrieve logs.
# Example: wait_for_cloud_init $ip $ssh_cmd
function wait_for_cloud_init {
  echo -e "# Wait for cloud init \n"

  local ip=$1 && shift
  local ssh_cmd=$1 && shift

  # Wait until cloud-final is either failed or active.
  $ssh_cmd "$ip" -- "
  echo 'Waiting for cloud-final to complete\n'
  start=\$(date -u +%s)
  while true; do
    systemctl --quiet is-failed cloud-final && exit 1
    systemctl --quiet is-active cloud-final && exit 0
    echo Waited \$(((\$(date -u +%s)-\$start)/60)) minutes
    echo ""
    sleep 30
  done"

  # Flush the journal to ensure we get the final logs of cloud-final if it died.
  $ssh_cmd "$ip" -- sudo journalctl --flush

  # Capture logs of cloud-init services
  for service in cloud-config cloud-final cloud-init-local cloud-init; do
    echo -e "[${service}] Get logs and check status"
    $ssh_cmd "$ip" -- sudo journalctl -a -b -u "$service" > "${ARTIFACTS_DOCKER_SERVER}/${service}.log"

    # Fail early if any cloud-init service failed
    $ssh_cmd "$ip" -- sudo systemctl status --full "$service" > "${ARTIFACTS_DOCKER_SERVER}/${service}-status.txt" || \
      {
        echo -e "[${service}] failed"
        echo -e "\nStatus:"
        cat "${ARTIFACTS_DOCKER_SERVER}/${service}.log"
        echo -e "\nLogs:"
        cat "${ARTIFACTS_DOCKER_SERVER}/${service}-status.txt"
        exit 1
      }
  done

  echo ""
}

# template_cloud_init_file templates the cloud init file and
# prints the file location.
# Example: template_cloud_init_file $server_private_ip $public_key_file
function template_cloud_init_file {
  local server_private_ip=$1 && shift
  local public_key_file=$1 && shift

  cloud_init_file="${ARTIFACTS_DOCKER_SERVER}/cloud-init.yaml"

  # Ensure cloud init file exists and is empty.
  echo "" > "$cloud_init_file"

  # Render cloud init file.
  # shellcheck disable=SC2016,SC2086,SC2153
  SERVER_PRIVATE_IP="${server_private_ip}" \
  SSH_PUBLIC_KEY="$(cat ${public_key_file})" \
  SERVER_KIND_SUBNET="${SERVER_KIND_SUBNET}" \
  SERVER_KIND_GATEWAY="${SERVER_KIND_GATEWAY}" \
    envsubst '${SERVER_PRIVATE_IP} ${SSH_PUBLIC_KEY} ${SERVER_KIND_SUBNET} ${SERVER_KIND_GATEWAY}' \
      < "./hack/remote/cloud-init.yaml.tpl" >> "$cloud_init_file"

  echo "${cloud_init_file}"
}
