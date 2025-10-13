//go:build tools
// +build tools

/*
Copyright 2025 The Kubernetes Authors.

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
	"time"
)

// mockGithubClient is a mock implementation of githubClientInterface for testing.
type mockGithubClient struct {
	// Mock responses
	diffResponse   *githubDiff
	refResponse    githubRef
	tagResponse    githubTag
	commitResponse githubCommit
	prsResponse    []githubPR

	// Mock errors
	diffError   error
	refError    error
	tagError    error
	commitError error
	prsError    error
}

// Ensure mockGithubClient implements githubClientInterface.
var _ githubClientInterface = (*mockGithubClient)(nil)

func (m *mockGithubClient) getDiffAllCommits(_, _ string) (*githubDiff, error) {
	if m.diffError != nil {
		return nil, m.diffError
	}
	return m.diffResponse, nil
}

func (m *mockGithubClient) getRef(_ string) (githubRef, error) {
	if m.refError != nil {
		return githubRef{}, m.refError
	}
	return m.refResponse, nil
}

func (m *mockGithubClient) getTag(_ string) (githubTag, error) {
	if m.tagError != nil {
		return githubTag{}, m.tagError
	}
	return m.tagResponse, nil
}

func (m *mockGithubClient) getCommit(_ string) (githubCommit, error) {
	if m.commitError != nil {
		return githubCommit{}, m.commitError
	}
	return m.commitResponse, nil
}

func (m *mockGithubClient) listMergedPRs(_, _ time.Time, _ ...string) ([]githubPR, error) {
	if m.prsError != nil {
		return nil, m.prsError
	}
	return m.prsResponse, nil
}

// newMockGithubClient creates a new mock client with default responses.
func newMockGithubClient() *mockGithubClient {
	return &mockGithubClient{
		diffResponse: &githubDiff{
			MergeBaseCommit: githubCommitNode{
				Commit: githubCommit{
					Message: "Merge commit",
					Committer: githubCommitter{
						Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
					},
				},
			},
			Commits: []githubCommitNode{
				{
					Commit: githubCommit{
						Message: "Merge pull request #1234 from test/branch",
					},
				},
			},
		},
		refResponse: githubRef{
			Object: githubObject{
				ObjectType: commitType,
				SHA:        "abc123",
			},
		},
		tagResponse: githubTag{
			Object: githubObject{
				ObjectType: tagType,
				SHA:        "def456",
			},
		},
		commitResponse: githubCommit{
			Message: "Test commit",
			Committer: githubCommitter{
				Date: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
			},
		},
		prsResponse: []githubPR{
			{
				Number: 1234,
				Title:  "Test PR",
				Labels: []githubLabel{
					{Name: "area/testing"},
				},
				User: githubUser{
					Login: "testuser",
				},
			},
		},
	}
}

// newMockGithubClientWithError creates a mock client that returns an error for specific operations.
func newMockGithubClientWithError(operation string, err error) *mockGithubClient {
	mock := newMockGithubClient()
	switch operation {
	case "diff":
		mock.diffError = err
	case "ref":
		mock.refError = err
	case "tag":
		mock.tagError = err
	case "commit":
		mock.commitError = err
	case "prs":
		mock.prsError = err
	}
	return mock
}

// newMockGithubClientForInvalidRef creates a mock client that simulates invalid ref scenarios.
func newMockGithubClientForInvalidRef() *mockGithubClient {
	mock := newMockGithubClient()
	mock.diffError = fmt.Errorf("invalid ref")
	return mock
}
