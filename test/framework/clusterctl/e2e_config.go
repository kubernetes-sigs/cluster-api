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
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	clusterctlconfig "sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/util"
)

// Provides access to the configuration for an e2e test.

// LoadE2EConfigInput is the input for LoadE2EConfig.
type LoadE2EConfigInput struct {
	// ConfigPath for the e2e test.
	ConfigPath string
}

// LoadE2EConfig loads the configuration for the e2e test environment.
func LoadE2EConfig(ctx context.Context, input LoadE2EConfigInput) *E2EConfig {
	configData, err := os.ReadFile(input.ConfigPath)
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
	Images []ContainerImage `json:"images,omitempty"`

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
	Versions []ProviderVersionSource `json:"versions,omitempty"`

	// Files is a list of files to be copied into the local repository for all the releases.
	Files []Files `json:"files,omitempty"`
}

// LoadImageBehavior indicates the behavior when loading an image.
type LoadImageBehavior string

const (
	// MustLoadImage causes a load operation to fail if the image cannot be
	// loaded.
	MustLoadImage LoadImageBehavior = "mustLoad"

	// TryLoadImage causes any errors that occur when loading an image to be
	// ignored.
	TryLoadImage LoadImageBehavior = "tryLoad"
)

// ContainerImage describes an image to load into a cluster and the behavior
// when loading the image.
type ContainerImage struct {
	// Name is the fully qualified name of the image.
	Name string

	// LoadBehavior may be used to dictate whether a failed load operation
	// should fail the test run. This is useful when wanting to load images
	// *if* they exist locally, but not wanting to fail if they don't.
	//
	// Defaults to MustLoadImage.
	LoadBehavior LoadImageBehavior
}

// ComponentSourceType indicates how a component's source should be obtained.
type ComponentSourceType string

const (
	// URLSource is component YAML available directly via a URL.
	// The URL may begin with http://, https:// or file://(can be omitted, relative paths supported).
	URLSource ComponentSourceType = "url"

	// KustomizeSource is a valid kustomization root that can be used to produce
	// the component YAML.
	KustomizeSource ComponentSourceType = "kustomize"
)

// ProviderVersionSource describes how to obtain a component's YAML.
type ProviderVersionSource struct {
	// Name is used for logging when a component has multiple sources.
	Name string `json:"name,omitempty"`

	// Value is the source of the component's YAML.
	// May be a URL or a kustomization root (specified by Type).
	// If a Type=url then Value may begin with file://, http://, or https://.
	// If a Type=kustomize then Value may be any valid go-getter URL. For
	// more information please see https://github.com/hashicorp/go-getter#url-format.
	Value string `json:"value"`

	// Contract defines the Cluster API contract version a specific version of the provider abides to.
	Contract string `json:"contract,omitempty"`

	// Type describes how to process the source of the component's YAML.
	//
	// Defaults to "kustomize".
	Type ComponentSourceType `json:"type,omitempty"`

	// Replacements is a list of patterns to replace in the component YAML
	// prior to application.
	Replacements []ComponentReplacement `json:"replacements,omitempty"`

	// Files is a list of files to be copied into the local repository for this release.
	Files []Files `json:"files,omitempty"`
}

// ComponentWaiterType indicates the type of check to use to determine if the
// installed components are ready.
type ComponentWaiterType string

const (
	// ServiceWaiter indicates to wait until a service's condition is Available.
	// When ComponentWaiter.Value is set to "service", the ComponentWaiter.Value
	// should be set to the name of a Service resource.
	ServiceWaiter ComponentWaiterType = "service"

	// PodsWaiter indicates to wait until all the pods in a namespace have a
	// condition of Ready.
	// When ComponentWaiter.Value is set to "pods", the ComponentWaiter.Value
	// should be set to the name of a Namespace resource.
	PodsWaiter ComponentWaiterType = "pods"
)

// ComponentWaiter contains information to help determine whether installed
// components are ready.
type ComponentWaiter struct {
	// Value varies depending on the specified Type.
	// Please see the documentation for the different WaiterType constants to
	// understand the valid values for this field.
	Value string `json:"value"`

	// Type describes the type of check to perform.
	//
	// Defaults to "pods".
	Type ComponentWaiterType `json:"type,omitempty"`
}

// ComponentReplacement is used to replace some of the generated YAML prior
// to application.
type ComponentReplacement struct {
	// Old is the pattern to replace.
	// A regular expression may be used.
	Old string `json:"old"`
	// New is the string used to replace the old pattern.
	// An empty string is valid.
	New string `json:"new,omitempty"`
}

// ComponentConfig describes a component required by the e2e test environment.
type ComponentConfig struct {
	// Name is the name of the component.
	// This field is primarily used for logging.
	Name string `json:"name"`

	// Sources is an optional list of component YAML to apply to the management
	// cluster.
	// This field may be omitted when wanting only to block progress via one or
	// more Waiters.
	Sources []ProviderVersionSource `json:"sources,omitempty"`

	// Waiters is an optional list of checks to perform in order to determine
	// whether or not the installed components are ready.
	Waiters []ComponentWaiter `json:"waiters,omitempty"`
}

// Files contains information about files to be copied into the local repository.
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
				version.Type = KustomizeSource
			}
			for j := range version.Files {
				file := &version.Files[j]
				if file.SourcePath != "" && file.TargetName == "" {
					file.TargetName = filepath.Base(file.SourcePath)
				}
			}
		}
		for j := range provider.Files {
			file := &provider.Files[j]
			if file.SourcePath != "" && file.TargetName == "" {
				file.TargetName = filepath.Base(file.SourcePath)
			}
		}
	}
	imageReplacer := strings.NewReplacer("{OS}", runtime.GOOS, "{ARCH}", runtime.GOARCH)
	for i := range c.Images {
		containerImage := &c.Images[i]
		containerImage.Name = imageReplacer.Replace(containerImage.Name)
		if containerImage.LoadBehavior == "" {
			containerImage.LoadBehavior = MustLoadImage
		}
	}
}

// AbsPaths makes relative paths absolute using the given base path.
func (c *E2EConfig) AbsPaths(basePath string) {
	for i := range c.Providers {
		provider := &c.Providers[i]
		for j := range provider.Versions {
			version := &provider.Versions[j]
			if version.Type != URLSource && version.Value != "" {
				if !filepath.IsAbs(version.Value) {
					version.Value = filepath.Join(basePath, version.Value)
				}
			} else if version.Type == URLSource && version.Value != "" {
				// Skip error, will be checked later when loading contents from URL
				u, _ := url.Parse(version.Value)

				if u != nil {
					switch u.Scheme {
					case "", fileURIScheme:
						fp := strings.TrimPrefix(version.Value, fmt.Sprintf("%s://", fileURIScheme))
						if !filepath.IsAbs(fp) {
							version.Value = filepath.Join(basePath, fp)
						}
					}
				}
			}

			for j := range version.Files {
				file := &version.Files[j]
				if file.SourcePath != "" {
					if !filepath.IsAbs(file.SourcePath) {
						file.SourcePath = filepath.Join(basePath, file.SourcePath)
					}
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
// - There should be one CoreProvider (cluster-api), one BootstrapProvider (kubeadm), one ControlPlaneProvider (kubeadm).
// - There should be one InfraProvider (pick your own).
// - Image should have name and loadBehavior be one of [mustload, tryload].
// - Intervals should be valid ginkgo intervals.
func (c *E2EConfig) Validate() error {
	// ManagementClusterName should not be empty.
	if c.ManagementClusterName == "" {
		return errEmptyArg("ManagementClusterName")
	}

	if err := c.validateProviders(); err != nil {
		return err
	}

	// Image should have name and loadBehavior be one of [mustload, tryload].
	for i, containerImage := range c.Images {
		if containerImage.Name == "" {
			return errEmptyArg(fmt.Sprintf("Images[%d].Name=%q", i, containerImage.Name))
		}
		switch containerImage.LoadBehavior {
		case MustLoadImage, TryLoadImage:
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

// validateProviders validates the provider configuration. More specifically:
// - Providers name should not be empty.
// - Providers type should be one of [CoreProvider, BootstrapProvider, ControlPlaneProvider, InfrastructureProvider].
// - Providers version should have a name.
// - Providers version.type should be one of [url, kustomize].
// - Providers version.replacements.old should be a valid regex.
// - Providers files should be an existing file and have a target name.
func (c *E2EConfig) validateProviders() error {
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

		// Providers providerVersion should have a name.
		// Providers providerVersion.type should be one of [url, kustomize].
		// Providers providerVersion.replacements.old should be a valid regex.
		for j, providerVersion := range providerConfig.Versions {
			if providerVersion.Name == "" {
				return errEmptyArg(fmt.Sprintf("Providers[%d].Sources[%d].Name", i, j))
			}
			if _, err := version.ParseSemantic(providerVersion.Name); err != nil {
				return errInvalidArg("Providers[%d].Sources[%d].Name=%q", i, j, providerVersion.Name)
			}
			switch providerVersion.Type {
			case URLSource, KustomizeSource:
				if providerVersion.Value == "" {
					return errEmptyArg(fmt.Sprintf("Providers[%d].Sources[%d].Value", i, j))
				}
			default:
				return errInvalidArg("Providers[%d].Sources[%d].Type=%q", i, j, providerVersion.Type)
			}
			for k, replacement := range providerVersion.Replacements {
				if _, err := regexp.Compile(replacement.Old); err != nil {
					return errInvalidArg("Providers[%d].Sources[%d].Replacements[%d].Old=%q: %v", i, j, k, replacement.Old, err)
				}
			}
			// Providers files should be an existing file and have a target name.
			for k, file := range providerVersion.Files {
				if file.SourcePath == "" {
					return errInvalidArg("Providers[%d].Sources[%d].Files[%d].SourcePath=%q", i, j, k, file.SourcePath)
				}
				if !fileExists(file.SourcePath) {
					return errInvalidArg("Providers[%d].Sources[%d].Files[%d].SourcePath=%q", i, j, k, file.SourcePath)
				}
				if file.TargetName == "" {
					return errInvalidArg("Providers[%d].Sources[%d].Files[%d].TargetName=%q", i, j, k, file.TargetName)
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
	if len(providersByType[clusterctlv1.InfrastructureProviderType]) < 1 {
		return errInvalidArg("invalid config: it is required to have at least one infrastructure-provider")
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

// InfrastructureProviders returns the infrastructure provider selected for running this E2E test.
func (c *E2EConfig) InfrastructureProviders() []string {
	InfraProviders := []string{}
	for _, provider := range c.Providers {
		if provider.Type == string(clusterctlv1.InfrastructureProviderType) {
			InfraProviders = append(InfraProviders, provider.Name)
		}
	}
	return InfraProviders
}

func (c *E2EConfig) HasDockerProvider() bool {
	for _, i := range c.InfrastructureProviders() {
		if i == "docker" {
			return true
		}
	}
	return false
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

func (c *E2EConfig) HasVariable(varName string) bool {
	if _, ok := os.LookupEnv(varName); ok {
		return true
	}

	_, ok := c.Variables[varName]
	return ok
}

// GetVariable returns a variable from environment variables or from the e2e config file.
func (c *E2EConfig) GetVariable(varName string) string {
	if value, ok := os.LookupEnv(varName); ok {
		return value
	}

	value, ok := c.Variables[varName]
	Expect(ok).NotTo(BeFalse())
	return value
}

// GetInt64PtrVariable returns an Int64Ptr variable from the e2e config file.
func (c *E2EConfig) GetInt64PtrVariable(varName string) *int64 {
	wCountStr := c.GetVariable(varName)
	if wCountStr == "" {
		return nil
	}

	wCount, err := strconv.ParseInt(wCountStr, 10, 64)
	Expect(err).NotTo(HaveOccurred())
	return pointer.Int64Ptr(wCount)
}

// GetInt32PtrVariable returns an Int32Ptr variable from the e2e config file.
func (c *E2EConfig) GetInt32PtrVariable(varName string) *int32 {
	wCountStr := c.GetVariable(varName)
	if wCountStr == "" {
		return nil
	}

	wCount, err := strconv.ParseUint(wCountStr, 10, 32)
	Expect(err).NotTo(HaveOccurred())
	return pointer.Int32Ptr(int32(wCount))
}

// GetProviderVersions returns the sorted list of versions defined for a provider.
func (c *E2EConfig) GetProviderVersions(provider string) []string {
	return c.getVersions(provider, "*")
}

func (c *E2EConfig) GetProvidersWithOldestVersion(providers ...string) []string {
	ret := make([]string, 0, len(providers))
	for _, p := range providers {
		versions := c.getVersions(p, "*")
		if len(versions) > 0 {
			ret = append(ret, fmt.Sprintf("%s:%s", p, versions[0]))
		}
	}
	return ret
}

func (c *E2EConfig) GetProviderLatestVersionsByContract(contract string, providers ...string) []string {
	ret := make([]string, 0, len(providers))
	for _, p := range providers {
		versions := c.getVersions(p, contract)
		if len(versions) > 0 {
			ret = append(ret, fmt.Sprintf("%s:%s", p, versions[len(versions)-1]))
		}
	}
	return ret
}

func (c *E2EConfig) getVersions(provider string, contract string) []string {
	versions := []string{}
	for _, p := range c.Providers {
		if p.Name == provider {
			for _, v := range p.Versions {
				if contract == "*" || v.Contract == contract {
					versions = append(versions, v.Name)
				}
			}
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		// NOTE: Ignoring errors because the validity of the format is ensured by Validation.
		vI, _ := version.ParseSemantic(versions[i])
		vJ, _ := version.ParseSemantic(versions[j])
		return vI.LessThan(vJ)
	})
	return versions
}
