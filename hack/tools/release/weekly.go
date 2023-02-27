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

// main is the main package for the release notes generator.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

/*
This tool prints all the titles of all PRs from previous release to HEAD.
This needs to be run *before* a tag is created.

Use these as the base of your release notes.
*/

const (
	features      = "Feature additions :sparkles:"
	bugs          = "Bug Fixes :bug: "
	documentation = "Documentation :book: "
	proposals     = "Proposals :memo: "
	warning       = "Breaking Changes :warning: "
	other         = "Others :seedling: "
	unknown       = "Sort these by hand :question: "
)

var (
	outputOrder = []string{
		proposals,
		warning,
		features,
		bugs,
		other,
		documentation,
		unknown,
	}

	from = flag.String("from", "", "Include commits starting from and including this date. Accepts format: YYYY-MM-DD")
	to   = flag.String("to", "", "Include commits up to and including this date. Accepts format: YYYY-MM-DD")

	milestone = flag.String("milestone", "v1.4", "Milestone. Accepts format: v1.4")

	tagRegex = regexp.MustCompile(`^\[release-[\w-\.]*\]`)
)

func main() {
	flag.Parse()
	os.Exit(run())
}

// Since git doesn't include the last day in rev-list we want to increase 1 day to include it in the interval.
func increaseDateByOneDay(date string) (string, error) {
	layout := "2006-01-02"
	datetime, err := time.Parse(layout, date)
	if err != nil {
		return "", err
	}
	datetime = datetime.Add(time.Hour * 24)
	return datetime.Format(layout), nil
}

func run() int {
	var commitRange string
	var cmd *exec.Cmd

	if *from == "" && *to == "" {
		fmt.Println("--from and --to are required together or both unset")
		return 1
	}

	commitRange = fmt.Sprintf("%s to %s", *from, *to)
	lastDay, err := increaseDateByOneDay(*to)
	if err != nil {
		fmt.Println(err)
		return 1
	}

	cmd = exec.Command("git", "rev-list", "HEAD", "--since=\""+*from+" 00:00:01\"", "--until=\""+lastDay+" 23:59:59\"", "--merges", "--pretty=format:%B") //nolint:gosec

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
		fmt.Println(err)
		fmt.Println(string(out))
		return 1
	}

	commits := []*commit{}
	outLines := strings.Split(string(out), "\n")
	for _, line := range outLines {
		line = strings.TrimSpace(line)
		last := len(commits) - 1
		switch {
		case strings.HasPrefix(line, "commit"):
			commits = append(commits, &commit{})
		case strings.HasPrefix(line, "Merge"):
			commits[last].merge = line
			continue
		case line == "":
		default:
			commits[last].body = line
		}
	}

	for _, c := range commits {
		body := trimTitle(c.body)
		var key, prNumber, fork string
		switch {
		case strings.HasPrefix(body, ":sparkles:"), strings.HasPrefix(body, "✨"):
			key = features
			body = strings.TrimPrefix(body, ":sparkles:")
			body = strings.TrimPrefix(body, "✨")
		case strings.HasPrefix(body, ":bug:"), strings.HasPrefix(body, "🐛"):
			key = bugs
			body = strings.TrimPrefix(body, ":bug:")
			body = strings.TrimPrefix(body, "🐛")
		case strings.HasPrefix(body, ":book:"), strings.HasPrefix(body, "📖"):
			key = documentation
			body = strings.TrimPrefix(body, ":book:")
			body = strings.TrimPrefix(body, "📖")
			if strings.Contains(body, "CAEP") || strings.Contains(body, "proposal") {
				key = proposals
			}
		case strings.HasPrefix(body, ":seedling:"), strings.HasPrefix(body, "🌱"):
			key = other
			body = strings.TrimPrefix(body, ":seedling:")
			body = strings.TrimPrefix(body, "🌱")
		case strings.HasPrefix(body, ":warning:"), strings.HasPrefix(body, "⚠️"):
			key = warning
			body = strings.TrimPrefix(body, ":warning:")
			body = strings.TrimPrefix(body, "⚠️")
		default:
			key = unknown
		}

		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		body = fmt.Sprintf("\t - %s", body)
		_, _ = fmt.Sscanf(c.merge, "Merge pull request %s from %s", &prNumber, &fork)
		if key == documentation {
			merges[key] = append(merges[key], prNumber)
			continue
		}
		merges[key] = append(merges[key], formatMerge(body, prNumber))
	}

	// TODO Turn this into a link (requires knowing the project name + organization)
	fmt.Println("Weekly update :rotating_light:")
	fmt.Printf("Changes from %v a total of %d new commits where merged into main.\n\n", commitRange, len(commits))

	for _, key := range outputOrder {
		mergeslice := merges[key]
		if len(mergeslice) == 0 {
			continue
		}

		switch key {
		case documentation:
			fmt.Printf("- %d Documentation and book contributions :book: \n\n", len(mergeslice))
		case other:
			fmt.Printf("- %d Other changes :seedling:\n\n", len(merges[other]))
		default:
			fmt.Printf("- %d %s\n", len(merges[key]), key)
			for _, merge := range mergeslice {
				fmt.Println(merge)
			}
			fmt.Println()
		}
	}

	fmt.Println("All merged PRs can be viewed in GitHub:")
	fmt.Println("https://github.com/kubernetes-sigs/cluster-api/pulls?q=is%3Apr+closed%3A" + *from + ".." + lastDay + "+is%3Amerged+milestone%3A" + *milestone + "+\n")

	fmt.Println("_Thanks to all our contributors!_ 😊")
	fmt.Println("/Your friendly comms release team")

	return 0
}

func trimTitle(title string) string {
	// Remove a tag prefix if found.
	title = tagRegex.ReplaceAllString(title, "")

	return strings.TrimSpace(title)
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
