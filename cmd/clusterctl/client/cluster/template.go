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
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/go-github/v33/github"
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
	GetFromConfigMap(namespace, name, dataKey, targetNamespace string, skipTemplateProcess bool) (repository.Template, error)

	// GetFromURL returns a workload cluster template from the given URL.
	GetFromURL(templateURL, targetNamespace string, skipTemplateProcess bool) (repository.Template, error)
}

// templateClient implements TemplateClient.
type templateClient struct {
	proxy               Proxy
	configClient        config.Client
	gitHubClientFactory func(configVariablesClient config.VariablesClient) (*github.Client, error)
	processor           yaml.Processor
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
	}
}

func (t *templateClient) GetFromConfigMap(configMapNamespace, configMapName, configMapDataKey, targetNamespace string, skipTemplateProcess bool) (repository.Template, error) {
	if configMapNamespace == "" {
		return nil, errors.New("invalid GetFromConfigMap operation: missing configMapNamespace value")
	}
	if configMapName == "" {
		return nil, errors.New("invalid GetFromConfigMap operation: missing configMapName value")
	}

	c, err := t.proxy.NewClient()
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

func (t *templateClient) GetFromURL(templateURL, targetNamespace string, skipTemplateProcess bool) (repository.Template, error) {
	if templateURL == "" {
		return nil, errors.New("invalid GetFromURL operation: missing templateURL value")
	}

	content, err := t.getURLContent(templateURL)
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

func (t *templateClient) getURLContent(templateURL string) ([]byte, error) {
	rURL, err := url.Parse(templateURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q", templateURL)
	}

	if rURL.Scheme == "https" && rURL.Host == "github.com" {
		return t.getGitHubFileContent(rURL)
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

func (t *templateClient) getGitHubFileContent(rURL *url.URL) ([]byte, error) {
	// Check if the path is in the expected format,
	urlSplit := strings.Split(strings.TrimPrefix(rURL.Path, "/"), "/")
	if len(urlSplit) < 5 {
		return nil, errors.Errorf(
			"invalid GitHub url %q: a GitHub url should be in the form https://github.com/{owner}/{repository}/blob/{branch}/{path-to-file}", rURL,
		)
	}

	// Extract all the info from url split.
	owner := urlSplit[0]
	repository := urlSplit[1]
	branch := urlSplit[3]
	path := strings.Join(urlSplit[4:], "/")

	// gets the GitHub client
	client, err := t.gitHubClientFactory(t.configClient.Variables())
	if err != nil {
		return nil, err
	}

	// gets the file from GiHub
	fileContent, _, _, err := client.Repositories.GetContents(context.TODO(), owner, repository, path, &github.RepositoryContentGetOptions{Ref: branch})
	if err != nil {
		return nil, handleGithubErr(err, "failed to get %q", rURL.Path)
	}
	if fileContent == nil {
		return nil, errors.Errorf("%q does not return a valid file content", rURL.Path)
	}
	if fileContent.Encoding == nil || *fileContent.Encoding != "base64" {
		return nil, errors.Errorf("invalid encoding detected for %q. Only base64 encoding supported", rURL.Path)
	}

	content, err := base64.StdEncoding.DecodeString(*fileContent.Content)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode file %q", rURL.Path)
	}
	return content, nil
}

func getGitHubClient(configVariablesClient config.VariablesClient) (*github.Client, error) {
	var authenticatingHTTPClient *http.Client
	if token, err := configVariablesClient.Get(config.GitHubTokenVariable); err == nil {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		authenticatingHTTPClient = oauth2.NewClient(context.TODO(), ts)
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
