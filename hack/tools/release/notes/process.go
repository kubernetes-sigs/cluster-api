//go:build tools
// +build tools

/*
Copyright 2023 The Kubernetes Authors.

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
	"regexp"
	"strings"

	"k8s.io/release/pkg/notes"

	release "sigs.k8s.io/cluster-api/hack/tools/release/internal"
)

const (
	missingAreaLabelPrefix   = "MISSING_AREA"
	areaLabelPrefix          = "area/"
	multipleAreaLabelsPrefix = "MULTIPLE_AREAS["
	documentationArea        = "Documentation"
)

var (
	defaultUserFriendlyAreas = map[string]string{
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
		"provider/bootstrap-kubeadm":        "CABPK",
		"provider/infrastructure-in-memory": "CAPIM",
		"provider/core":                     "Core",
		"runtime-sdk":                       "Runtime SDK",
		"ci":                                "CI",
		"machinehealthcheck":                "MachineHealthCheck",
		"clusterctl":                        "clusterctl", // Preserve lowercase
		"util":                              "util",       // Preserve lowercase
		"community-meeting":                 "Community meeting",
	}

	tagRegex              = regexp.MustCompile(`^\[release-[\w-\.]*\]`)
	releaseBackportMarker = regexp.MustCompile(`(?m)^\[release-\d\.\d\]\s*`)
)

type prEntriesProcessor struct {
	userFriendlyAreas map[string]string
	addAreaPrefix     bool
}

func newPREntryProcessor(addAreaPrefix bool) prEntriesProcessor {
	return prEntriesProcessor{
		userFriendlyAreas: defaultUserFriendlyAreas,
		addAreaPrefix:     addAreaPrefix,
	}
}

type dependenciesProcessor struct {
	repo    string
	fromTag string
	toTag   string
}

func newDependenciesProcessor(repo, fromTag, toTag string) dependenciesProcessor {
	return dependenciesProcessor{
		repo:    repo,
		fromTag: fromTag,
		toTag:   toTag,
	}
}

// process generates a PR entry ready for printing per PR. It extracts the area
// from the PR labels and appends it as a prefix to the title.
// It might skip some PRs depending on the title.
func (g prEntriesProcessor) process(prs []pr) []notesEntry {
	entries := make([]notesEntry, 0, len(prs))
	for i := range prs {
		pr := &prs[i]

		entry := g.generateNoteEntry(pr)

		if entry == nil || entry.title == "" {
			log.Printf("Ignoring PR [%s (#%d)]", pr.title, pr.number)
			continue
		}

		entries = append(entries, *entry)
	}

	return entries
}

func (d dependenciesProcessor) generateDependencies(previousRelease ref) (string, error) {
	repoURL := fmt.Sprintf("https://github.com/%s", d.repo)

	fromTag := d.fromTag
	if previousRelease.value != "" {
		fromTag = previousRelease.value
	}

	deps, err := notes.NewDependencies().ChangesForURL(
		repoURL, fromTag, d.toTag,
	)
	if err != nil {
		return "", err
	}
	return deps, nil
}

func (g prEntriesProcessor) generateNoteEntry(p *pr) *notesEntry {
	entry := &notesEntry{}

	entry.title = trimTitle(p.title)

	var area string
	if g.addAreaPrefix {
		area = g.extractArea(p)
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
		return nil
	case strings.HasPrefix(entry.title, ":seedling:"), strings.HasPrefix(entry.title, "🌱"):
		// Skip PRs from depndabot. Dependency updates are listed in the dependencies section.
		if p.user == "dependabot[bot]" {
			return nil
		}
		entry.section = release.Other
		entry.title = removePrefixes(entry.title, []string{":seedling:", "🌱"})
	default:
		entry.section = release.Unknown
	}

	// If the area label indicates documentation, use documentation as the section
	// no matter what was the emoji used. This takes into account that the area label
	// tends to be more accurate than the emoji (data point observed by the release team).
	// We handle this after the switch statement to make sure we remove all emoji prefixes.
	if area == documentationArea {
		entry.section = release.Documentation
	}

	entry.title = strings.TrimSpace(entry.title)
	entry.title = trimReleaseBackportMarker(entry.title)

	if entry.title == "" {
		return nil
	}

	if g.addAreaPrefix {
		entry.title = trimAreaFromTitle(entry.title, area)
		entry.title = capitalize(entry.title)
		entry.title = fmt.Sprintf("- %s: %s", area, entry.title)
	} else {
		entry.title = capitalize(entry.title)
		entry.title = fmt.Sprintf("- %s", entry.title)
	}

	entry.prNumber = fmt.Sprintf("%d", p.number)
	entry.title = formatPREntry(entry.title, entry.prNumber)

	return entry
}

// extractArea processes the PR labels to extract the area.
func (g prEntriesProcessor) extractArea(pr *pr) string {
	var areaLabels []string
	for _, label := range pr.labels {
		if area, ok := trimAreaLabel(label); ok {
			if userFriendlyArea, ok := g.userFriendlyAreas[area]; ok {
				area = userFriendlyArea
			} else {
				area = capitalize(area)
			}

			areaLabels = append(areaLabels, area)
		}
	}

	switch len(areaLabels) {
	case 0:
		return missingAreaLabelPrefix
	case 1:
		return areaLabels[0]
	default:
		return multipleAreaLabelsPrefix + strings.Join(areaLabels, "/") + "]"
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

// trimTitle removes release tags and white space from
// PR titles.
func trimTitle(title string) string {
	// Remove a tag prefix if found.
	title = tagRegex.ReplaceAllString(title, "")

	return strings.TrimSpace(title)
}

// formatPREntry appends the PR number at the end of the title
// and makes it a valid link in GitHub release notes.
func formatPREntry(line, prNumber string) string {
	if prNumber == "" {
		return line
	}
	return fmt.Sprintf("%s (#%s)", line, prNumber)
}

func capitalize(str string) string {
	return strings.ToUpper(string(str[0])) + str[1:]
}

// trimReleaseBackportMarker removes the `[release-x.x]` prefix from a PR title if present.
// These are mostly used for back-ported PRs in release branches.
func trimReleaseBackportMarker(title string) string {
	return releaseBackportMarker.ReplaceAllString(title, "${1}")
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
