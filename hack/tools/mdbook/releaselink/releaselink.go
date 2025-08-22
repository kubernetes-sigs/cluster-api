//go:build tools
// +build tools

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

// main is the main package for mdbook-releaselink.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/blang/semver/v4"
	"sigs.k8s.io/kubebuilder/docs/book/utils/plugin"

	"sigs.k8s.io/cluster-api/internal/goproxy"
)

// ReleaseLink responds to {{#releaselink <args>}} input. It asks for a `gomodule` parameter
// pointing to a published modules at index.golang.org, it then lists all the versions available
// and resolves it back to the GitHub repository link using the `asset` specified.
// It's possible to add a `version` parameter, which accepts ranges (e.g. >=1.0.0) or wildcards (e.g. >=1.x, v0.1.x)
// to filter the retrieved versions.
// By default pre-releases won't be included unless a `prereleases` parameter is set to `true`.
type ReleaseLink struct{}

// SupportsOutput checks if the given plugin supports the given output format.
func (ReleaseLink) SupportsOutput(_ string) bool { return true }

// Process modifies the book in the input, which gets returned as the result of the plugin.
func (l ReleaseLink) Process(input *plugin.Input) error {
	return plugin.EachCommand(&input.Book, "releaselink", func(_ *plugin.BookChapter, args string) (string, error) {
		var gomodule, asset, repo string
		var found bool

		tags := reflect.StructTag(strings.TrimSpace(args))
		if gomodule, found = tags.Lookup("gomodule"); !found {
			return "", fmt.Errorf("releaselink requires tag \"gomodule\" to be set")
		}
		if asset, found = tags.Lookup("asset"); !found {
			return "", fmt.Errorf("releaselink requires tag \"asset\" to be set")
		}
		if repo, found = tags.Lookup("repo"); !found {
			return "", fmt.Errorf("releaselink requires tag \"repo\" to be set")
		}
		versionRange := semver.MustParseRange(tags.Get("version"))
		includePrereleases := tags.Get("prereleases") == "true"

		scheme, host, err := goproxy.GetSchemeAndHost(os.Getenv("GOPROXY"))
		if err != nil {
			return "", err
		}
		if scheme == "" || host == "" {
			return "", fmt.Errorf("releaselink does not support disabling the go proxy: GOPROXY=%q", os.Getenv("GOPROXY"))
		}

		goproxyClient := goproxy.NewClient(scheme, host)

		parsedTags, err := goproxyClient.GetVersions(context.Background(), gomodule)
		if err != nil {
			return "", err
		}

		var picked semver.Version
		for i, tag := range parsedTags {
			if !includePrereleases && len(tag.Pre) > 0 {
				continue
			}
			if versionRange(tag) {
				picked = parsedTags[i]
			}
		}

		return fmt.Sprintf("%s/releases/download/v%s/%s", repo, picked, asset), nil
	})
}

func main() {
	cfg := ReleaseLink{}
	if err := plugin.Run(cfg, os.Stdin, os.Stdout, os.Args[1:]...); err != nil {
		log.Fatal(err.Error())
	}
}
