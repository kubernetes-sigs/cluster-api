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

package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v33/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/util/version"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
)

const (
	httpsScheme              = "https"
	githubDomain             = "github.com"
	githubReleaseRepository  = "releases"
	githubLatestReleaseLabel = "latest"
)

var (
	// Caches used to limit the number of GitHub API calls.

	cacheVersions = map[string][]string{}
	cacheReleases = map[string]*github.RepositoryRelease{}
	cacheFiles    = map[string][]byte{}
)

// gitHubRepository provides support for providers hosted on GitHub.
//
// We support GitHub repositories that use the release feature to publish artifacts and versions.
// Repositories must use versioned releases, including the "latest" meta version
// (https://help.github.com/en/github/administering-a-repository/linking-to-releases#linking-to-the-latest-release).
type gitHubRepository struct {
	providerConfig           config.Provider
	configVariablesClient    config.VariablesClient
	authenticatingHTTPClient *http.Client
	owner                    string
	repository               string
	defaultVersion           string
	rootPath                 string
	componentsPath           string
	injectClient             *github.Client
}

var _ Repository = &gitHubRepository{}

type githubRepositoryOption func(*gitHubRepository)

func injectGithubClient(c *github.Client) githubRepositoryOption {
	return func(g *gitHubRepository) {
		g.injectClient = c
	}
}

// DefaultVersion returns defaultVersion field of gitHubRepository struct.
func (g *gitHubRepository) DefaultVersion() string {
	return g.defaultVersion
}

// GetVersions returns the list of versions that are available in a provider repository.
func (g *gitHubRepository) GetVersions() ([]string, error) {
	versions, err := g.getVersions()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get repository versions")
	}
	return versions, nil
}

// RootPath returns rootPath field of gitHubRepository struct.
func (g *gitHubRepository) RootPath() string {
	return g.rootPath
}

// ComponentsPath returns componentsPath field of gitHubRepository struct.
func (g *gitHubRepository) ComponentsPath() string {
	return g.componentsPath
}

// GetFile returns a file for a given provider version.
func (g *gitHubRepository) GetFile(version, path string) ([]byte, error) {
	release, err := g.getReleaseByTag(version)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get GitHub release %s", version)
	}

	// download files from the release
	files, err := g.downloadFilesFromRelease(release, path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download files from GitHub release %s", version)
	}

	return files, nil
}

// NewGitHubRepository returns a gitHubRepository implementation.
func NewGitHubRepository(providerConfig config.Provider, configVariablesClient config.VariablesClient, opts ...githubRepositoryOption) (Repository, error) {
	if configVariablesClient == nil {
		return nil, errors.New("invalid arguments: configVariablesClient can't be nil")
	}

	rURL, err := url.Parse(providerConfig.URL())
	if err != nil {
		return nil, errors.Wrap(err, "invalid url")
	}

	// Check if the url is a github repository
	if rURL.Scheme != httpsScheme || rURL.Host != githubDomain {
		return nil, errors.New("invalid url: a GitHub repository url should start with https://github.com")
	}

	// Check if the path is in the expected format,
	// url's path has an extra leading slash at the end which we need to clean up before splitting.
	urlSplit := strings.Split(strings.TrimPrefix(rURL.Path, "/"), "/")
	if len(urlSplit) < 5 || urlSplit[2] != githubReleaseRepository {
		return nil, errors.Errorf(
			"invalid url: a GitHub repository url should be in the form https://github.com/{owner}/{Repository}/%s/{latest|version-tag}/{componentsClient.yaml}",
			githubReleaseRepository,
		)
	}

	// Extract all the info from url split.
	owner := urlSplit[0]
	repository := urlSplit[1]
	defaultVersion := urlSplit[3]
	path := strings.Join(urlSplit[4:], "/")

	// use path's directory as a rootPath
	rootPath := filepath.Dir(path)
	// use the file name (if any) as componentsPath
	componentsPath := getComponentsPath(path, rootPath)

	repo := &gitHubRepository{
		providerConfig:        providerConfig,
		configVariablesClient: configVariablesClient,
		owner:                 owner,
		repository:            repository,
		defaultVersion:        defaultVersion,
		rootPath:              rootPath,
		componentsPath:        componentsPath,
	}

	// process githubRepositoryOptions
	for _, o := range opts {
		o(repo)
	}

	if token, err := configVariablesClient.Get(config.GitHubTokenVariable); err == nil {
		repo.setClientToken(token)
	}

	if err := repo.resolveRedirects(); err != nil {
		return nil, errors.Wrap(err, "failed to resolve repository redirects")
	}

	if defaultVersion == githubLatestReleaseLabel {
		repo.defaultVersion, err = latestContractRelease(repo, clusterv1.GroupVersion.Version)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get GitHub latest version")
		}
	}

	return repo, nil
}

// getComponentsPath returns the file name.
func getComponentsPath(path string, rootPath string) string {
	filePath := strings.TrimPrefix(path, rootPath)
	componentsPath := strings.TrimPrefix(filePath, "/")
	return componentsPath
}

// getClient returns a github API client.
func (g *gitHubRepository) getClient() *github.Client {
	if g.injectClient != nil {
		return g.injectClient
	}
	return github.NewClient(g.authenticatingHTTPClient)
}

// setClientToken sets authenticatingHTTPClient field of gitHubRepository struct.
func (g *gitHubRepository) setClientToken(token string) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	g.authenticatingHTTPClient = oauth2.NewClient(context.TODO(), ts)
}

// getVersions returns all the release versions for a github repository.
func (g *gitHubRepository) getVersions() ([]string, error) {
	cacheID := fmt.Sprintf("%s/%s", g.owner, g.repository)
	if versions, ok := cacheVersions[cacheID]; ok {
		return versions, nil
	}

	client := g.getClient()

	// get all the releases
	// NB. currently Github API does not support result ordering, so it not possible to limit results
	releases, _, err := client.Repositories.ListReleases(context.TODO(), g.owner, g.repository, nil)
	if err != nil {
		return nil, g.handleGithubErr(err, "failed to get the list of releases")
	}
	versions := []string{}
	for _, r := range releases {
		r := r // pin
		if r.TagName == nil {
			continue
		}
		tagName := *r.TagName
		if _, err := version.ParseSemantic(tagName); err != nil {
			// Discard releases with tags that are not a valid semantic versions (the user can point explicitly to such releases).
			continue
		}
		versions = append(versions, tagName)
	}

	cacheVersions[cacheID] = versions
	return versions, nil
}

// getReleaseByTag returns the github repository release with a specific tag name.
func (g *gitHubRepository) getReleaseByTag(tag string) (*github.RepositoryRelease, error) {
	cacheID := fmt.Sprintf("%s/%s:%s", g.owner, g.repository, tag)
	if release, ok := cacheReleases[cacheID]; ok {
		return release, nil
	}

	client := g.getClient()

	release, _, err := client.Repositories.GetReleaseByTag(context.TODO(), g.owner, g.repository, tag)
	if err != nil {
		return nil, g.handleGithubErr(err, "failed to read release %q", tag)
	}

	if release == nil {
		return nil, errors.Errorf("failed to get release %q", tag)
	}

	cacheReleases[cacheID] = release
	return release, nil
}

// downloadFilesFromRelease download a file from release.
func (g *gitHubRepository) downloadFilesFromRelease(release *github.RepositoryRelease, fileName string) ([]byte, error) {
	ctx := context.TODO()

	cacheID := fmt.Sprintf("%s/%s:%s:%s", g.owner, g.repository, *release.TagName, fileName)
	if content, ok := cacheFiles[cacheID]; ok {
		return content, nil
	}

	client := g.getClient()
	absoluteFileName := filepath.Join(g.rootPath, fileName)

	// search for the file into the release assets, retrieving the asset id
	var assetID *int64
	for _, a := range release.Assets {
		if a.Name != nil && *a.Name == absoluteFileName {
			assetID = a.ID
			break
		}
	}
	if assetID == nil {
		return nil, errors.Errorf("failed to get file %q from %q release", fileName, *release.TagName)
	}

	reader, redirect, err := client.Repositories.DownloadReleaseAsset(context.TODO(), g.owner, g.repository, *assetID, http.DefaultClient)
	if err != nil {
		return nil, g.handleGithubErr(err, "failed to download file %q from %q release", *release.TagName, fileName)
	}
	if redirect != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, redirect, http.NoBody)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to download file %q from %q release via redirect location %q: failed to create request", *release.TagName, fileName, redirect)
		}

		response, err := http.DefaultClient.Do(req) //nolint:bodyclose // (NB: The reader is actually closed in a defer)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to download file %q from %q release via redirect location %q", *release.TagName, fileName, redirect)
		}
		reader = response.Body
	}
	defer reader.Close()

	// Read contents from the reader (redirect or not), and return.
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read downloaded file %q from %q release", *release.TagName, fileName)
	}

	cacheFiles[cacheID] = content
	return content, nil
}

// handleGithubErr wraps error messages.
func (g *gitHubRepository) handleGithubErr(err error, message string, args ...interface{}) error {
	if _, ok := err.(*github.RateLimitError); ok {
		return errors.New("rate limit for github api has been reached. Please wait one hour or get a personal API token and assign it to the GITHUB_TOKEN environment variable")
	}
	return errors.Wrapf(err, message, args...)
}

// resolveRedirects resolves a renamed GitHub repository to the correct one.
// For example, https://github.com/jetstack/cert-manager got renamed to  https://github.com/cert-manager/cert-manager
// this behavior should be caught before we do any operations
func (g *gitHubRepository) resolveRedirects() error {
	client := g.getClient()

	repo, _, err := client.Repositories.Get(context.TODO(), g.owner, g.repository)
	if err != nil {
		return g.handleGithubErr(err, "failed to resolve repository %s/%s", g.owner, g.repository)
	}

	if g.repository != *repo.Name {
		g.repository = *repo.Name
	}

	if g.owner != *repo.Owner.Login {
		g.owner = *repo.Owner.Login
	}

	return nil
}
