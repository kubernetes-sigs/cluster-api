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
set -x
set -e
set -o pipefail

test() {
	previous_release=$1
	release_branch_for_test=$2
	golden_file=$3
	
	if [ ! -d "$release_branch_for_test" ]; then
		git worktree add "$release_branch_for_test"
	fi

	(cd "$release_branch_for_test" && git pull upstream "$release_branch_for_test")

	NOTES_TEST_FOLDER=$(realpath "$release_branch_for_test")
	export  NOTES_TEST_FOLDER
	export NOTES_TEST_GOLDEN_FILE="$golden_file"
	export NOTES_TEST_PREVIOUS_RELEASE_TAG="$previous_release"
	go test -v -tags tools,integration -run TestReleaseNotes sigs.k8s.io/cluster-api/hack/tools/release

	git worktree remove "$release_branch_for_test"
}

script_dir=$(realpath "$(dirname "$0")")

# This tests a patch release computing the PR list from previous patch tag
# to HEAD. Since v1.3 is out of support, we won't be backporting new PRs
# so the branch should remain untouched and the test valid.
test v1.3.9 release-1.3 "$script_dir/golden/v1.3.10.md"

# The release notes command computes everything from last tag
# to HEAD. Hence if we use the head of release-1.5, this test will
# become invalid everytime we backport some PR to release branch release-1.5.
# Here we cheat a little by poiting to worktree to v1.5.0, which is the
# release that this test is simulating, so it should not exist yet. But
# it represents accurately the HEAD of release-1.5 when we released v1.5.0.
test v1.4.0 v1.5.0 "$script_dir/golden/v1.5.0.md" 
