/*
Copyright 2022 The Kubernetes Authors.

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
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
)

const (
	gitlabHostPrefix          = "gitlab."
	gitlabPackagesAPIPrefix   = "/api/v4/projects/"
	gitlabPackagesAPIPackages = "packages"
	gitlabPackagesAPIGeneric  = "generic"
)

// gitLabRepository provides support for providers hosted on GitLab.
//
// We support GitLab repositories that use the generic packages feature to publish artifacts and versions.
// Repositories must use versioned releases.
type gitLabRepository struct {
	providerConfig           config.Provider
	configVariablesClient    config.VariablesClient
	authenticatingHTTPClient *http.Client
	host                     string
	projectSlug              string
	packageName              string
	defaultVersion           string
	rootPath                 string
	componentsPath           string
}

var _ Repository = &gitLabRepository{}

// NewGitLabRepository returns a gitLabRepository implementation.
func NewGitLabRepository(ctx context.Context, providerConfig config.Provider, configVariablesClient config.VariablesClient) (Repository, error) {
	if configVariablesClient == nil {
		return nil, errors.New("invalid arguments: configVariablesClient can't be nil")
	}

	rURL, err := url.Parse(providerConfig.URL())
	if err != nil {
		return nil, errors.Wrap(err, "invalid url")
	}

	urlSplit := strings.Split(strings.TrimPrefix(rURL.EscapedPath(), "/"), "/")

	// Check if the url is a Gitlab repository
	if rURL.Scheme != httpsScheme ||
		len(urlSplit) != 9 ||
		!strings.HasPrefix(rURL.EscapedPath(), gitlabPackagesAPIPrefix) ||
		urlSplit[4] != gitlabPackagesAPIPackages ||
		urlSplit[5] != gitlabPackagesAPIGeneric {
		return nil, errors.New("invalid url: a GitLab repository url should be in the form https://{host}/api/v4/projects/{projectSlug}/packages/generic/{packageName}/{defaultVersion}/{componentsPath}")
	}

	httpClient := http.DefaultClient
	// Extract all the info from url split.
	host := rURL.Host
	projectSlug := urlSplit[3]
	packageName := urlSplit[6]
	defaultVersion := urlSplit[7]
	rootPath := "."
	componentsPath := urlSplit[8]

	repo := &gitLabRepository{
		providerConfig:           providerConfig,
		configVariablesClient:    configVariablesClient,
		authenticatingHTTPClient: httpClient,
		host:                     host,
		projectSlug:              projectSlug,
		packageName:              packageName,
		defaultVersion:           defaultVersion,
		rootPath:                 rootPath,
		componentsPath:           componentsPath,
	}
	if token, err := configVariablesClient.Get(config.GitLabAccessTokenVariable); err == nil {
		repo.setClientToken(ctx, token)
	}

	return repo, nil
}

// setClientToken sets authenticatingHTTPClient field of gitLabRepository struct.
func (g *gitLabRepository) setClientToken(ctx context.Context, token string) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token, TokenType: "Bearer"},
	)
	g.authenticatingHTTPClient = oauth2.NewClient(ctx, ts)
}

// Host returns host field of gitLabRepository struct.
func (g *gitLabRepository) Host() string {
	return g.host
}

// ProjectSlug returns projectSlug field of gitLabRepository struct.
func (g *gitLabRepository) ProjectSlug() string {
	return g.projectSlug
}

// DefaultVersion returns defaultVersion field of gitLabRepository struct.
func (g *gitLabRepository) DefaultVersion() string {
	return g.defaultVersion
}

// GetVersions returns the list of versions that are available in a provider repository.
func (g *gitLabRepository) GetVersions(_ context.Context) ([]string, error) {
	// TODO Get versions from GitLab API
	return []string{g.defaultVersion}, nil
}

// RootPath returns rootPath field of gitLabRepository struct.
func (g *gitLabRepository) RootPath() string {
	return g.rootPath
}

// ComponentsPath returns componentsPath field of gitLabRepository struct.
func (g *gitLabRepository) ComponentsPath() string {
	return g.componentsPath
}

// GetFile returns a file for a given provider version.
func (g *gitLabRepository) GetFile(ctx context.Context, version, path string) ([]byte, error) {
	url := fmt.Sprintf(
		"https://%s/api/v4/projects/%s/packages/generic/%s/%s/%s",
		g.host,
		g.projectSlug,
		g.packageName,
		version,
		path,
	)

	if content, ok := cacheFiles[url]; ok {
		return content, nil
	}

	timeoutctx, cancel := context.WithTimeoutCause(ctx, 30*time.Second, errors.New("http request timeout expired"))
	defer cancel()
	request, err := http.NewRequestWithContext(timeoutctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get file %q with version %q from %q: failed to create request", path, version, url)
	}

	response, err := g.authenticatingHTTPClient.Do(request)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get file %q with version %q from %q", path, version, url)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		// explicitly check for 401 and return a more specific error
		if response.StatusCode == http.StatusUnauthorized {
			return nil, errors.Errorf("failed to get file %q with version %q from %q: unauthorized access, please check your credentials", path, version, url)
		}
		return nil, errors.Errorf("failed to get file %q with version %q from %q, got %d", path, version, url, response.StatusCode)
	}

	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get file %q with version %q from %q", path, version, url)
	}

	cacheFiles[url] = content
	return content, nil
}
