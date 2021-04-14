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

package main

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"strings"

	"sigs.k8s.io/kubebuilder/docs/book/utils/plugin"
)

// Embed.
type Embed struct{}

func (_ Embed) SupportsOutput(_ string) bool { return true }
func (l Embed) Process(input *plugin.Input) error {
	return plugin.EachCommand(&input.Book, "embed-github", func(chapter *plugin.BookChapter, args string) (string, error) {
		tags := reflect.StructTag(strings.TrimSpace(args))

		repository := tags.Get("repo")
		filePath := tags.Get("path")
		branch := tags.Get("branch")
		if branch == "" {
			branch = "master"
		}

		rawURL := url.URL{
			Scheme: "https",
			Host:   "raw.githubusercontent.com",
			Path:   path.Join("/", repository, branch, filePath),
		}

		resp, err := http.Get(rawURL.String())
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		out, err := io.ReadAll(resp.Body)
		return string(out), err
	})
}

func main() {
	cfg := Embed{}
	if err := plugin.Run(cfg, os.Stdin, os.Stdout, os.Args[1:]...); err != nil {
		log.Fatal(err.Error())
	}
}
