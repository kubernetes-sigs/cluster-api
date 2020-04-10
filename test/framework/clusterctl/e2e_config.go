// +build e2e

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

package clusterctl

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"time"

	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	clusterctlconfig "sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/yaml"
)

// Provides access to the configuration for an e2e test.

// LoadE2EConfigInput is the input for LoadE2EConfig.
type LoadE2EConfigInput struct {
	// ConfigPath for the e2e test.
	ConfigPath string
}

// LoadE2EConfig loads the configuration for the e2e test environment.
func LoadE2EConfig(ctx context.Context, input LoadE2EConfigInput) *E2EConfig {
	configData, err := ioutil.ReadFile(input.ConfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to read the e2e test config file")
	Expect(configData).ToNot(BeEmpty(), "The e2e test config file should not be empty")

	config := &E2EConfig{}
	Expect(yaml.Unmarshal(configData, config)).To(Succeed(), "Failed to convert the e2e test config file to yaml")

	config.Defaults()
	config.AbsPaths(filepath.Dir(input.ConfigPath))

	Expect(config.Validate()).To(Succeed(), "The e2e test config file is not valid")

	return config
}

// E2EConfig defines the configuration of an e2e test environment.
type E2EConfig struct {
	// Name is the name of the Kind management cluster.
	// Defaults to test-[random generated suffix].
	ManagementClusterName string `json:"managementClusterName,omitempty"`

	// Images is a list of container images to load into the Kind cluster.
	Images []framework.ContainerImage `json:"images,omitempty"`

	// Providers is a list of providers to be configured in the local repository that will be created for the e2e test.
	// It is required to provide following providers
	// - cluster-api
	// - bootstrap kubeadm
	// - control-plane kubeadm
	// - one infrastructure provider
	// The test will adapt to the selected infrastructure provider
	Providers []ProviderConfig `json:"providers,omitempty"`

	// Variables to be added to the clusterctl config file
	// Please note that clusterctl read variables from OS environment variables as well, so you can avoid to hard code
	// sensitive data in the config file.
	Variables map[string]string `json:"variables,omitempty"`

	// Intervals to be used for long operations during tests
	Intervals map[string][]string `json:"intervals,omitempty"`
}

// ProviderConfig describes a provider to be configured in the local repository that will be created for the e2e test.
type ProviderConfig struct {
	// Name is the name of the provider.
	Name string `json:"name"`

	// Type is the type of the provider.
	Type string `json:"type"`

	// Versions is a list of component YAML to be added to the local repository, one for each release.
	// Please note that the first source will be used a a default release for this provider.
	Versions []framework.ComponentSource `json:"versions,omitempty"`

	// Files is a list of test files to be copied into the local repository for the default release of this provider.
	Files []Files `json:"files,omitempty"`
}

// Files contains information about files to be copied into the local repository
type Files struct {
	// SourcePath path of the file.
	SourcePath string `json:"sourcePath"`

	// TargetName name of the file copied into the local repository. if empty, the source name
	// Will be preserved
	TargetName string `json:"targetName,omitempty"`
}

// Defaults assigns default values to the object. More specifically:
// - ManagementClusterName gets a default name if empty.
// - Providers version gets type KustomizeSource if not otherwise specified.
// - Providers file gets targetName = sourceName if not otherwise specified.
// - Images gets LoadBehavior = MustLoadImage if not otherwise specified.
func (c *E2EConfig) Defaults() {
	if c.ManagementClusterName == "" {
		c.ManagementClusterName = fmt.Sprintf("test-%s", util.RandomString(6))
	}
	for i := range c.Providers {
		provider := &c.Providers[i]
		for j := range provider.Versions {
			version := &provider.Versions[j]
			if version.Type == "" {
				version.Type = framework.KustomizeSource
			}
		}
		for j := range provider.Files {
			file := &provider.Files[j]
			if file.SourcePath != "" && file.TargetName == "" {
				file.TargetName = filepath.Base(file.SourcePath)
			}
		}
	}
	for i := range c.Images {
		containerImage := &c.Images[i]
		if containerImage.LoadBehavior == "" {
			containerImage.LoadBehavior = framework.MustLoadImage
		}
	}
}

// AbsPaths makes relative paths absolute using the give base path.
func (c *E2EConfig) AbsPaths(basePath string) {
	for i := range c.Providers {
		provider := &c.Providers[i]
		for j := range provider.Versions {
			version := &provider.Versions[j]
			if version.Value != "" {
				if !filepath.IsAbs(version.Value) {
					version.Value = filepath.Join(basePath, version.Value)
				}
			}
		}
		for j := range provider.Files {
			file := &provider.Files[j]
			if file.SourcePath != "" {
				if !filepath.IsAbs(file.SourcePath) {
					file.SourcePath = filepath.Join(basePath, file.SourcePath)
				}
			}
		}
	}
}

func errInvalidArg(format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return errors.Errorf("invalid argument: %s", msg)
}

func errEmptyArg(argName string) error {
	return errInvalidArg("%s is empty", argName)
}

// Validate validates the configuration. More specifically:
// - ManagementClusterName should not be empty.
// - Providers name should not be empty.
// - Providers type should be one of [CoreProvider, BootstrapProvider, ControlPlaneProvider, InfrastructureProvider].
// - Providers version should have a name.
// - Providers version.type should be one of [url, kustomize].
// - Providers version.replacements.old should be a valid regex.
// - Providers files should be an existing file and have a target name.
// - There should be one CoreProvider (cluster-api), one BootstrapProvider (kubeadm), one ControlPlaneProvider (kubeadm).
// - There should be one InfraProvider (pick your own).
// - Image should have name and loadBehavior be one of [mustload, tryload].
// - Intervals should be valid ginkgo intervals.
func (c *E2EConfig) Validate() error {
	// ManagementClusterName should not be empty.
	if c.ManagementClusterName == "" {
		return errEmptyArg("ManagementClusterName")
	}

	providersByType := map[clusterctlv1.ProviderType][]string{
		clusterctlv1.CoreProviderType:           nil,
		clusterctlv1.BootstrapProviderType:      nil,
		clusterctlv1.ControlPlaneProviderType:   nil,
		clusterctlv1.InfrastructureProviderType: nil,
	}
	for i, providerConfig := range c.Providers {
		// Providers name should not be empty.
		if providerConfig.Name == "" {
			return errEmptyArg(fmt.Sprintf("Providers[%d].Name", i))
		}
		// Providers type should be one of [CoreProvider, BootstrapProvider, ControlPlaneProvider, InfrastructureProvider].
		providerType := clusterctlv1.ProviderType(providerConfig.Type)
		switch providerType {
		case clusterctlv1.CoreProviderType, clusterctlv1.BootstrapProviderType, clusterctlv1.ControlPlaneProviderType, clusterctlv1.InfrastructureProviderType:
			providersByType[providerType] = append(providersByType[providerType], providerConfig.Name)
		default:
			return errInvalidArg("Providers[%d].Type=%q", i, providerConfig.Type)
		}

		// Providers version should have a name.
		// Providers version.type should be one of [url, kustomize].
		// Providers version.replacements.old should be a valid regex.
		for j, version := range providerConfig.Versions {
			if version.Name == "" {
				return errEmptyArg(fmt.Sprintf("Providers[%d].Sources[%d].Name", i, j))
			}
			//TODO: check if name is a valid semantic version
			switch version.Type {
			case framework.URLSource, framework.KustomizeSource:
				if version.Value == "" {
					return errEmptyArg(fmt.Sprintf("Providers[%d].Sources[%d].Value", i, j))
				}
			default:
				return errInvalidArg("Providers[%d].Sources[%d].Type=%q", i, j, version.Type)
			}
			for k, replacement := range version.Replacements {
				if _, err := regexp.Compile(replacement.Old); err != nil {
					return errInvalidArg("Providers[%d].Sources[%d].Replacements[%d].Old=%q: %v", i, j, k, replacement.Old, err)
				}
			}
		}

		// Providers files should be an existing file and have a target name.
		for j, file := range providerConfig.Files {
			if file.SourcePath == "" {
				return errInvalidArg("Providers[%d].Files[%d].SourcePath=%q", i, j, file.SourcePath)
			}
			if !fileExists(file.SourcePath) {
				return errInvalidArg("Providers[%d].Files[%d].SourcePath=%q", i, j, file.SourcePath)
			}
			if file.TargetName == "" {
				return errInvalidArg("Providers[%d].Files[%d].TargetName=%q", i, j, file.TargetName)
			}
		}
	}

	// There should be one CoreProvider (cluster-api), one BootstrapProvider (kubeadm), one ControlPlaneProvider (kubeadm).
	if len(providersByType[clusterctlv1.CoreProviderType]) != 1 {
		return errInvalidArg("invalid config: it is required to have exactly one core-provider")
	}
	if providersByType[clusterctlv1.CoreProviderType][0] != clusterctlconfig.ClusterAPIProviderName {
		return errInvalidArg("invalid config: core-provider should be named %s", clusterctlconfig.ClusterAPIProviderName)
	}

	if len(providersByType[clusterctlv1.BootstrapProviderType]) != 1 {
		return errInvalidArg("invalid config: it is required to have exactly one bootstrap-provider")
	}
	if providersByType[clusterctlv1.BootstrapProviderType][0] != clusterctlconfig.KubeadmBootstrapProviderName {
		return errInvalidArg("invalid config: bootstrap-provider should be named %s", clusterctlconfig.KubeadmBootstrapProviderName)
	}

	if len(providersByType[clusterctlv1.ControlPlaneProviderType]) != 1 {
		return errInvalidArg("invalid config: it is required to have exactly one control-plane-provider")
	}
	if providersByType[clusterctlv1.ControlPlaneProviderType][0] != clusterctlconfig.KubeadmControlPlaneProviderName {
		return errInvalidArg("invalid config: control-plane-provider should be named %s", clusterctlconfig.KubeadmControlPlaneProviderName)
	}

	// There should be one InfraProvider (pick your own).
	if len(providersByType[clusterctlv1.InfrastructureProviderType]) != 1 {
		return errInvalidArg("invalid config: it is required to have exactly one infrastructure-provider")
	}

	// Image should have name and loadBehavior be one of [mustload, tryload].
	for i, containerImage := range c.Images {
		if containerImage.Name == "" {
			return errEmptyArg(fmt.Sprintf("Images[%d].Name=%q", i, containerImage.Name))
		}
		switch containerImage.LoadBehavior {
		case framework.MustLoadImage, framework.TryLoadImage:
			// Valid
		default:
			return errInvalidArg("Images[%d].LoadBehavior=%q", i, containerImage.LoadBehavior)
		}
	}

	// Intervals should be valid ginkgo intervals.
	for k, intervals := range c.Intervals {
		switch len(intervals) {
		case 0:
			return errInvalidArg("Intervals[%s]=%q", k, intervals)
		case 1, 2:
		default:
			return errInvalidArg("Intervals[%s]=%q", k, intervals)
		}
		for _, i := range intervals {
			if _, err := time.ParseDuration(i); err != nil {
				return errInvalidArg("Intervals[%s]=%q", k, intervals)
			}
		}
	}

	return nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// InfraProvider returns the infrastructure provider selected for running this E2E test.
func (c *E2EConfig) InfraProviders() []string {
	InfraProviders := []string{}
	for _, provider := range c.Providers {
		if provider.Type == string(clusterctlv1.InfrastructureProviderType) {
			InfraProviders = append(InfraProviders, provider.Name)
		}
	}
	return InfraProviders
}

// GetIntervals returns the intervals to be applied to a Eventually operation.
// It searches for [spec]/[key] intervals first, and if it is not found, it searches
// for default/[key]. If also the default/[key] intervals are not found,
// ginkgo DefaultEventuallyTimeout and DefaultEventuallyPollingInterval are used.
func (c *E2EConfig) GetIntervals(spec, key string) []interface{} {
	intervals, ok := c.Intervals[fmt.Sprintf("%s/%s", spec, key)]
	if !ok {
		if intervals, ok = c.Intervals[fmt.Sprintf("default/%s", key)]; !ok {
			return nil
		}
	}
	intervalsInterfaces := make([]interface{}, len(intervals))
	for i := range intervals {
		intervalsInterfaces[i] = intervals[i]
	}
	return intervalsInterfaces
}
