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

set -o errexit
set -o nounset
set -o pipefail

if [ -z "${1}" ]; then
  echo "must provide module as first parameter"
  exit 1
fi

if [ -z "${2}" ]; then
  echo "must provide binary name as second parameter"
  exit 1
fi

if [ -z "${3}" ]; then
  echo "must provide version as third parameter"
  exit 1
fi

if [ -z "${4}" ]; then
  echo "must provide the replace parameter as third parameter"
  exit 1
fi

if [ -z "${GOBIN}" ]; then
  echo "GOBIN is not set. Must set GOBIN to install the bin in a specified directory."
  exit 1
fi

rm -f "${GOBIN}/${2}"* || true

export GOWORK="off"

ORIGINAL_WORKDIR="$(pwd)"
TMP_MODULE_DIR="${2}.tmp"

# Create TMP_MODULE_DIR to create a go module for building the binary.
# This is required because CAPI's hack/tools is not compatible to `go install`.
rm -r "${TMP_MODULE_DIR}" || true
mkdir -p "${TMP_MODULE_DIR}"
cd "${TMP_MODULE_DIR}"

# Initialize a go module and place a tools.go file for building the binary.
go mod init "tools"

# Set a replace from the original module to the fork
go mod edit -replace "${4}@${3}"

# Create go file which imports the required package and resolve dependencies.
cat << EOF > tools.go
//go:build tools
// +build tools
package tools

import (
  _ "${1}"
)
EOF

go mod tidy

# Build the binary.
go build -tags=tools -o "${GOBIN}/${2}-${3}" "${1}"

# Get back to the original directory and cleanup the temporary directory.
cd "${ORIGINAL_WORKDIR}"
rm -r "${TMP_MODULE_DIR}"

# Link the unversioned name to the versioned binary.
ln -sf "${GOBIN}/${2}-${3}" "${GOBIN}/${2}"
