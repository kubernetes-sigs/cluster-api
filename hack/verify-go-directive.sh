#!/usr/bin/env bash

# Copyright 2024 The Kubernetes Authors.
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

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

function usage {
  local script
  script="$(basename "$0")"
  cat >&2 <<EOF
Usage: ${script} [-g <maximum go directive>]
This script should be run at the root of a module.
-g <maximum go directive>
  Compare the go directive in the local working copy's go.mod
  to the specified maximum version it can be. Versions provided
  here are of the form 1.x.y, without the 'go' prefix.
Examples:
  ${script} -g 1.20
  ${script} -g 1.21.6
EOF
  exit 1
}

directory=""
max=""
while getopts g: opt; do
  case "$opt" in
    g) max="$OPTARG";;
    *) usage;;
  esac
done

if [[ -z "${max}" || "${max}" == go* ]]; then
  usage
fi

if [[ -z "${directory}" ]]; then
  directory="."
fi

# Recursive search for go.mod files
find "${directory}" -name "go.mod" -type f -print0 | while IFS= read -r -d '' file; do
  echo "Running go directive verify test for ${file}"
  if ! current=$(awk '$1 == "go" {print $2; exit}' "$file"); then
    echo >&2 "FAIL: could not get value of go directive from ${file}"
    exit 1
  fi

  if ! printf '%s\n' "${current}" "${max}" | sort --check=silent --version-sort; then
    echo >&2 "FAIL: current Go directive ${current} in ${file} is greater than ${max}"
    exit 1
  fi
done