/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package versioninfo

import (
	"fmt"
	"strings"
)

var (
	// GitBranch is the branch from which this binary was built
	GitBranch string
	// GitReleaseTag is the git tag from which this binary is released
	GitReleaseTag string
	// GitReleaseCommit is the commit corresponding to the GitReleaseTag
	GitReleaseCommit string
	// GitTreeState indicates if the git tree, from which this binary was built, was clean or dirty
	GitTreeState string
	// GitCommit is the git commit at which this binary binary was built
	GitCommit string
	// GitMajor is the major version of the release
	GitMajor string
	// GitMinor is the minor version of the release
	GitMinor string
)

// VersionInfo returns version information for the supplied binary
func VersionInfo(binName string) string {
	var vi strings.Builder
	vi.WriteString(fmt.Sprintf("%s version info:\n", binName))
	vi.WriteString(fmt.Sprintf("GitReleaseTag: %q, Major: %q, Minor: %q, GitRelaseCommit: %q\n", GitReleaseTag, GitMajor, GitMinor, GitReleaseCommit))
	vi.WriteString(fmt.Sprintf("Git Branch: %q\n", GitBranch))
	vi.WriteString(fmt.Sprintf("Git commit: %q\n", GitCommit))
	vi.WriteString(fmt.Sprintf("Git tree state: %q\n", GitTreeState))

	return vi.String()
}
