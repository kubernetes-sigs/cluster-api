#!/bin/bash

# Copyright 2022 The Kubernetes Authors.
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

RESULT=0
SUMMARY=$(cat ./docs/book/src/SUMMARY.md)

pushd ./docs/book/src > /dev/null
FILES=$(find -- * -name "*.md" ! -name "SUMMARY.md")
while read -r file; do
  if ! [[ $SUMMARY == *"${file}"* ]]; then
    RESULT=1
    echo "Didn't find $file in SUMMARY.md"
  fi
done <<< "${FILES}"
popd > /dev/null

exit ${RESULT}