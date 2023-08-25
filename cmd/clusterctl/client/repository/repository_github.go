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
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/google/go-github/v53/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/internal/goproxy"
)

const (
	httpsScheme                    = "https"
	githubDomain                   = "github.com"
	githubReleaseRepository        = "releases"
	githubLatestReleaseLabel       = "latest"
	githubListReleasesPerPageLimit = 100
)

var (
	errNotFound = errors.New("404 Not Found")

	// Caches used to limit the number of GitHub API calls.

	cacheVersions              = map[string][]string{}
	cacheReleases              = map[string]*github.RepositoryRelease{}
	cacheFiles                 = map[string][]byte{}
	retryableOperationInterval = 10 * time.Second
	retryableOperationTimeout  = 1 * time.Minute
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
	injectGoproxyClient      *goproxy.Client
}

var _ Repository = &gitHubRepository{}

type githubRepositoryOption func(*gitHubRepository)

func injectGithubClient(c *github.Client) githubRepositoryOption {
	return func(g *gitHubRepository) {
		g.injectClient = c
	}
}

func injectGoproxyClient(c *goproxy.Client) githubRepositoryOption {
	return func(g *gitHubRepository) {
		g.injectGoproxyClient = c
	}
}

// DefaultVersion returns defaultVersion field of gitHubRepository struct.
func (g *gitHubRepository) DefaultVersion() string {
	return g.defaultVersion
}

// GetVersions returns the list of versions that are available in a provider repository.
func (g *gitHubRepository) GetVersions(ctx context.Context) ([]string, error) {
	log := logf.Log

	cacheID := fmt.Sprintf("%s/%s", g.owner, g.repository)
	if versions, ok := cacheVersions[cacheID]; ok {
		return versions, nil
	}

	goProxyClient, err := g.getGoproxyClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get versions client")
	}

	var versions []string
	if goProxyClient != nil {
		// A goproxy is also able to handle the github repository path instead of the actual go module name.
		gomodulePath := path.Join(githubDomain, g.owner, g.repository)

		var parsedVersions semver.Versions
		parsedVersions, err = goProxyClient.GetVersions(ctx, gomodulePath)

		// Log the error before fallback to github repository client happens.
		if err != nil {
			log.V(5).Info("error using Goproxy client to list versions for repository, falling back to github client", "owner", g.owner, "repository", g.repository, "error", err)
		}

		for _, v := range parsedVersions {
			versions = append(versions, "v"+v.String())
		}
	}

	// Fallback to github repository client if goProxyClient is nil or an error occurred.
	if goProxyClient == nil || err != nil {
		versions, err = g.getVersions(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get repository versions")
		}
	}

	cacheVersions[cacheID] = versions
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
func (g *gitHubRepository) GetFile(ctx context.Context, version, path string) ([]byte, error) {
	log := logf.Log

	cacheID := fmt.Sprintf("%s/%s:%s:%s", g.owner, g.repository, version, path)
	if content, ok := cacheFiles[cacheID]; ok {
		return content, nil
	}

	// Try to get the file using http get.
	// NOTE: this can be disabled by setting GORPOXY to `direct` or `off` (same knobs used for skipping goproxy requests).
	if goProxyClient, _ := g.getGoproxyClient(ctx); goProxyClient != nil {
		files, err := g.httpGetFilesFromRelease(ctx, version, path)
		if err != nil {
			log.V(5).Info("error using httpGet to get file from GitHub releases, falling back to github client", "owner", g.owner, "repository", g.repository, "version", version, "path", path, "error", err)
		} else {
			cacheFiles[cacheID] = files
			return files, nil
		}
	}

	// If the http get request failed (or it is disabled) falls back on using the GITHUB api to download the file

	release, err := g.getReleaseByTag(ctx, version)
	if err != nil {
		if errors.Is(err, errNotFound) {
			// If it was ErrNotFound, then there is no release yet for the resolved tag.
			// Ref: https://github.com/kubernetes-sigs/cluster-api/issues/7889
			return nil, errors.Wrapf(err, "release not found for version %s, please retry later or set \"GOPROXY=off\" to get the current stable release", version)
		}
		return nil, errors.Wrapf(err, "failed to get GitHub release %s", version)
	}

	// Download files from the release.
	files, err := g.downloadFilesFromRelease(ctx, release, path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download files from GitHub release %s", version)
	}

	cacheFiles[cacheID] = files
	return files, nil
}

// NewGitHubRepository returns a gitHubRepository implementation.
func NewGitHubRepository(ctx context.Context, providerConfig config.Provider, configVariablesClient config.VariablesClient, opts ...githubRepositoryOption) (Repository, error) {
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

	// Use path's directory as a rootPath.
	rootPath := filepath.Dir(path)
	// Use the file name (if any) as componentsPath.
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

	// Process githubRepositoryOptions.
	for _, o := range opts {
		o(repo)
	}

	if token, err := configVariablesClient.Get(config.GitHubTokenVariable); err == nil {
		repo.setClientToken(ctx, token)
	}

	if defaultVersion == githubLatestReleaseLabel {
		repo.defaultVersion, err = latestContractRelease(ctx, repo, clusterv1.GroupVersion.Version)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get latest release")
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

// getGoproxyClient returns a go proxy client.
// It returns nil, nil if the environment variable is set to `direct` or `off`
// to skip goproxy requests.
func (g *gitHubRepository) getGoproxyClient(_ context.Context) (*goproxy.Client, error) {
	if g.injectGoproxyClient != nil {
		return g.injectGoproxyClient, nil
	}
	scheme, host, err := goproxy.GetSchemeAndHost(os.Getenv("GOPROXY"))
	if err != nil {
		return nil, err
	}
	// Don't return a client if scheme and host is set to empty string.
	if scheme == "" && host == "" {
		return nil, nil
	}
	return goproxy.NewClient(scheme, host), nil
}

// setClientToken sets authenticatingHTTPClient field of gitHubRepository struct.
func (g *gitHubRepository) setClientToken(ctx context.Context, token string) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	g.authenticatingHTTPClient = oauth2.NewClient(ctx, ts)
}

// getVersions returns all the release versions for a github repository.
func (g *gitHubRepository) getVersions(ctx context.Context) ([]string, error) {
	client := g.getClient()

	// Get all the releases.
	// NB. currently Github API does not support result ordering, so it not possible to limit results
	var allReleases []*github.RepositoryRelease
	var retryError error
	_ = wait.PollUntilContextTimeout(ctx, retryableOperationInterval, retryableOperationTimeout, true, func(ctx context.Context) (bool, error) {
		var listReleasesErr error
		// Get the first page of GitHub releases.
		releases, response, listReleasesErr := client.Repositories.ListReleases(ctx, g.owner, g.repository, &github.ListOptions{PerPage: githubListReleasesPerPageLimit})
		if listReleasesErr != nil {
			retryError = g.handleGithubErr(listReleasesErr, "failed to get the list of releases")
			// Return immediately if we are rate limited.
			if _, ok := listReleasesErr.(*github.RateLimitError); ok {
				return false, retryError
			}
			return false, nil
		}
		allReleases = append(allReleases, releases...)

		// Paginated GitHub APIs provide pointers to the first, next, previous and last
		// pages in the response, which can be used to iterate through the pages.
		// https://github.com/google/go-github/blob/14bb610698fc2f9013cad5db79b2d5fe4d53e13c/github/github.go#L541-L551
		for response.NextPage != 0 {
			releases, response, listReleasesErr = client.Repositories.ListReleases(ctx, g.owner, g.repository, &github.ListOptions{Page: response.NextPage, PerPage: githubListReleasesPerPageLimit})
			if listReleasesErr != nil {
				retryError = g.handleGithubErr(listReleasesErr, "failed to get the list of releases")
				// Return immediately if we are rate limited.
				if _, ok := listReleasesErr.(*github.RateLimitError); ok {
					return false, retryError
				}
				return false, nil
			}
			allReleases = append(allReleases, releases...)
		}
		retryError = nil
		return true, nil
	})
	if retryError != nil {
		return nil, retryError
	}
	versions := []string{}
	for _, r := range allReleases {
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

	return versions, nil
}

// getReleaseByTag returns the github repository release with a specific tag name.
func (g *gitHubRepository) getReleaseByTag(ctx context.Context, tag string) (*github.RepositoryRelease, error) {
	cacheID := fmt.Sprintf("%s/%s:%s", g.owner, g.repository, tag)
	if release, ok := cacheReleases[cacheID]; ok {
		return release, nil
	}

	client := g.getClient()

	var release *github.RepositoryRelease
	var retryError error
	_ = wait.PollUntilContextTimeout(ctx, retryableOperationInterval, retryableOperationTimeout, true, func(ctx context.Context) (bool, error) {
		var getReleasesErr error
		release, _, getReleasesErr = client.Repositories.GetReleaseByTag(ctx, g.owner, g.repository, tag)
		if getReleasesErr != nil {
			retryError = g.handleGithubErr(getReleasesErr, "failed to read release %q", tag)
			// Return immediately if not found
			if errors.Is(retryError, errNotFound) {
				return false, retryError
			}
			// Return immediately if we are rate limited.
			if _, ok := getReleasesErr.(*github.RateLimitError); ok {
				return false, retryError
			}
			return false, nil
		}
		retryError = nil
		return true, nil
	})
	if retryError != nil {
		return nil, retryError
	}

	cacheReleases[cacheID] = release
	return release, nil
}

// httpGetFilesFromRelease gets a file from github using http get.
func (g *gitHubRepository) httpGetFilesFromRelease(ctx context.Context, version, fileName string) ([]byte, error) {
	downloadURL := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", g.owner, g.repository, version, fileName)
	var retryError error
	var content []byte
	_ = wait.PollUntilContextTimeout(ctx, retryableOperationInterval, retryableOperationTimeout, true, func(ctx context.Context) (bool, error) {
		resp, err := http.Get(downloadURL) //nolint:gosec,noctx
		if err != nil {
			retryError = errors.Wrap(err, "error sending request")
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			retryError = errors.Errorf("error getting file, status code: %d", resp.StatusCode)
			return false, nil
		}

		content, err = io.ReadAll(resp.Body)
		if err != nil {
			retryError = errors.Wrap(err, "error reading response body")
			return false, nil
		}

		retryError = nil
		return true, nil
	})
	if retryError != nil {
		return nil, retryError
	}
	return content, nil
}

// downloadFilesFromRelease download a file from release.
func (g *gitHubRepository) downloadFilesFromRelease(ctx context.Context, release *github.RepositoryRelease, fileName string) ([]byte, error) {
	client := g.getClient()
	absoluteFileName := filepath.Join(g.rootPath, fileName)

	// Search for the file into the release assets, retrieving the asset id.
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

	var reader io.ReadCloser
	var retryError error
	var content []byte
	_ = wait.PollUntilContextTimeout(ctx, retryableOperationInterval, retryableOperationTimeout, true, func(ctx context.Context) (bool, error) {
		var redirect string
		var downloadReleaseError error
		reader, redirect, downloadReleaseError = client.Repositories.DownloadReleaseAsset(ctx, g.owner, g.repository, *assetID, http.DefaultClient)
		if downloadReleaseError != nil {
			retryError = g.handleGithubErr(downloadReleaseError, "failed to download file %q from %q release", *release.TagName, fileName)
			// Return immediately if we are rate limited.
			if _, ok := downloadReleaseError.(*github.RateLimitError); ok {
				return false, retryError
			}
			return false, nil
		}
		defer reader.Close()

		if redirect != "" {
			// NOTE: DownloadReleaseAsset should not return a redirect address when used with the DefaultClient.
			retryError = errors.New("unexpected redirect while downloading the release asset")
			return true, retryError
		}

		// Read contents from the reader (redirect or not), and return.
		var err error
		content, err = io.ReadAll(reader)
		if err != nil {
			retryError = errors.Wrapf(err, "failed to read downloaded file %q from %q release", *release.TagName, fileName)
			return false, nil
		}

		retryError = nil
		return true, nil
	})
	if retryError != nil {
		return nil, retryError
	}

	return content, nil
}

// handleGithubErr wraps error messages.
func (g *gitHubRepository) handleGithubErr(err error, message string, args ...interface{}) error {
	if _, ok := err.(*github.RateLimitError); ok {
		return errors.New("rate limit for github api has been reached. Please wait one hour or get a personal API token and assign it to the GITHUB_TOKEN environment variable")
	}
	if ghErr, ok := err.(*github.ErrorResponse); ok {
		if ghErr.Response.StatusCode == http.StatusNotFound {
			return errNotFound
		}
	}
	return errors.Wrapf(err, message, args...)
}
