#!/bin/bash
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

# Verifies git commits starting from predefined prefix.
# Script is meant to be run in CI. Locally it can produce confusing outputs when local "master" branch is not in sync with remote one

PREFIX="UPSTREAM: <[[:alnum:]]+>: openshift:"

if ! [[ "$(git log -1 --pretty=%B --no-merges)" =~ ^$PREFIX ]]; then
  echo "Last commit message didn't contain needed prefix. Offending commit message is:"
  git log -1 --pretty=%B --no-merges
  exit 1
fi

git log master.. --pretty=%s --no-merges | while read -r; do
  if ! [[ "$REPLY" =~ ^$PREFIX ]]; then
    echo "Git history in this PR doesn't conform to set commit message standards. Offending commit message is:"
    echo "$REPLY"
    exit 1
  fi
done

echo "All looks good"
