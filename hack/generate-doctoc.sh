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

if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

if [[ -z "$(command -v doctoc)" ]]; then
  echo "doctoc is not available on your system, skipping verification"
  echo "Note: The doctoc module can be installed via npm (https://www.npmjs.com/package/doctoc)"
  exit 0
fi

doctoc ./CONTRIBUTING.md docs/release/release-team-onboarding.md docs/release/release-team.md docs/proposals
