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

package framework

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/test/framework/exec"
	"sigs.k8s.io/yaml"
)

const (
	// DefaultManagementClusterName is the default name of the Kind cluster
	// used by the the e2e framework.
	DefaultManagementClusterName = "mgmt"

	// DefaultKubernetesVersion is the default version of Kubernetes to deploy
	// for testing.
	DefaultKubernetesVersion = "v1.16.2"
)

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
	// The URL may begin with file://, http://, or https://.
	URLSource ComponentSourceType = "url"

	// KustomizeSource is a valid kustomization root that can be used to produce
	// the component YAML.
	KustomizeSource ComponentSourceType = "kustomize"
)

// ComponentSource describes how to obtain a component's YAML.
type ComponentSource struct {
	// Name is used for logging when a component has multiple sources.
	Name string `json:"name,omitempty"`

	// Value is the source of the component's YAML.
	// May be a URL or a kustomization root (specified by Type).
	// If a Type=url then Value may begin with file://, http://, or https://.
	// If a Type=kustomize then Value may be any valid go-getter URL. For
	// more information please see https://github.com/hashicorp/go-getter#url-format.
	Value string `json:"value"`

	// Type describes how to process the source of the component's YAML.
	//
	// Defaults to "kustomize".
	Type ComponentSourceType `json:"type,omitempty"`

	// Replacements is a list of patterns to replace in the component YAML
	// prior to application.
	Replacements []ComponentReplacement `json:"replacements,omitempty"`
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
	Sources []ComponentSource `json:"sources,omitempty"`

	// Waiters is an optional list of checks to perform in order to determine
	// whether or not the installed components are ready.
	Waiters []ComponentWaiter `json:"waiters,omitempty"`
}

// Config is the input used to configure the e2e test environment.
// Deprecated. Please use clusterctl.E2EConfig instead.
type Config struct {
	// Name is the name of the Kind management cluster.
	// Defaults to DefaultManagementClusterName.
	ManagementClusterName string `json:"managementClusterName,omitempty"`

	// KubernetesVersion is the version of Kubernetes to deploy when testing.
	// Defaults to DefaultKubernetesVersion.
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`

	// Images is a list of container images to load into the Kind cluster.
	Images []ContainerImage `json:"images,omitempty"`

	// Components is a list of component configurations applied to the
	// Kind cluster.
	// The components are applied serially, in the listed order.
	Components []ComponentConfig `json:"components,omitempty"`
}

// Defaults assigns default values to the object.
func (c *Config) Defaults() {
	if c.ManagementClusterName == "" {
		c.ManagementClusterName = DefaultManagementClusterName
	}
	if c.KubernetesVersion == "" {
		c.KubernetesVersion = DefaultKubernetesVersion
	}
	for i := range c.Components {
		componentConfig := &c.Components[i]
		for j := range componentConfig.Sources {
			source := &componentConfig.Sources[j]
			if source.Value != "" && source.Type == "" {
				source.Type = KustomizeSource
			}
		}
		for j := range componentConfig.Waiters {
			waiter := &componentConfig.Waiters[j]
			if waiter.Value != "" && waiter.Type == "" {
				waiter.Type = PodsWaiter
			}
		}
	}
	for i := range c.Images {
		containerImage := &c.Images[i]
		if containerImage.LoadBehavior == "" {
			containerImage.LoadBehavior = MustLoadImage
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

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.ManagementClusterName == "" {
		return errEmptyArg("ManagementClusterName")
	}
	if c.KubernetesVersion == "" {
		return errEmptyArg("KubernetesVersion")
	}
	for i, componentConfig := range c.Components {
		for j, source := range componentConfig.Sources {
			switch source.Type {
			case URLSource, KustomizeSource:
				if source.Value == "" {
					return errEmptyArg(fmt.Sprintf("Components[%d].Sources[%d].Value", i, j))
				}
			default:
				return errInvalidArg("Components[%d].Sources[%d].Type=%q", i, j, source.Type)
			}
			for k, replacement := range source.Replacements {
				if _, err := regexp.Compile(replacement.Old); err != nil {
					return errInvalidArg("Components[%d].Sources[%d].Replacements[%d].Old=%q: %v", i, j, k, replacement.Old, err)
				}
			}
		}
		for j, waiter := range componentConfig.Waiters {
			switch waiter.Type {
			case PodsWaiter, ServiceWaiter:
				if waiter.Value == "" {
					return errEmptyArg(fmt.Sprintf("Components[%d].Waiters[%d].Value", i, j))
				}
			default:
				return errInvalidArg("Components[%d].Waiters[%d].Type=%q", i, j, waiter.Type)
			}
		}
	}
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
	return nil
}

// LoadConfig loads a Config from the provided YAML data.
func LoadConfig(data []byte) (*Config, error) {
	if len(data) == 0 {
		return nil, io.ErrShortBuffer
	}
	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}
	return config, nil
}

// DefaultConfig returns a default Config object that loads cert-manager,
// CAPI core, the Kubeadm Bootstrapper, and the Kubeadm ControlPlane.
//
// Callers may append their own images to the returne Config.Images and their
// own components to Config.Components in order to stand up a management cluster
// for testing infrastructure providers.
func DefaultConfig() (Config, error) {
	config, err := LoadConfig([]byte(defaultConfigYAML))
	if err != nil {
		return Config{}, err
	}
	return *config, nil
}

// MustDefaultConfig panics if DefaultConfig returns an error.
func MustDefaultConfig() Config {
	config, err := DefaultConfig()
	if err != nil {
		panic(errors.Wrap(err, "failed to load default config YAML"))
	}
	return config
}

// YAMLForComponentSource returns the YAML for the provided component source.
func YAMLForComponentSource(ctx context.Context, source ComponentSource) ([]byte, error) {
	var data []byte

	switch source.Type {
	case URLSource:
		resp, err := http.Get(source.Value)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		data = buf
	case KustomizeSource:
		kustomize := exec.NewCommand(
			exec.WithCommand("kustomize"),
			exec.WithArgs("build", source.Value))
		stdout, stderr, err := kustomize.Run(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to execute kustomize: %s", stderr)
		}
		data = stdout
	default:
		return nil, errors.Errorf("invalid type: %q", source.Type)
	}

	for _, replacement := range source.Replacements {
		rx, err := regexp.Compile(replacement.Old)
		if err != nil {
			return nil, err
		}
		data = rx.ReplaceAll(data, []byte(replacement.New))
	}

	return data, nil
}

// ComponentGeneratorForComponentSource returns a ComponentGenerator for the
// provided ComponentSource.
func ComponentGeneratorForComponentSource(source ComponentSource) ComponentGenerator {
	return componentSourceGenerator{ComponentSource: source}
}

type componentSourceGenerator struct {
	ComponentSource
}

// GetName returns the name of the component.
func (g componentSourceGenerator) GetName() string {
	return g.Name
}

// Manifests return the YAML bundle.
func (g componentSourceGenerator) Manifests(ctx context.Context) ([]byte, error) {
	return YAMLForComponentSource(ctx, g.ComponentSource)
}
