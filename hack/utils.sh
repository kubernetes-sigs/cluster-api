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

# get_root_path returns the root path of the project source tree
get_root_path() {
    git rev-parse --show-toplevel
}

# cd_root_path cds to the root path of the project source tree
cd_root_path() {
    cd "$(get_root_path)" || exit
}

# get_capd_root_path returns the root path of CAPD source tree
get_capd_root_path() {
    echo "$(get_root_path)"/test/infrastructure/docker
}

# cd_capd_root_path cds to the root path of the CAPD source tree
cd_capd_root_path() {
    cd "$(get_capd_root_path)" || exit
}
