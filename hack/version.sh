#!/usr/bin/env bash
# Copyright 2020 The Kubernetes Authors.
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

version::get_version_vars() {
    # shellcheck disable=SC1083
    GIT_COMMIT="$(git rev-parse HEAD^{commit})"

    if git_status=$(git status --porcelain 2>/dev/null) && [[ -z ${git_status} ]]; then
        GIT_TREE_STATE="clean"
    else
        GIT_TREE_STATE="dirty"
    fi

    # stolen from k8s.io/hack/lib/version.sh
    # Use git describe to find the version based on annotated tags.
    if GIT_VERSION=$(git describe --abbrev=14 --match "v[0-9]*" 2>/dev/null); then
        # This translates the "git describe" to an actual semver.org
        # compatible semantic version that looks something like this:
        #   v1.1.0-alpha.0.6+84c76d1142ea4d
        #
        # TODO: We continue calling this "git version" because so many
        # downstream consumers are expecting it there.
        # shellcheck disable=SC2001
        DASHES_IN_VERSION=$(echo "${GIT_VERSION}" | sed "s/[^-]//g")
        if [[ "${DASHES_IN_VERSION}" == "---" ]] ; then
            # We have distance to subversion (v1.1.0-subversion-1-gCommitHash)
            # shellcheck disable=SC2001
            GIT_VERSION=$(echo "${GIT_VERSION}" | sed "s/-\([0-9]\{1,\}\)-g\([0-9a-f]\{14\}\)$/.\1\-\2/")
        elif [[ "${DASHES_IN_VERSION}" == "--" ]] ; then
            # We have distance to base tag (v1.1.0-1-gCommitHash)
            # shellcheck disable=SC2001
            GIT_VERSION=$(echo "${GIT_VERSION}" | sed "s/-g\([0-9a-f]\{14\}\)$/-\1/")
        fi
        if [[ "${GIT_TREE_STATE}" == "dirty" ]]; then
            # git describe --dirty only considers changes to existing files, but
            # that is problematic since new untracked .go files affect the build,
            # so use our idea of "dirty" from git status instead.
            GIT_VERSION+="-dirty"
        fi


        # Try to match the "git describe" output to a regex to try to extract
        # the "major" and "minor" versions and whether this is the exact tagged
        # version or whether the tree is between two tagged versions.
        if [[ "${GIT_VERSION}" =~ ^v([0-9]+)\.([0-9]+)(\.[0-9]+)?([-].*)?([+].*)?$ ]]; then
            GIT_MAJOR=${BASH_REMATCH[1]}
            GIT_MINOR=${BASH_REMATCH[2]}
        fi

        # If GIT_VERSION is not a valid Semantic Version, then refuse to build.
        if ! [[ "${GIT_VERSION}" =~ ^v([0-9]+)\.([0-9]+)(\.[0-9]+)?(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
            echo "GIT_VERSION should be a valid Semantic Version. Current value: ${GIT_VERSION}"
            echo "Please see more details here: https://semver.org"
            exit 1
        fi
    fi

    GIT_RELEASE_TAG=$(git describe --abbrev=0 --tags)
    GIT_RELEASE_COMMIT=$(git rev-list -n 1  "${GIT_RELEASE_TAG}")
}

# stolen from k8s.io/hack/lib/version.sh and modified
# Prints the value that needs to be passed to the -ldflags parameter of go build
version::ldflags() {
    version::get_version_vars

    local -a ldflags
    function add_ldflag() {
        local key=${1}
        local val=${2}
        ldflags+=(
            "-X 'sigs.k8s.io/cluster-api/version.${key}=${val}'"
        )
    }

    add_ldflag "buildDate" "$(date ${SOURCE_DATE_EPOCH:+"--date=@${SOURCE_DATE_EPOCH}"} -u +'%Y-%m-%dT%H:%M:%SZ')"
    add_ldflag "gitCommit" "${GIT_COMMIT}"
    add_ldflag "gitTreeState" "${GIT_TREE_STATE}"
    add_ldflag "gitMajor" "${GIT_MAJOR}"
    add_ldflag "gitMinor" "${GIT_MINOR}"
    add_ldflag "gitVersion" "${GIT_VERSION}"
    add_ldflag "gitReleaseCommit" "${GIT_RELEASE_COMMIT}"

    # The -ldflags parameter takes a single string, so join the output.
    echo "${ldflags[*]-}"
}

version::ldflags
