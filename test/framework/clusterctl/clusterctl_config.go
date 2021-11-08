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
	"os"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

// Provide helpers for working with the clusterctl config file.

// clusterctlConfig defines the content of the clusterctl config file.
// The main responsibility for this structure is to point clusterctl to the local repository that should be used for E2E tests.
type clusterctlConfig struct {
	Path   string
	Values map[string]interface{}
}

// providerConfig mirrors the clusterctl config.Provider interface and allows serialization of the corresponding info into a clusterctl config file.
type providerConfig struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
	Type string `json:"type,omitempty"`
}

// write writes a clusterctl config file to disk.
func (c *clusterctlConfig) write() {
	data, err := yaml.Marshal(c.Values)
	Expect(err).ToNot(HaveOccurred(), "Failed to convert to yaml the clusterctl config file")

	Expect(os.WriteFile(c.Path, data, 0600)).To(Succeed(), "Failed to write the clusterctl config file")
}
