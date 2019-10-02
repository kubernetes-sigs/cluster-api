#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
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

CONFIG_DIR=config-capi

# find the module version and the location on the system
src=$(go mod download -json sigs.k8s.io/cluster-api  | jq -r .Dir)

# -p will not fail if the dir exists
mkdir -p "${CONFIG_DIR}"

# -af will keep preserve structure and attributes and overwrite existing files
cp -af "${src}/config/" "${CONFIG_DIR}"

# Add back the write permissions so we can modify files if we want (and overwrite them on next update)
chmod -R +w "${CONFIG_DIR}"


