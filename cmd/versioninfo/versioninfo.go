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
)

const (
	defaultVersion   = "v0.0.0"
	defaultShortHash = "0000000"
)

var (
	// version is the version being released
	version string
	// ShortHash is the short form of the git hash of the commit being built
	shortHash string
)

// VersionInfo returns version information for the supplied binary
func VersionInfo(binName string) string {
	if version == "" {
		version = defaultVersion
	}
	if shortHash == "" {
		shortHash = defaultShortHash
	}
	return fmt.Sprintf("%s %s+%s\n", binName, version, shortHash)
}
