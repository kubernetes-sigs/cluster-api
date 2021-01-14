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

# Log an error and exit.
# Args:
#   $1 Message to log with the error
#   $2 The error code to return
log::error_exit() {
  local message="${1}"
  local code="${2}"

  log::error "${message}"
  # {{ if .ControlPlane }}
  log::info "Removing member from cluster status"
  kubeadm reset -f update-cluster-status || true
  log::info "Removing etcd member"
  kubeadm reset -f remove-etcd-member || true
  # {{ end }}
  log::info "Resetting kubeadm"
  kubeadm reset -f || true
  log::error "cluster.x-k8s.io kubeadm bootstrap script $0 exiting with status ${code}"
  exit "${code}"
}

log::success_exit() {
  log::info "cluster.x-k8s.io kubeadm bootstrap script $0 finished"
  exit 0
}

# Log an error but keep going.
log::error() {
  local message="${1}"
  timestamp=$(date --iso-8601=seconds)
  echo "!!! [${timestamp}] ${1}" >&2
  shift
  for message; do
    echo "    ${message}" >&2
  done
}

# Print a status line.  Formatted to show up in a stream of output.
log::info() {
  timestamp=$(date --iso-8601=seconds)
  echo "+++ [${timestamp}] ${1}"
  shift
  for message; do
    echo "    ${message}"
  done
}

check_kubeadm_command() {
  local command="${1}"
  local code="${2}"
  case ${code} in
  "0")
    log::info "kubeadm reported successful execution for ${command}"
    ;;
  "1")
    log::error "kubeadm reported failed action(s) for ${command}"
    ;;
  "2")
    log::error "kubeadm reported preflight check error during ${command}"
    ;;
  "3")
    log::error_exit "kubeadm reported validation error for ${command}" "${code}"
    ;;
  *)
    log::error "kubeadm reported unknown error ${code} for ${command}"
    ;;
  esac
}

function retry-command() {
  n=0
  local kubeadm_return
  until [ $n -ge 5 ]; do
    log::info "running '$*'"
    # shellcheck disable=SC1083
    "$@" --config=/tmp/kubeadm-join-config.yaml {{.KubeadmVerbosity}}
    kubeadm_return=$?
    check_kubeadm_command "'$*'" "${kubeadm_return}"
    if [ ${kubeadm_return} -eq 0 ]; then
      break
    fi
    # We allow preflight errors to pass
    if [ ${kubeadm_return} -eq 2 ]; then
      break
    fi
    n=$((n + 1))
    sleep 15
  done
  if [ ${kubeadm_return} -ne 0 ]; then
    log::error_exit "too many errors, exiting" "${kubeadm_return}"
  fi
}

# {{ if .ControlPlane }}
function try-or-die-command() {
  local kubeadm_return
  log::info "running '$*'"
  # shellcheck disable=SC1083
  "$@" --config=/tmp/kubeadm-join-config.yaml {{.KubeadmVerbosity}}
  kubeadm_return=$?
  check_kubeadm_command "'$*'" "${kubeadm_return}"
  if [ ${kubeadm_return} -ne 0 ]; then
    log::error_exit "fatal error, exiting" "${kubeadm_return}"
  fi
}
# {{ end }}

retry-command kubeadm join phase preflight --ignore-preflight-errors=DirAvailable--etc-kubernetes-manifests
# {{ if .ControlPlane }}
retry-command kubeadm join phase control-plane-prepare download-certs
retry-command kubeadm join phase control-plane-prepare certs
retry-command kubeadm join phase control-plane-prepare kubeconfig
retry-command kubeadm join phase control-plane-prepare control-plane
# {{ end }}
retry-command kubeadm join phase kubelet-start
# {{ if .ControlPlane }}
try-or-die-command kubeadm join phase control-plane-join etcd
retry-command kubeadm join phase control-plane-join update-status
retry-command kubeadm join phase control-plane-join mark-control-plane
# {{ end }}

log::success_exit
