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

package framework

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/framework/management/kind"
)

// InitManagementClusterInput is the information required to initialize a new
// management cluster for e2e testing.
type InitManagementClusterInput struct {
	Config

	// Scheme is used to initialize the scheme for the management cluster
	// client.
	// Defaults to a new runtime.Scheme.
	Scheme *runtime.Scheme

	// ComponentGenerators is a list objects that supply additional component
	// YAML to apply to the management cluster.
	// Please note this is meant to be used at runtime to add YAML to the
	// management cluster outside of what is provided by the Components field.
	// For example, a caller could use this field to apply a Secret required by
	// some component from the Components field.
	ComponentGenerators []ComponentGenerator

	// NewManagementClusterFn may be used to provide a custom function for
	// returning a new management cluster. Otherwise kind.NewCluster is used.
	NewManagementClusterFn func() (ManagementCluster, error)
}

// Defaults assigns default values to the object.
func (c *InitManagementClusterInput) Defaults(ctx context.Context) {
	c.Config.Defaults()
	if c.Scheme == nil {
		c.Scheme = runtime.NewScheme()
	}
	if c.NewManagementClusterFn == nil {
		c.NewManagementClusterFn = func() (ManagementCluster, error) {
			return kind.NewCluster(ctx, c.ManagementClusterName, c.Scheme)
		}
	}
}

// InitManagementCluster returns a new cluster initialized as a CAPI management
// cluster.
func InitManagementCluster(ctx context.Context, input *InitManagementClusterInput) ManagementCluster {
	By("initializing the management cluster")
	Expect(input).ToNot(BeNil())

	By("initialzing the management cluster configuration defaults")
	input.Defaults(ctx)

	By("validating the management cluster configuration")
	Expect(input.Validate()).To(Succeed())

	By("loading the kubernetes and capi core schemes")
	TryAddDefaultSchemes(input.Scheme)

	By("creating the management cluster")
	managementCluster, err := input.NewManagementClusterFn()
	Expect(err).ToNot(HaveOccurred())
	Expect(managementCluster).ToNot(BeNil())

	// Load the images.
	if imageLoader, ok := managementCluster.(ImageLoader); ok {
		By("management cluster supports loading images")
		for _, image := range input.Images {
			switch image.LoadBehavior {
			case MustLoadImage:
				By(fmt.Sprintf("must load image %s into the management cluster", image.Name))
				Expect(imageLoader.LoadImage(ctx, image.Name)).To(Succeed())
			case TryLoadImage:
				By(fmt.Sprintf("try to load image %s into the management cluster", image.Name))
				imageLoader.LoadImage(ctx, image.Name) //nolint:errcheck
			}
		}
	}

	// Install the YAML from the component generators.
	for _, componentGenerator := range input.ComponentGenerators {
		InstallComponents(ctx, managementCluster, componentGenerator)
	}

	// Install all components.
	for _, component := range input.Components {
		for _, source := range component.Sources {
			name := component.Name
			if source.Name != "" {
				name = fmt.Sprintf("%s/%s", component.Name, source.Name)
			}
			source.Name = name
			InstallComponents(ctx, managementCluster, ComponentGeneratorForComponentSource(source))
		}
		for _, waiter := range component.Waiters {
			switch waiter.Type {
			case PodsWaiter:
				WaitForPodsReadyInNamespace(ctx, managementCluster, waiter.Value)
			case ServiceWaiter:
				WaitForAPIServiceAvailable(ctx, managementCluster, waiter.Value)
			}
		}
	}

	return managementCluster
}
