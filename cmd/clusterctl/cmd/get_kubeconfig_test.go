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

package cmd

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

var (
	//go:embed testdata/existing-kubeconfig.yaml
	existingKubeconfig string

	//go:embed testdata/single-workload-kubeconfig.yaml
	singleWorkloadKubeconfig string

	//go:embed testdata/multiple-workload-kubeconfig.yaml
	multipleWorkloadKubeconfig string
)

func Test_intoKubeconfig(t *testing.T) {
	tmpDir := t.TempDir()

	tt := []struct {
		name     string
		path     string
		data     string
		expected string
	}{
		{
			name:     "inserting into empty kubeconfig",
			path:     filepath.Join(tmpDir, "empty-kubeconfig"),
			data:     "",
			expected: singleWorkloadKubeconfig,
		},
		{
			name:     "inserting into existing kubeconfig",
			path:     filepath.Join(tmpDir, "existing-kubeconfig"),
			data:     existingKubeconfig,
			expected: multipleWorkloadKubeconfig,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			if tc.data != "" {
				err := os.WriteFile(tc.path, []byte(tc.data), 0600)
				g.Expect(err).ToNot(HaveOccurred())
			}

			err := intoKubeconfig(tc.path, singleWorkloadKubeconfig)
			g.Expect(err).ToNot(HaveOccurred())

			expected, err := os.ReadFile(tc.path)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(string(expected)).To(Equal(tc.expected))
		})
	}
}
