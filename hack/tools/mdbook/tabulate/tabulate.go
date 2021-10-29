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

package main

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"sigs.k8s.io/kubebuilder/docs/book/utils/plugin"
)

// Tabulate adds support for switchable tabs in mdbook.
type Tabulate struct{}

// SupportsOutput checks if the given plugin supports the given output format.
func (Tabulate) SupportsOutput(_ string) bool { return true }

// Process modifies the book in the input, which gets returned as the result of the plugin.
func (l Tabulate) Process(input *plugin.Input) error {
	if err := plugin.EachCommand(&input.Book, "tabs", func(chapter *plugin.BookChapter, args string) (string, error) {
		var bld strings.Builder
		tags := reflect.StructTag(strings.TrimSpace(args))
		groupName := tags.Get("name")

		bld.WriteString(`<div id="`)
		bld.WriteString(groupName)
		bld.WriteString(`" class="tabset">`)
		for i, tabName := range strings.Split(tags.Get("tabs"), ",") {
			groupTabID := fmt.Sprintf("%s-%s", groupName, tabName)
			checked := ""
			if i == 0 {
				checked = "checked"
			}

			bld.WriteString(fmt.Sprintf(`<input type="radio" name="%s" id="%s" aria-controls="%s" %s>`, groupName, groupTabID, groupTabID, checked))
			bld.WriteString(fmt.Sprintf(`<label for="%s">%s</label>`, groupTabID, tabName))
		}
		bld.WriteString(`<div class="tab-panels">`)

		return bld.String(), nil
	}); err != nil {
		return err
	}

	if err := plugin.EachCommand(&input.Book, "tab", func(chapter *plugin.BookChapter, name string) (string, error) {
		return fmt.Sprintf(`<section id="tab-%s" class="tab-panel">`, name), nil
	}); err != nil {
		return err
	}

	if err := plugin.EachCommand(&input.Book, "/tab", func(chapter *plugin.BookChapter, args string) (string, error) {
		return "</section>", nil
	}); err != nil {
		return err
	}

	if err := plugin.EachCommand(&input.Book, "/tabs", func(chapter *plugin.BookChapter, args string) (string, error) {
		return "</div></div>", nil
	}); err != nil {
		return err
	}

	return nil
}

func main() {
	cfg := Tabulate{}
	if err := plugin.Run(cfg, os.Stdin, os.Stdout, os.Args[1:]...); err != nil {
		log.Fatal(err.Error())
	}
}
