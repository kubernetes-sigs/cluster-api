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
	"sort"
	"strings"

	release "sigs.k8s.io/cluster-api/hack/tools/release/internal"
)

var defaultOutputOrder = []string{
	release.Proposals,
	release.Warning,
	release.Features,
	release.Bugs,
	release.Other,
	release.Documentation,
	release.Unknown,
}

var isExpanderAdded = false
var isPreReleasePrinted = false

// releaseNotesPrinter outputs the PR entries following
// the right format for the release notes.
type releaseNotesPrinter struct {
	outputOrder            []string
	releaseType            string
	printKubernetesSupport bool
	printDeprecation       bool
	fromTag                string
	repo                   string
}

func newReleaseNotesPrinter(repo, fromTag string) *releaseNotesPrinter {
	return &releaseNotesPrinter{
		repo:        repo,
		fromTag:     fromTag,
		outputOrder: defaultOutputOrder,
	}
}

// print outputs to stdout the release notes.
func (p *releaseNotesPrinter) print(entries []notesEntry, commitsInRelease int, dependencies string, previousReleaseRef ref) {
	merges := map[string][]string{
		release.Features:      {},
		release.Bugs:          {},
		release.Documentation: {},
		release.Warning:       {},
		release.Other:         {},
		release.Unknown:       {},
	}

	for _, entry := range entries {
		if entry.section == release.Documentation {
			merges[entry.section] = append(merges[entry.section], "#"+entry.prNumber)
		} else {
			merges[entry.section] = append(merges[entry.section], entry.title)
		}
	}

	if p.releaseType != "" && !isPreReleasePrinted {
		fmt.Printf("🚨 This is a %s. Use it only for testing purposes. If you find any bugs, file an [issue](https://github.com/%s/issues/new).\n", p.releaseType, p.repo)
		isPreReleasePrinted = true
	}

	if p.releaseType != "" && previousReleaseRef.value == "" {
		// This will add the release notes expansion functionality for a pre-release version
		fmt.Print(`<details>
<summary>More details about the release</summary>
`)
		fmt.Printf("\n:warning: **%s NOTES** :warning:\n", p.releaseType)

		isExpanderAdded = true
	}

	if p.printKubernetesSupport && previousReleaseRef.value == "" {
		fmt.Print(`## 👌 Kubernetes version support

- Management Cluster: v1.**X**.x -> v1.**X**.x
- Workload Cluster: v1.**X**.x -> v1.**X**.x

[More information about version support can be found here](https://cluster-api.sigs.k8s.io/reference/versions.html)

`)
	}

	fmt.Print(`## Highlights

* REPLACE ME

`)

	if p.printDeprecation {
		fmt.Print(`## Deprecation Warning

REPLACE ME: A couple sentences describing the deprecation, including links to docs.

* [GitHub issue #REPLACE ME](REPLACE ME)

`)
	}

	if previousReleaseRef.value != "" {
		fmt.Printf("## Changes since %s\n", previousReleaseRef.value)
	} else {
		fmt.Printf("## Changes since %s\n", p.fromTag)
	}

	fmt.Printf("## :chart_with_upwards_trend: Overview\n")
	if commitsInRelease == 1 {
		fmt.Println("- 1 new commit merged")
	} else if commitsInRelease > 1 {
		fmt.Printf("- %d new commits merged\n", commitsInRelease)
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

	for _, key := range p.outputOrder {
		mergeslice := merges[key]
		if len(mergeslice) == 0 {
			continue
		}

		switch key {
		case release.Documentation:
			if previousReleaseRef.value == "" {
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

	fmt.Print(dependencies)

	fmt.Println("")
	if isExpanderAdded {
		fmt.Print("</details>\n<br/>\n")
	}
	if previousReleaseRef.value == "" {
		fmt.Println("_Thanks to all our contributors!_ 😊")
	}
}
