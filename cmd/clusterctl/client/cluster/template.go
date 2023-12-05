/*
Copyright 2020 The Kubernetes Authors.

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

package cluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/go-github/v53/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
)

// TemplateClient has methods to work with templates stored in the cluster/out of the provider repository.
type TemplateClient interface {
	// GetFromConfigMap returns a workload cluster template from the given ConfigMap.
	GetFromConfigMap(ctx context.Context, namespace, name, dataKey, targetNamespace string, skipTemplateProcess bool) (repository.Template, error)

	// GetFromURL returns a workload cluster template from the given URL.
	GetFromURL(ctx context.Context, templateURL, targetNamespace string, skipTemplateProcess bool) (repository.Template, error)
}

// templateClient implements TemplateClient.
type templateClient struct {
	proxy               Proxy
	configClient        config.Client
	gitHubClientFactory func(ctx context.Context, configVariablesClient config.VariablesClient) (*github.Client, error)
	processor           yaml.Processor
	httpClient          *http.Client
}

// ensure templateClient implements TemplateClient.
var _ TemplateClient = &templateClient{}

// TemplateClientInput is an input struct for newTemplateClient.
type TemplateClientInput struct {
	proxy        Proxy
	configClient config.Client
	processor    yaml.Processor
}

// newTemplateClient returns a templateClient.
func newTemplateClient(input TemplateClientInput) *templateClient {
	return &templateClient{
		proxy:               input.proxy,
		configClient:        input.configClient,
		gitHubClientFactory: getGitHubClient,
		processor:           input.processor,
		httpClient:          http.DefaultClient,
	}
}

func (t *templateClient) GetFromConfigMap(ctx context.Context, configMapNamespace, configMapName, configMapDataKey, targetNamespace string, skipTemplateProcess bool) (repository.Template, error) {
	if configMapNamespace == "" {
		return nil, errors.New("invalid GetFromConfigMap operation: missing configMapNamespace value")
	}
	if configMapName == "" {
		return nil, errors.New("invalid GetFromConfigMap operation: missing configMapName value")
	}

	c, err := t.proxy.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	configMap := &corev1.ConfigMap{}
	key := client.ObjectKey{
		Namespace: configMapNamespace,
		Name:      configMapName,
	}

	if err := c.Get(ctx, key, configMap); err != nil {
		return nil, errors.Wrapf(err, "error reading ConfigMap %s/%s", configMapNamespace, configMapName)
	}

	data, ok := configMap.Data[configMapDataKey]
	if !ok {
		return nil, errors.Errorf("the ConfigMap %s/%s does not have the %q data key", configMapNamespace, configMapName, configMapDataKey)
	}

	return repository.NewTemplate(repository.TemplateInput{
		RawArtifact:           []byte(data),
		ConfigVariablesClient: t.configClient.Variables(),
		Processor:             t.processor,
		TargetNamespace:       targetNamespace,
		SkipTemplateProcess:   skipTemplateProcess,
	})
}

func (t *templateClient) GetFromURL(ctx context.Context, templateURL, targetNamespace string, skipTemplateProcess bool) (repository.Template, error) {
	if templateURL == "" {
		return nil, errors.New("invalid GetFromURL operation: missing templateURL value")
	}

	content, err := t.getURLContent(ctx, templateURL)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid GetFromURL operation")
	}

	return repository.NewTemplate(repository.TemplateInput{
		RawArtifact:           content,
		ConfigVariablesClient: t.configClient.Variables(),
		Processor:             t.processor,
		TargetNamespace:       targetNamespace,
		SkipTemplateProcess:   skipTemplateProcess,
	})
}

func (t *templateClient) getURLContent(ctx context.Context, templateURL string) ([]byte, error) {
	if templateURL == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read stdin")
		}
		return b, nil
	}

	rURL, err := url.Parse(templateURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q", templateURL)
	}

	if rURL.Scheme == "https" {
		if rURL.Host == "github.com" {
			return t.getGitHubFileContent(ctx, rURL)
		}
		return t.getRawURLFileContent(ctx, templateURL)
	}

	if rURL.Scheme == "file" || rURL.Scheme == "" {
		return t.getLocalFileContent(rURL)
	}

	return nil, errors.Errorf("unable to read content from %q. Only reading from GitHub and local file system is supported", templateURL)
}

func (t *templateClient) getLocalFileContent(rURL *url.URL) ([]byte, error) {
	f, err := os.Stat(rURL.Path)
	if err != nil {
		return nil, errors.Errorf("failed to read file %q", rURL.Path)
	}
	if f.IsDir() {
		return nil, errors.Errorf("invalid path: file %q is actually a directory", rURL.Path)
	}
	content, err := os.ReadFile(rURL.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read file %q", rURL.Path)
	}

	return content, nil
}

func (t *templateClient) getGitHubFileContent(ctx context.Context, rURL *url.URL) ([]byte, error) {
	// Check if the path is in the expected format,
	urlSplit := strings.Split(strings.TrimPrefix(rURL.Path, "/"), "/")
	if len(urlSplit) < 5 {
		return nil, errors.Errorf(
			"invalid GitHub url %q: a GitHub url should be in on of these the forms\n"+
				"- https://github.com/{owner}/{repository}/blob/{branch}/{path-to-file}\n"+
				"- https://github.com/{owner}/{repository}/releases/download/{tag}/{asset-file-name}", rURL,
		)
	}

	// Extract all the info from url split.
	owner := urlSplit[0]
	repo := urlSplit[1]
	linkType := urlSplit[2]

	// gets the GitHub client
	ghClient, err := t.gitHubClientFactory(ctx, t.configClient.Variables())
	if err != nil {
		return nil, err
	}

	// gets the file from GiHub
	switch linkType {
	case "blob": // get file from a code in a github repo
		branch := urlSplit[3]
		path := strings.Join(urlSplit[4:], "/")

		return getGithubFileContentFromCode(ctx, ghClient, rURL.Path, owner, repo, path, branch)

	case "releases": // get a github release asset
		if urlSplit[3] != "download" {
			break
		}
		tag := urlSplit[4]
		assetName := urlSplit[5]

		return getGithubAssetFromRelease(ctx, ghClient, rURL.Path, owner, repo, tag, assetName)
	}

	return nil, fmt.Errorf("unknown github URL: %v", rURL)
}

func getGithubFileContentFromCode(ctx context.Context, ghClient *github.Client, fullPath string, owner string, repo string, path string, branch string) ([]byte, error) {
	fileContent, _, _, err := ghClient.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{Ref: branch})
	if err != nil {
		return nil, handleGithubErr(err, "failed to get %q", fullPath)
	}
	if fileContent == nil {
		return nil, errors.Errorf("%q does not return a valid file content", fullPath)
	}
	if fileContent.Encoding == nil || *fileContent.Encoding != "base64" {
		return nil, errors.Errorf("invalid encoding detected for %q. Only base64 encoding supported", fullPath)
	}
	content, err := base64.StdEncoding.DecodeString(*fileContent.Content)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode file %q", fullPath)
	}
	return content, nil
}

func (t *templateClient) getRawURLFileContent(ctx context.Context, rURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	response, err := t.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to get file, got %d", response.StatusCode)
	}

	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func getGithubAssetFromRelease(ctx context.Context, ghClient *github.Client, path string, owner string, repo string, tag string, assetName string) ([]byte, error) {
	release, _, err := ghClient.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, handleGithubErr(err, "failed to get release '%s' from %s/%s repository", tag, owner, repo)
	}

	if release == nil {
		return nil, fmt.Errorf("can't find release '%s' in %s/%s repository", tag, owner, repo)
	}

	var rc io.ReadCloser
	for _, asset := range release.Assets {
		if asset.GetName() == assetName {
			rc, _, err = ghClient.Repositories.DownloadReleaseAsset(ctx, owner, repo, asset.GetID(), ghClient.Client())
			if err != nil {
				return nil, errors.Wrapf(err, "failed to download file %q", path)
			}
			break
		}
	}

	if rc == nil {
		return nil, fmt.Errorf("failed to download the file %q", path)
	}

	defer func() { _ = rc.Close() }()

	return io.ReadAll(rc)
}

func getGitHubClient(ctx context.Context, configVariablesClient config.VariablesClient) (*github.Client, error) {
	var authenticatingHTTPClient *http.Client
	if token, err := configVariablesClient.Get(config.GitHubTokenVariable); err == nil {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		authenticatingHTTPClient = oauth2.NewClient(ctx, ts)
	}

	return github.NewClient(authenticatingHTTPClient), nil
}

// handleGithubErr wraps error messages.
func handleGithubErr(err error, message string, args ...interface{}) error {
	if _, ok := err.(*github.RateLimitError); ok {
		return errors.New("rate limit for github api has been reached. Please wait one hour or get a personal API token and assign it to the GITHUB_TOKEN environment variable")
	}
	return errors.Wrapf(err, message, args...)
}
