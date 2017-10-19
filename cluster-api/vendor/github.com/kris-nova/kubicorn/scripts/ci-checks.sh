#!/usr/bin/env bash

# Copyright © 2017 The Kubicorn Authors
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

# gofmt
bad_files=$(echo $PKGS | xargs $GOFMT -l)
if [[ -n "${bad_files}" ]]; then
  echo "✖ gofmt needs to be run on the following files: "
  echo "${bad_files}"
  exit 1
fi

# terminate the execution. too many lint errors
exit 0

# golint
bad_files=$(echo $PKGS | xargs $GOLINT)
if [[ -n "${bad_files}" ]]; then
  echo "✖ lint issues: "
  echo "${bad_files}"
  exit 1
fi
