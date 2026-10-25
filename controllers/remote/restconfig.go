/*
Copyright 2021 The Kubernetes Authors.

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

package remote

import (
	"fmt"
	"os"
	"path/filepath"
	gruntime "runtime"
	"strings"

	"sigs.k8s.io/cluster-api/version"
)

const (
	unknowString = "unknown"
)

func buildUserAgent(command, version, sourceName, os, arch, commit string) string {
	return fmt.Sprintf(
		"%s/%s %s (%s/%s) cluster.x-k8s.io/%s", command, version, sourceName, os, arch, commit)
}

// DefaultClusterAPIUserAgent returns a User-Agent string built from static global vars.
func DefaultClusterAPIUserAgent(sourceName string) string {
	return buildUserAgent(
		adjustCommand(os.Args[0]),
		adjustVersion(version.Get().GitVersion),
		adjustSourceName(sourceName),
		gruntime.GOOS,
		gruntime.GOARCH,
		adjustCommit(version.Get().GitCommit))
}

// adjustSourceName returns the name of the source calling the client.
func adjustSourceName(c string) string {
	if c == "" {
		return unknowString
	}
	return c
}

// adjustCommit returns sufficient significant figures of the commit's git hash.
func adjustCommit(c string) string {
	if c == "" {
		return unknowString
	}
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// adjustVersion strips "alpha", "beta", etc. from version in form
// major.minor.patch-[alpha|beta|etc].
func adjustVersion(v string) string {
	if v == "" {
		return unknowString
	}
	seg := strings.SplitN(v, "-", 2)
	return seg[0]
}

// adjustCommand returns the last component of the
// OS-specific command path for use in User-Agent.
func adjustCommand(p string) string {
	// Unlikely, but better than returning "".
	if p == "" {
		return unknowString
	}
	return filepath.Base(p)
}
