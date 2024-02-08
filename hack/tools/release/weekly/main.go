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
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"

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

	since  string
	until  string
	branch string

	timeLayout = "2006-01-02"
	repo       = "cluster-api"
	owner      = "kubernetes-sigs"

	tagRegex = regexp.MustCompile(`^\[release-[\w-\.]*\]`)
)

func main() {
	flag.StringVar(&since, "since", "", "Include commits starting from and including this date. Accepts format: YYYY-MM-DD")
	flag.StringVar(&until, "until", "", "Include commits up to and including this date. Accepts format: YYYY-MM-DD")
	flag.StringVar(&branch, "branch", "release-1.6", "Release branch. Accepts formats: main, release-1.6")
	flag.Parse()
	os.Exit(run())
}

func run() int {
	if since == "" && until == "" {
		fmt.Println("--since and --until are required together or both unset")
		return 1
	}

	ghToken := os.Getenv("GITHUB_TOKEN")
	client := createGitHubClient(ghToken)

	branchValid, err := isValidBranch(branch, owner, repo, client)
	if err != nil {
		fmt.Printf("Unable to verify if branch '%s' is valid: %s\n", branch, err.Error())
		return 1
	}

	if !branchValid {
		fmt.Printf("Invalid branch '%s': branch does not exist. Example of valid branches: main, release-1.5.\n", branch)
		return 1
	}

	sinceTime, err := parseTime(since)
	if err != nil {
		fmt.Printf("Unable to parse time for 'since' parameter: %s\n", since)
		return 1
	}

	untilTime, err := parseTime(until)
	if err != nil {
		fmt.Printf("Unable to parse time for 'until' parameter: %s\n", until)
		return 1
	}

	pullRequests, err := getMergedPullRequests(client, owner, repo, branch, sinceTime, untilTime)
	if err != nil {
		fmt.Println("Unable to get merged pull requests:", err.Error())
		return 1
	}

	merges := map[string][]string{
		release.Features:      {},
		release.Bugs:          {},
		release.Documentation: {},
		release.Warning:       {},
		release.Other:         {},
		release.Unknown:       {},
	}

	for _, pr := range pullRequests {
		prTitle := trimTitle(pr.GetTitle())
		var key, prNumber string
		switch {
		case strings.HasPrefix(prTitle, ":sparkles:"), strings.HasPrefix(prTitle, "✨"):
			key = release.Features
			prTitle = strings.TrimPrefix(prTitle, ":sparkles:")
			prTitle = strings.TrimPrefix(prTitle, "✨")
		case strings.HasPrefix(prTitle, ":bug:"), strings.HasPrefix(prTitle, "🐛"):
			key = release.Bugs
			prTitle = strings.TrimPrefix(prTitle, ":bug:")
			prTitle = strings.TrimPrefix(prTitle, "🐛")
		case strings.HasPrefix(prTitle, ":book:"), strings.HasPrefix(prTitle, "📖"):
			key = release.Documentation
			prTitle = strings.TrimPrefix(prTitle, ":book:")
			prTitle = strings.TrimPrefix(prTitle, "📖")
			if strings.Contains(prTitle, "CAEP") || strings.Contains(prTitle, "proposal") {
				key = release.Proposals
			}
		case strings.HasPrefix(prTitle, ":seedling:"), strings.HasPrefix(prTitle, "🌱"):
			key = release.Other
			prTitle = strings.TrimPrefix(prTitle, ":seedling:")
			prTitle = strings.TrimPrefix(prTitle, "🌱")
		case strings.HasPrefix(prTitle, ":warning:"), strings.HasPrefix(prTitle, "⚠️"):
			key = release.Warning
			prTitle = strings.TrimPrefix(prTitle, ":warning:")
			prTitle = strings.TrimPrefix(prTitle, "⚠️")
		default:
			key = release.Unknown
		}

		prTitle = strings.TrimSpace(prTitle)
		if prTitle == "" {
			continue
		}
		prTitle = fmt.Sprintf("\t - %s", prTitle)
		if key == release.Documentation {
			merges[key] = append(merges[key], prNumber)
			continue
		}
		merges[key] = append(merges[key], formatMerge(prTitle, strconv.Itoa(pr.GetNumber())))
	}

	// TODO Turn this into a link (requires knowing the project name + organization).
	fmt.Println("Weekly update :rotating_light:")
	fmt.Printf("From %s to %s a total of %d changes merged into %s.\n\n", sinceTime.Format(timeLayout), untilTime.Format(timeLayout), len(pullRequests), branch)

	for _, key := range outputOrder {
		mergeslice := merges[key]
		if len(mergeslice) == 0 {
			continue
		}

		switch key {
		case release.Documentation:
			fmt.Printf("- %d Documentation and book contributions :book: \n\n", len(mergeslice))
		case release.Other:
			fmt.Printf("- %d Other changes :seedling:\n\n", len(merges[release.Other]))
		default:
			fmt.Printf("- %d %s\n", len(merges[key]), key)
			for _, merge := range mergeslice {
				fmt.Println(merge)
			}
			fmt.Println()
		}
	}

	fmt.Println("All merged PRs can be viewed in GitHub:")
	fmt.Println("https://github.com/kubernetes-sigs/cluster-api/pulls?q=is%3Apr+closed%3A" + sinceTime.Format(timeLayout) + ".." + untilTime.Format(timeLayout) + "+is%3Amerged+base%3A" + branch + "+\n")

	fmt.Println("*Thanks to all our contributors!* 😊")
	fmt.Println("/Your friendly comms release team")

	return 0
}

func trimTitle(title string) string {
	// Remove a tag prefix if found.
	title = tagRegex.ReplaceAllString(title, "")

	return strings.TrimSpace(title)
}

func formatMerge(line, prNumber string) string {
	if prNumber == "" {
		return line
	}
	return fmt.Sprintf("%s (#%s)", line, prNumber)
}

// Parse the time from string to time.Time format.
func parseTime(date string) (time.Time, error) {
	datetime, err := time.Parse(timeLayout, date)
	if err != nil {
		return time.Time{}, err
	}
	return datetime, nil
}

func getMergedPullRequests(client *github.Client, owner string, repo string, branch string, since time.Time, until time.Time) ([]*github.PullRequest, error) {
	var allPullRequests []*github.PullRequest

	listOptions := &github.ListOptions{PerPage: 100}

	for {
		pullRequests, resp, err := client.PullRequests.List(context.Background(), owner, repo, &github.PullRequestListOptions{
			State:       "closed",
			Sort:        "updated",
			Direction:   "desc",
			ListOptions: *listOptions,
			Base:        branch,
		})
		if err != nil {
			return nil, err
		}

		// Include PRs until EOD.
		until = until.Add(time.Hour * 24)
		for _, pr := range pullRequests {
			// Our query returned PRs sorted by most recently _updated_.
			// The moment we get a PR that is updated before our `since` timestamp,
			// we have seen all _merged_ PRs since `since`.
			if pr.UpdatedAt.Before(since) {
				return allPullRequests, nil
			}

			if pr.MergedAt != nil && pr.MergedAt.After(since) && pr.MergedAt.Before(until) {
				allPullRequests = append(allPullRequests, pr)
			}
		}

		if resp.NextPage == 0 {
			break
		}

		listOptions.Page = resp.NextPage
	}

	return allPullRequests, nil
}

// Checks if the branch exists.
func isValidBranch(branchName string, owner string, repo string, client *github.Client) (bool, error) {
	branches, _, err := client.Repositories.ListBranches(context.Background(), owner, repo, nil)
	if err != nil {
		return false, err
	}
	for _, branch := range branches {
		if branchName == branch.GetName() {
			return true, nil
		}
	}

	return false, nil
}

// If the token is empty, we create a GitHub client without authentication
// Using a token enables more api requests until we run into rate limitation
// This is not necessary, but a nice to have when extending this script and going beyond
// normal usage.
func createGitHubClient(ghToken string) *github.Client {
	if ghToken != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: ghToken},
		)

		tc := oauth2.NewClient(context.Background(), ts)

		return github.NewClient(tc)
	}

	return github.NewClient(nil)
}
