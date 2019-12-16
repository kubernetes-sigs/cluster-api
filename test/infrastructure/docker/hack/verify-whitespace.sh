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

set -o nounset
set -o pipefail

# shellcheck source=./hack/utils.sh
source "$(git rev-parse --show-toplevel)/hack/utils.sh"

cd_capd_root_path

echo "Verifying trailing whitespace..."
TRAILING="$(grep -rnI '[[:blank:]]$' . | grep -v -e .git)"

ERR="0"
if [[ -n "$TRAILING" ]]; then
    echo "Found trailing whitespace in the follow files:"
    echo "${TRAILING}"
    ERR="1"
fi

echo -e "Verifying new lines at end of files..."
FILES="$(git ls-files | grep -I -v -e vendor)"
while read -r LINE; do
    grep -qI . "${LINE}" || continue # skip binary files
    c="$(tail -c 1 "${LINE}")"
    if [[ "$c" != "" ]]; then
        echo "${LINE}: no newline at the end of file"
        ERR=1
    fi
done <<< "${FILES}"

if [[ "$ERR" == "1" ]]; then
    echo "Found whitespace errors!"
    exit 1
fi
