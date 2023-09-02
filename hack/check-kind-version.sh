#!/usr/bin/env bash

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

if ! command -v kind &> /dev/null; then
    echo "😭 kind is not installed. Get it here: https://kind.sigs.k8s.io/"
    exit 1
fi

# Get the version of kind
VERSION=$(kind version | cut -d ' ' -f2 | tr -d 'v')

# Split the version into major and minor parts
IFS='.' read -ra VER_PARTS <<< "$VERSION"

MIN_VERSION="20"

# Check the version
if [ "${VER_PARTS[0]}" -gt "0" ] || ([ "${VER_PARTS[0]}" -eq "0" ] && [ "${VER_PARTS[1]}" -ge "$MIN_VERSION" ]); then
    echo "👍 kind is version 0.$MIN_VERSION or newer."
else
    echo "😭 kind is older than version 0.$MIN_VERSION"
    exit 1
fi
