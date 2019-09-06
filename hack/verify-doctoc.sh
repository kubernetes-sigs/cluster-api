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

command -v doctoc || echo "doctoc is not available on your system, skipping verification" && exit 0

doctoc_files="README.md \
              CONTRIBUTING.md \
              cmd/clusterctl/README.md \
              docs/scope-and-objectives.md \
              docs/staging-use-cases.md \
              docs/developer/releasing.md \
              docs/proposals/20181121-machine-api.md \
              docs/proposals/20190610-machine-states-preboot-bootstrapping.md \
              docs/proposals/YYYYMMDD-template.md"

function check_doctoc(){
    changed_files=""
    for file in $doctoc_files
    do
        res=$(git diff --cached "$file")
        if [ "$res" ]
        then
            changed_files="$changed_files\n$file"
        fi
    done

    if [ "${changed_files}" ];then
        echo -e "Please update these files: $changed_files."
        echo "Update with doctoc FILENAME."
        echo "Re-commit with -n/--no-verify option."
        exit 1
    fi

}

doctoc "${doctoc_files}" >/dev/null 2>&1
check=$(git diff)
if [ -n "$check" ];then
    check_doctoc
fi
