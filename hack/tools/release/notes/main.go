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
	"sync"
	"time"

	release "sigs.k8s.io/cluster-api/hack/tools/release/internal"
)

/*
This tool prints all the titles of all PRs from previous release to HEAD.
This needs to be run *before* a tag is created.

Use these as the base of your release notes.
*/

var (
	outputOrder = []string{
		release.Proposals,
		release.Warning,
		release.Features,
		release.Bugs,
		release.Other,
		release.Documentation,
		release.Unknown,
	}

	repo = flag.String("repository", "kubernetes-sigs/cluster-api", "The repo to run the tool from.")

	fromTag = flag.String("from", "", "The tag or commit to start from.")

	since      = flag.String("since", "", "Include commits starting from and including this date. Accepts format: YYYY-MM-DD")
	until      = flag.String("until", "", "Include commits up to and including this date. Accepts format: YYYY-MM-DD")
	numWorkers = flag.Int("workers", 10, "Number of concurrent routines to process PR entries. If running into GitHub rate limiting, use 1.")

	prefixAreaLabel = flag.Bool("prefix-area-label", true, "If enabled, will prefix the area label.")

	preReleaseVersion           = flag.Bool("pre-release-version", false, "If enabled, will add a pre-release warning header. (default false)")
	deprecation                 = flag.Bool("deprecation", true, "If enabled, will add a templated deprecation warning header.")
	addKubernetesVersionSupport = flag.Bool("add-kubernetes-version-support", true, "If enabled, will add the Kubernetes version support header.")

	tagRegex = regexp.MustCompile(`^\[release-[\w-\.]*\]`)

	userFriendlyAreas = map[string]string{
		"e2e-testing":                       "e2e",
		"provider/control-plane-kubeadm":    "KCP",
		"provider/infrastructure-docker":    "CAPD",
		"dependency":                        "Dependency",
		"devtools":                          "Devtools",
		"machine":                           "Machine",
		"api":                               "API",
		"machinepool":                       "MachinePool",
		"clustercachetracker":               "ClusterCacheTracker",
		"clusterclass":                      "ClusterClass",
		"testing":                           "Testing",
		"release":                           "Release",
		"machineset":                        "MachineSet",
		"clusterresourceset":                "ClusterResourceSet",
		"machinedeployment":                 "MachineDeployment",
		"ipam":                              "IPAM",
		"provider/bootstrap-kubeadm":        "CAPBK",
		"provider/infrastructure-in-memory": "CAPIM",
		"provider/core":                     "Core",
		"runtime-sdk":                       "Runtime SDK",
		"ci":                                "CI",
		"machinehealthcheck":                "MachineHealthCheck",
		"clusterctl":                        "clusterctl", // Preserve lowercase
		"util":                              "util",       // Preserve lowercase
		"Community-meeting":                 "Community meeting",
	}

	releaseBackportMarker = regexp.MustCompile(`(?m)^\[release-\d\.\d\]\s*`)
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
	documentationAreaLabel   = "documentation"
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

	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/pulls/%s", *repo, prID)) //nolint:gosec

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %v", string(out), err)
	}

	pr := &githubPullRequest{}
	if err := json.Unmarshal(out, pr); err != nil {
		return "", err
	}

	var areaLabels []string
	for _, label := range pr.Labels {
		if area, ok := trimAreaLabel(label.Name); ok {
			if userFriendlyArea, ok := userFriendlyAreas[area]; ok {
				area = userFriendlyArea
			} else {
				area = capitalize(area)
			}

			areaLabels = append(areaLabels, area)
		}
	}

	switch len(areaLabels) {
	case 0:
		return missingAreaLabelPrefix, nil
	case 1:
		return areaLabels[0], nil
	default:
		return multipleAreaLabelsPrefix + strings.Join(areaLabels, "/") + "]", nil
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
		release.Features:      {},
		release.Bugs:          {},
		release.Documentation: {},
		release.Warning:       {},
		release.Other:         {},
		release.Unknown:       {},
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

	results := make(chan releaseNoteEntryResult)
	commitCh := make(chan *commit)
	var wg sync.WaitGroup

	wg.Add(*numWorkers)
	for i := 0; i < *numWorkers; i++ {
		go func() {
			for commit := range commitCh {
				processed := releaseNoteEntryResult{}
				processed.prEntry, processed.err = generateReleaseNoteEntry(commit)
				results <- processed
			}
			wg.Done()
		}()
	}

	go func() {
		for _, c := range commits {
			commitCh <- c
		}
		close(commitCh)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if result.err != nil {
			fmt.Println(result.err)
			os.Exit(0)
		}

		if result.prEntry == nil || result.prEntry.title == "" {
			continue
		}

		if result.prEntry.section == release.Documentation {
			merges[result.prEntry.section] = append(merges[result.prEntry.section], result.prEntry.prNumber)
		} else {
			merges[result.prEntry.section] = append(merges[result.prEntry.section], result.prEntry.title)
		}
	}

	if *preReleaseVersion {
		fmt.Printf("🚨 This is a RELEASE CANDIDATE. Use it only for testing purposes. If you find any bugs, file an [issue](https://github.com/%s/issues/new).\n", *repo)
	}

	if *addKubernetesVersionSupport {
		fmt.Print(`## 👌 Kubernetes version support

- Management Cluster: v1.**X**.x -> v1.**X**.x
- Workload Cluster: v1.**X**.x -> v1.**X**.x

[More information about version support can be found here](https://cluster-api.sigs.k8s.io/reference/versions.html)

`)
	}

	fmt.Print(`## Highlights

* REPLACE ME

`)

	if *deprecation {
		fmt.Print(`## Deprecation Warning

REPLACE ME: A couple sentences describing the deprecation, including links to docs.

* [GitHub issue #REPLACE ME](REPLACE ME)

`)
	}

	fmt.Printf("## Changes since %v\n", commitRange)

	fmt.Printf("## :chart_with_upwards_trend: Overview\n")
	if count := len(commits); count == 1 {
		fmt.Println("- 1 new commit merged")
	} else if count > 1 {
		fmt.Printf("- %d new commits merged\n", count)
	}
	if count := len(merges[release.Warning]); count == 1 {
		fmt.Println("- 1 breaking change :warning:")
	} else if count > 1 {
		fmt.Printf("- %d breaking changes :warning:\n", count)
	}
	if count := len(merges[release.Features]); count == 1 {
		fmt.Println("- 1 feature addition ✨")
	} else if count > 1 {
		fmt.Printf("- %d feature additions ✨\n", count)
	}
	if count := len(merges[release.Bugs]); count == 1 {
		fmt.Println("- 1 bug fixed 🐛")
	} else if count > 1 {
		fmt.Printf("- %d bugs fixed 🐛\n", count)
	}
	fmt.Println()

	for _, key := range outputOrder {
		mergeslice := merges[key]
		if len(mergeslice) == 0 {
			continue
		}

		switch key {
		case release.Documentation:
			sort.Strings(mergeslice)
			if len(mergeslice) == 1 {
				fmt.Printf(
					":book: Additionally, there has been 1 contribution to our documentation and book. (%s) \n\n",
					mergeslice[0],
				)
			} else {
				fmt.Printf(
					":book: Additionally, there have been %d contributions to our documentation and book. (%s) \n\n",
					len(mergeslice),
					strings.Join(mergeslice, ", "),
				)
			}
		default:
			fmt.Println("## " + key)
			sort.Slice(mergeslice, func(i int, j int) bool {
				str1 := strings.ToLower(mergeslice[i])
				str2 := strings.ToLower(mergeslice[j])
				return str1 < str2
			})

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

// releaseNoteEntryResult is the result of processing a PR to create a release note item.
// Used to aggregate the line item and error when processing concurrently.
type releaseNoteEntryResult struct {
	prEntry *releaseNoteEntry
	err     error
}

// releaseNoteEntry represents a line item in the release notes.
type releaseNoteEntry struct {
	title    string
	section  string
	prNumber string
}

// removePrefixes removes the specified prefixes from the title.
func removePrefixes(title string, prefixes []string) string {
	entryWithoutTag := title
	for _, prefix := range prefixes {
		entryWithoutTag = strings.TrimLeft(strings.TrimPrefix(entryWithoutTag, prefix), " ")
	}

	return entryWithoutTag
}

// trimAreaFromTitle removes the prefixed area from title to avoid duplication.
func trimAreaFromTitle(title, area string) string {
	titleWithoutArea := title
	pattern := `(?i)^` + regexp.QuoteMeta(area+":")
	re := regexp.MustCompile(pattern)
	titleWithoutArea = re.ReplaceAllString(titleWithoutArea, "")
	titleWithoutArea = strings.TrimSpace(titleWithoutArea)
	return titleWithoutArea
}

func capitalize(str string) string {
	return strings.ToUpper(string(str[0])) + str[1:]
}

// generateReleaseNoteEntry processes a commit into a PR line item for the release notes.
func generateReleaseNoteEntry(c *commit) (*releaseNoteEntry, error) {
	entry := &releaseNoteEntry{}
	if c.body == "" {
		c.body = "ERROR: BODY MISSING. FIX MANUALLY"
	}
	entry.title = trimTitle(c.body)
	var fork string

	var area string
	if *prefixAreaLabel {
		var err error
		area, err = getAreaLabel(c.merge)
		if err != nil {
			return nil, err
		}
	}

	switch {
	case strings.HasPrefix(entry.title, ":sparkles:"), strings.HasPrefix(entry.title, "✨"):
		entry.section = release.Features
		entry.title = removePrefixes(entry.title, []string{":sparkles:", "✨"})
	case strings.HasPrefix(entry.title, ":bug:"), strings.HasPrefix(entry.title, "🐛"):
		entry.section = release.Bugs
		entry.title = removePrefixes(entry.title, []string{":bug:", "🐛"})
	case strings.HasPrefix(entry.title, ":book:"), strings.HasPrefix(entry.title, "📖"):
		entry.section = release.Documentation
		entry.title = removePrefixes(entry.title, []string{":book:", "📖"})
		if strings.Contains(entry.title, "CAEP") || strings.Contains(entry.title, "proposal") {
			entry.section = release.Proposals
		}
	case strings.HasPrefix(entry.title, ":warning:"), strings.HasPrefix(entry.title, "⚠️"):
		entry.section = release.Warning
		entry.title = removePrefixes(entry.title, []string{":warning:", "⚠️"})
	case strings.HasPrefix(entry.title, "🚀"), strings.HasPrefix(entry.title, "🌱 Release v1."):
		// TODO(g-gaston): remove the second condition using 🌱 prefix once 1.6 is released
		// Release trigger PRs from previous releases are not included in the release notes
		return nil, nil
	case strings.HasPrefix(entry.title, ":seedling:"), strings.HasPrefix(entry.title, "🌱"):
		entry.section = release.Other
		entry.title = removePrefixes(entry.title, []string{":seedling:", "🌱"})
	default:
		entry.section = release.Unknown
	}

	// If the area label indicates documentation, use documentation as the section
	// no matter what was the emoji used. This takes into account that the area label
	// tends to be more accurate than the emoji (data point observed by the release team).
	// We handle this after the switch statement to make sure we remove all emoji prefixes.
	if area == documentationAreaLabel {
		entry.section = release.Documentation
	}

	entry.title = strings.TrimSpace(entry.title)
	entry.title = trimReleaseBackportMarker(entry.title)

	if entry.title == "" {
		return entry, nil
	}

	if *prefixAreaLabel {
		entry.title = trimAreaFromTitle(entry.title, area)
		entry.title = capitalize(entry.title)
		entry.title = fmt.Sprintf("- %s: %s", area, entry.title)
	} else {
		entry.title = capitalize(entry.title)
		entry.title = fmt.Sprintf("- %s", entry.title)
	}

	_, _ = fmt.Sscanf(c.merge, "Merge pull request %s from %s", &entry.prNumber, &fork)
	entry.title = formatMerge(entry.title, entry.prNumber)

	return entry, nil
}

// trimReleaseBackportMarker removes the `[release-x.x]` prefix from a PR title if present.
// These are mostly used for back-ported PRs in release branches.
func trimReleaseBackportMarker(title string) string {
	return releaseBackportMarker.ReplaceAllString(title, "${1}")
}
