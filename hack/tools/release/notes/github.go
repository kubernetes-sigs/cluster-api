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
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os/exec"
	"strings"
	"time"
)

// githubClient uses the gh CLI to make API request to GitHub.
type githubClient struct {
	// repo is full [org]/[repo_name]
	repo string
}

// githubDiff is the API response for the "compare" endpoint.
type githubDiff struct {
	// MergeBaseCommit points to most recent common ancestor between two references.
	MergeBaseCommit githubCommitNode   `json:"merge_base_commit"`
	Commits         []githubCommitNode `json:"commits"`
	Total           int                `json:"total_commits"`
}

type githubCommitNode struct {
	Commit githubCommit `json:"commit"`
}

type githubCommitter struct {
	Date time.Time `json:"date"`
}

// getDiffAllCommits calls the `compare` endpoint, iterating over all pages and aggregating results.
func (c githubClient) getDiffAllCommits(base, head string) (*githubDiff, error) {
	pageSize := 250
	url := fmt.Sprintf("repos/%s/compare/%s...%s", c.repo, base, head)

	diff, commits, err := iterate(c, url, pageSize, nil, func(page *githubDiff) ([]githubCommitNode, int) {
		return page.Commits, page.Total
	})
	if err != nil {
		return nil, err
	}

	diff.Commits = commits

	return diff, nil
}

// githubRef is the API response for the "ref" endpoint.
type githubRef struct {
	Object githubObject `json:"object"`
}

type objectType string

const (
	commitType objectType = "commit"
	tagType    objectType = "tag"
)

type githubObject struct {
	ObjectType objectType `json:"type"`
	SHA        string     `json:"sha"`
}

// getRef calls the `git/ref` endpoint.
func (c githubClient) getRef(ref string) (githubRef, error) {
	refResponse := githubRef{}
	if err := c.runGHAPICommand(fmt.Sprintf("repos/%s/git/ref/%s", c.repo, ref), &refResponse); err != nil {
		return githubRef{}, err
	}
	return refResponse, nil
}

// githubTag is the API response for the "tags" endpoint.
type githubTag struct {
	Object githubObject `json:"object"`
}

// getTag calls the `tags` endpoint.
func (c githubClient) getTag(tagSHA string) (githubTag, error) {
	tagResponse := githubTag{}
	if err := c.runGHAPICommand(fmt.Sprintf("repos/%s/git/tags/%s", c.repo, tagSHA), &tagResponse); err != nil {
		return githubTag{}, err
	}
	return tagResponse, nil
}

// githubCommit is the API response for a "git/commits" request.
type githubCommit struct {
	Message   string          `json:"message"`
	Committer githubCommitter `json:"committer"`
}

// getCommit calls the `commits` endpoint.
func (c githubClient) getCommit(sha string) (githubCommit, error) {
	commit := githubCommit{}
	if err := c.runGHAPICommand(fmt.Sprintf("repos/%s/git/commits/%s", c.repo, sha), &commit); err != nil {
		return githubCommit{}, err
	}
	return commit, nil
}

// githubPRList is the API response for the "search" endpoint.
type githubPRList struct {
	Total int        `json:"total_count"`
	Items []githubPR `json:"items"`
}

// githubPR is the API object included in a "search" query response when the
// return item is a PR.
type githubPR struct {
	Number uint64        `json:"number"`
	Title  string        `json:"title"`
	Labels []githubLabel `json:"labels"`
	User   githubUser    `json:"user"`
}

type githubLabel struct {
	Name string `json:"name"`
}

type githubUser struct {
	Login string `json:"login"`
}

// listMergedPRs calls the `search` endpoint and queries for PRs.
func (c githubClient) listMergedPRs(after, before time.Time, baseBranches ...string) ([]githubPR, error) {
	pageSize := 100
	searchQuery := fmt.Sprintf("repo:%s+is:pr+is:merged+merged:%s..%s", c.repo, after.Format(time.RFC3339), before.Format(time.RFC3339))
	if len(baseBranches) != 0 {
		searchQuery += "+base:" + strings.Join(baseBranches, "+base:")
	}

	_, prs, err := iterate(c, "search/issues", pageSize, []string{"q=" + searchQuery}, func(page *githubPRList) ([]githubPR, int) {
		return page.Items, page.Total
	})
	if err != nil {
		return nil, err
	}

	return prs, nil
}

func (c githubClient) runGHAPICommand(url string, response any) error {
	cmd := exec.Command("gh", "api", url) //nolint:noctx

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v", string(out), err)
	}

	return json.Unmarshal(out, response)
}

type extractFunc[T, C any] func(page *T) (pageElements []C, totalElements int)

func iterate[T, C any](client githubClient, url string, pageSize int, extraQueryArgs []string, extract extractFunc[T, C]) (*T, []C, error) {
	page := 0
	totalElements := math.MaxInt
	elementsRead := 0

	var firstPage *T
	var collection []C

	for elementsRead < totalElements {
		page++
		requestURL := fmt.Sprintf("%s?per_page=%d&page=%d&%s", url, pageSize, page, strings.Join(extraQueryArgs, "&"))
		log.Printf("Calling endpoint %s", requestURL)
		pageResult := new(T)
		if err := client.runGHAPICommand(requestURL, pageResult); err != nil {
			return nil, nil, err
		}

		pageRead, t := extract(pageResult)
		collection = append(collection, pageRead...)
		elementsRead += len(pageRead)
		totalElements = t

		if firstPage == nil {
			firstPage = pageResult
		}
	}

	log.Printf("Total of %d pages and %d elements read", page, len(collection))

	return firstPage, collection, nil
}
