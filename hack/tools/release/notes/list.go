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
	"time"

	"github.com/pkg/errors"
)

// githubFromToPRLister lists PRs from GitHub contained between two refs.
type githubFromToPRLister struct {
	client         *githubClient
	fromRef, toRef ref
	// branch is optional. It helps optimize the PR query by restricting
	// the results to PRs merged in the selected branch and in main
	branch string
}

func newGithubFromToPRLister(repo string, fromRef, toRef ref, branch string) *githubFromToPRLister {
	return &githubFromToPRLister{
		client:  &githubClient{repo: repo},
		fromRef: fromRef,
		toRef:   toRef,
		branch:  branch,
	}
}

// listPRs returns the PRs merged between `fromRef` and `toRef` (included).
// It lists all PRs merged in main in the configured branch in the date
// range between fromRef and toRef (we include main because minor releases
// include PRs both in main and the release branch).
// Then it crosschecks them with the PR numbers found in the commits
// between fromRef and toRef, discarding any PR not seeing in the commits list.
// This ensures we don't include any PR merged in the same date range that
// doesn't belong to our git timeline.
func (l *githubFromToPRLister) listPRs(previousReleaseRef ref) ([]pr, error) {
	var (
		diff *githubDiff
		err  error
	)
	if previousReleaseRef.value != "" {
		log.Printf("Computing diff between %s and %s", previousReleaseRef.value, l.toRef)
		diff, err = l.client.getDiffAllCommits(previousReleaseRef.value, l.toRef.value)
	} else {
		log.Printf("Computing diff between %s and %s", l.fromRef, l.toRef)
		diff, err = l.client.getDiffAllCommits(l.fromRef.value, l.toRef.value)
	}
	if err != nil {
		return nil, err
	}

	log.Printf("Reading ref %s for upper limit", l.toRef)
	toRef, err := l.client.getRef(l.toRef.String())
	if err != nil {
		return nil, err
	}

	var toCommitSHA string
	if toRef.Object.ObjectType == tagType {
		log.Printf("Reading tag info %s for upper limit", toRef.Object.SHA)
		toTag, err := l.client.getTag(toRef.Object.SHA)
		if err != nil {
			return nil, err
		}
		toCommitSHA = toTag.Object.SHA
	} else {
		toCommitSHA = toRef.Object.SHA
	}

	log.Printf("Reading commit %s for upper limit", toCommitSHA)
	toCommit, err := l.client.getCommit(toCommitSHA)
	if err != nil {
		return nil, err
	}

	fromDate := diff.MergeBaseCommit.Commit.Committer.Date
	// We add an extra minute to avoid errors by 1 (observed during testing)
	// We cross check the list of PRs against the list of commits, so we will filter out
	// any PRs entries not belonging to the computed diff
	toDate := toCommit.Committer.Date.Add(1 * time.Minute)

	log.Printf("Listing PRs from %s to %s", fromDate, toDate)
	// We include both the configured branch and `main` as the base branches because when
	// cutting a new minor version, there will be  PRs merged in both main and the release branch.
	// This is just an optimization to avoid listing PRs over all branches, since we know we don't
	// need the PRs from other release branches.
	gPRs, err := l.client.listMergedPRs(fromDate, toDate, l.branch, "main")
	if err != nil {
		return nil, err
	}

	log.Printf("Found %d PRs in github", len(gPRs))

	selectedPRNumbers := buildSetOfPRNumbers(diff.Commits)

	prs := make([]pr, 0, len(gPRs))
	for _, p := range gPRs {
		if _, ok := selectedPRNumbers[fmt.Sprintf("%d", p.Number)]; !ok {
			continue
		}
		labels := make([]string, 0, len(p.Labels))
		for _, l := range p.Labels {
			labels = append(labels, l.Name)
		}
		prs = append(prs, pr{
			number: p.Number,
			title:  p.Title,
			labels: labels,
			user:   p.User.Login,
		})
	}

	log.Printf("%d PRs match the commits from the git diff", len(prs))

	if len(prs) != len(selectedPRNumbers) {
		return nil, errors.Errorf("expected %d PRs from commit list but only found %d", len(selectedPRNumbers), len(prs))
	}

	return prs, nil
}

var (
	mergeCommitMessage        = regexp.MustCompile(`(?m)^Merge pull request #(\d+) .*$`)
	tideSquashedCommitMessage = regexp.MustCompile(`(?m)^.+\(#(?P<number>\d+)\)$`)
)

func buildSetOfPRNumbers(commits []githubCommitNode) map[string]struct{} {
	prNumbers := make(map[string]struct{})
	for _, commit := range commits {
		match := mergeCommitMessage.FindStringSubmatch(commit.Commit.Message)
		if len(match) == 2 {
			prNumbers[match[1]] = struct{}{}
			continue
		}

		match = tideSquashedCommitMessage.FindStringSubmatch(commit.Commit.Message)
		if len(match) == 2 {
			prNumbers[match[1]] = struct{}{}
		}
	}

	return prNumbers
}
