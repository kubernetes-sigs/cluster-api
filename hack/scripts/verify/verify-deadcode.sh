#!/usr/bin/env bash

# Copyright 2026 The Kubernetes Authors.
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

# Reports functions under internal/** that no entry point (manager binary or any
# test) can reach. Only internal/** is gated as it cannot be imported outside the
# module tree, so unreachable there means dead.
#
# Notes: an ephemeral go.work (temp GOWORK, never touches the tree) spans all
# modules so cross-module usage is seen; -tags=e2e builds the e2e test binary
# whose entry point is build-tagged; *_test.go and */fake/ are excluded as
# intentional test support.

set -o errexit
set -o nounset
set -o pipefail

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

DEADCODE="${1:-deadcode}"

REPO_ROOT=$(git rev-parse --show-toplevel)
cd "${REPO_ROOT}"

# Build an ephemeral workspace spanning all modules in a temp GOWORK file so the
# working tree is never modified (go.work is git-ignored on purpose).
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT
export GOWORK="${WORK_DIR}/go.work"
go work init
go work use "${REPO_ROOT}" "${REPO_ROOT}/test" "${REPO_ROOT}/hack/tools"

echo "Running deadcode on internal/** across all modules..."
findings="$(
  "${DEADCODE}" -test -tags=e2e \
    -filter 'sigs.k8s.io/cluster-api/internal/' \
    sigs.k8s.io/cluster-api/... \
    sigs.k8s.io/cluster-api/test/... |
    grep -v '_test\.go:' |
    grep -v '/fake/' ||
    true
)"

if [[ -n "${findings}" ]]; then
  echo >&2 "FAIL: unreachable (dead) functions found under internal/:"
  echo >&2 ""
  echo >&2 "${findings}"
  echo >&2 ""
  echo >&2 "These functions are reachable from neither the manager binary nor any test."
  echo >&2 "Delete them, or wire them up. If a finding is a false positive (e.g. a build"
  echo >&2 "tag other than e2e hides its only caller), adjust hack/verify-deadcode.sh."
  exit 1
fi

echo "PASS: no dead functions found under internal/."
