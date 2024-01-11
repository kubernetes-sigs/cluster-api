/*
Copyright 2021 The Kubernetes Authors.

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
	"testing"

	. "github.com/onsi/gomega"
)

func Test_E2EConfig_DeepCopy(t *testing.T) {
	g := NewWithT(t)

	config1 := E2EConfig{
		ManagementClusterName: "config1-cluster-name",
		Images: []ContainerImage{
			{
				Name: "config1-image",
			},
		},
		Providers: []ProviderConfig{
			{
				Name: "config1-provider",
			},
		},
		Variables: map[string]string{
			"config1-variable": "config1-variable-value",
		},
		Intervals: map[string][]string{
			"config1-interval": {"2m", "10s"},
		},
	}

	config2 := config1.DeepCopy()

	// Verify everything was copied from config1
	g.Expect(config2.ManagementClusterName).To(Equal("config1-cluster-name"))
	g.Expect(config2.Images).To(Equal([]ContainerImage{
		{
			Name: "config1-image",
		},
	}))
	g.Expect(config2.Providers).To(Equal([]ProviderConfig{
		{
			Name: "config1-provider",
		},
	}))
	g.Expect(config2.Variables).To(Equal(map[string]string{
		"config1-variable": "config1-variable-value",
	}))
	g.Expect(config2.Intervals).To(Equal(map[string][]string{
		"config1-interval": {"2m", "10s"},
	}))

	// Mutate config2
	config2.ManagementClusterName = "config2-cluster-name"
	config2.Images[0].Name = "config2-image"
	config2.Providers[0].Name = "config2-provider"
	config2.Variables["config2-variable"] = "config2-variable-value"
	config2.Intervals["config2-interval"] = []string{"2m", "10s"}

	// Validate config1 was not mutated
	g.Expect(config1.ManagementClusterName).To(Equal("config1-cluster-name"))
	g.Expect(config1.Images).To(Equal([]ContainerImage{
		{
			Name: "config1-image",
		},
	}))
	g.Expect(config1.Providers).To(Equal([]ProviderConfig{
		{
			Name: "config1-provider",
		},
	}))
	g.Expect(config1.Variables).To(Equal(map[string]string{
		"config1-variable": "config1-variable-value",
	}))
	g.Expect(config1.Intervals).To(Equal(map[string][]string{
		"config1-interval": {"2m", "10s"},
	}))
}
