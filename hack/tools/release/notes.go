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
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
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
	proposals     = ":memo: Proposals"
	warning       = ":warning: Breaking Changes"
	other         = ":seedling: Others"
	unknown       = ":question: Sort these by hand"
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

	fromTag = flag.String("from", "", "The tag or commit to start from.")

	since = flag.String("since", "", "Include commits starting from and including this date. Accepts format: YYYY-MM-DD")
	until = flag.String("until", "", "Include commits up to and including this date. Accepts format: YYYY-MM-DD")

	tagRegex = regexp.MustCompile(`^\[release-[\w-\.]*\]`)
)

func main() {
	flag.Parse()
	os.Exit(run())
}

func lastTag() string {
	if fromTag != nil && *fromTag != "" {
		return *fromTag
	}
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

const (
	missingAreaLabelPrefix   = "MISSING_AREA"
	areaLabelPrefix          = "area/"
	multipleAreaLabelsPrefix = "MULTIPLE_AREAS["
)

type githubPullRequest struct {
	Labels []githubLabel `json:"labels"`
}

type githubLabel struct {
	Name string `json:"name"`
}

func getAreaLabel(merge string) (string, error) {
	// Get pr id from merge commit
	prID := strings.Replace(strings.TrimSpace(strings.Split(merge, " ")[3]), "#", "", -1)

	cmd := exec.Command("gh", "api", "repos/kubernetes-sigs/cluster-api/pulls/"+prID) //nolint:gosec

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	pr := &githubPullRequest{}
	if err := json.Unmarshal(out, pr); err != nil {
		return "", err
	}

	var areaLabels []string
	for _, label := range pr.Labels {
		if area, ok := trimAreaLabel(label.Name); ok {
			areaLabels = append(areaLabels, area)
		}
	}

	switch len(areaLabels) {
	case 0:
		return missingAreaLabelPrefix, nil
	case 1:
		return areaLabels[0], nil
	default:
		return multipleAreaLabelsPrefix + strings.Join(areaLabels, "|") + "]", nil
	}
}

// trimAreaLabel removes the "area/" prefix from area labels and returns it.
// If the label is an area label, the second return value is true, otherwise false.
func trimAreaLabel(label string) (string, bool) {
	trimmed := strings.TrimPrefix(label, areaLabelPrefix)
	if len(trimmed) < len(label) {
		return trimmed, true
	}

	return label, false
}

func run() int {
	if err := ensureInstalledDependencies(); err != nil {
		fmt.Println(err)
		return 1
	}

	var commitRange string
	var cmd *exec.Cmd

	if *since != "" && *until != "" {
		commitRange = fmt.Sprintf("%s - %s", *since, *until)

		lastDay, err := increaseDateByOneDay(*until)
		if err != nil {
			fmt.Println(err)
			return 1
		}
		cmd = exec.Command("git", "rev-list", "HEAD", "--since=\""+*since+"\"", "--until=\""+lastDay+"\"", "--merges", "--pretty=format:%B") //nolint:gosec
	} else if *since != "" || *until != "" {
		fmt.Println("--since and --until are required together or both unset")
		return 1
	} else {
		commitRange = lastTag()
		cmd = exec.Command("git", "rev-list", commitRange+"..HEAD", "--merges", "--pretty=format:%B") //nolint:gosec
	}

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
		prefix, err := getAreaLabel(c.merge)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
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
		body = fmt.Sprintf("- %s: %s", prefix, body)
		_, _ = fmt.Sscanf(c.merge, "Merge pull request %s from %s", &prNumber, &fork)
		if key == documentation {
			merges[key] = append(merges[key], prNumber)
			continue
		}
		merges[key] = append(merges[key], formatMerge(body, prNumber))
	}

	// TODO Turn this into a link (requires knowing the project name + organization)
	fmt.Printf("Changes since %v\n---\n", commitRange)

	fmt.Printf("## :chart_with_upwards_trend: Fun stats\n")
	fmt.Printf("- %d new commits merged\n", len(commits))
	fmt.Printf("- %d breaking changes :warning:\n", len(merges[warning]))
	fmt.Printf("- %d feature additions ✨\n", len(merges[features]))
	fmt.Printf("- %d bugs fixed 🐛\n", len(merges[bugs]))
	fmt.Println()

	for _, key := range outputOrder {
		mergeslice := merges[key]
		if len(mergeslice) == 0 {
			continue
		}

		switch key {
		case documentation:
			fmt.Printf(
				":book: Additionally, there have been %d contributions to our documentation and book. (%s) \n\n",
				len(mergeslice),
				strings.Join(mergeslice, ", "),
			)
		default:
			fmt.Println("## " + key)
			sort.Strings(mergeslice)
			for _, merge := range mergeslice {
				fmt.Println(merge)
			}
			fmt.Println()
		}
	}

	fmt.Println("")
	fmt.Println("_Thanks to all our contributors!_ 😊")

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

func ensureInstalledDependencies() error {
	if !commandExists("git") {
		return errors.New("git not available. Git is required to be present in the PATH")
	}

	if !commandExists("gh") {
		return errors.New("gh GitHub CLI not available. GitHub CLI is required to be present in the PATH. Refer to https://cli.github.com/ for installation")
	}

	return nil
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
