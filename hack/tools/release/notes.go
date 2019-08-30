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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

/*
This tool prints all the titles of all PRs from previous release to HEAD.
This needs to be run *before* a tag is created.

Use these as the base of your release notes.
*/

const (
	features      = ":sparkles: New Features"
	bugs          = ":bug: Bug Fixes"
	documentation = ":book: Documentation"
	warning       = ":warning: Breaking Changes"
	other         = ":running: Others"
	unknown       = ":question: Sort these by hand"
)

var (
	outputOrder = []string{
		warning,
		features,
		bugs,
		documentation,
		other,
		unknown,
	}
)

func main() {
	os.Exit(run())
}

func lastTag() string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	out, err := cmd.Output()
	if err != nil {
		return firstCommit()
	}
	return string(bytes.TrimSpace(out))
}

func firstCommit() string {
	cmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "UNKNOWN"
	}
	return string(bytes.TrimSpace(out))
}

func run() int {
	lastTag := lastTag()
	cmd := exec.Command("git", "rev-list", lastTag+"..HEAD", "--merges", "--pretty=format:%B")

	merges := map[string][]string{
		features:      {},
		bugs:          {},
		documentation: {},
		warning:       {},
		other:         {},
		unknown:       {},
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error")
		fmt.Println(string(out))
		return 1
	}
	commits := []commit{}
	outLines := strings.Split(string(out), "\n")
	c := commit{}
	for i := 0; i < len(outLines); i++ {
		line := strings.TrimSpace(outLines[i])
		switch {
		case line == "":
			commits = append(commits, c)
			c = commit{}
		case strings.HasPrefix(line, "Merge"):
			c.merge = line
		case strings.HasPrefix(line, "commit"):
			continue
		default:
			c.body = line
		}
	}
	for _, c := range commits {
		firstWord := strings.Split(c.body, " ")[0]
		var key, prNumber, fork string
		switch {
		case strings.HasPrefix(firstWord, ":sparkles:"), strings.HasPrefix(firstWord, "✨"):
			key = features
		case strings.HasPrefix(firstWord, ":bug:"), strings.HasPrefix(firstWord, "🐛"):
			key = bugs
		case strings.HasPrefix(firstWord, ":book:"), strings.HasPrefix(firstWord, "📖"):
			key = documentation
		case strings.HasPrefix(firstWord, ":running:"), strings.HasPrefix(firstWord, "🏃"):
			key = other
		case strings.HasPrefix(firstWord, ":warning:"), strings.HasPrefix(firstWord, "⚠️"):
			key = warning
		default:
			key = unknown
		}

		if key != unknown {
			c.body = c.body[len(firstWord):]
		}
		c.body = fmt.Sprintf("    - %s", strings.TrimSpace(c.body))
		fmt.Sscanf(c.merge, "Merge pull request %s from %s", &prNumber, &fork)
		merges[key] = append(merges[key], formatMerge(c.body, prNumber))
	}

	// TODO Turn this into a link (requires knowing the project name + organization)
	fmt.Printf("Changes since %v\n---\n", lastTag)

	for _, key := range outputOrder {
		mergeslice := merges[key]
		fmt.Println("## " + key)
		for _, merge := range mergeslice {
			fmt.Println(merge)
		}
		fmt.Println()
	}

	fmt.Println("_Thanks to all our contributors!_ 😊")

	return 0
}

type commit struct {
	merge string
	body  string
}

func formatMerge(line, prNumber string) string {
	if prNumber == "" {
		return line
	}
	return fmt.Sprintf("%s (%s)", line, prNumber)
}
