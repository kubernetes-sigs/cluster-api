/*
Copyright 2025 The Kubernetes Authors.

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

package migrate

import (
	"bytes"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

func TestEngine_Migrate(t *testing.T) {
	g := NewWithT(t)

	parser := NewYAMLParser(scheme.Scheme)
	converter, err := NewConverter(clusterv1.GroupVersion)
	g.Expect(err).ToNot(HaveOccurred())

	engine, err := NewEngine(parser, converter)
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("should convert v1beta1 cluster.x-k8s.io resources", func(t *testing.T) {
		g := NewWithT(t)

		input := `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: test-cluster
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
      - 192.168.0.0/16
`

		inputReader := strings.NewReader(input)
		outputBuffer := &bytes.Buffer{}
		errorsBuffer := &bytes.Buffer{}

		result, err := engine.Migrate(MigrationOptions{
			Input:  inputReader,
			Output: outputBuffer,
			Errors: errorsBuffer,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).ToNot(BeNil())
		g.Expect(result.TotalResources).To(Equal(1))
		g.Expect(result.ConvertedCount).To(Equal(1))
		g.Expect(result.SkippedCount).To(Equal(0))
		g.Expect(result.ErrorCount).To(Equal(0))

		output := outputBuffer.String()
		g.Expect(output).To(ContainSubstring("apiVersion: cluster.x-k8s.io/v1beta2"))
		g.Expect(output).To(ContainSubstring("kind: Cluster"))
		g.Expect(output).To(ContainSubstring("name: test-cluster"))
	})

	t.Run("should pass through non-CAPI resources", func(t *testing.T) {
		g := NewWithT(t)

		input := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: default
data:
  key: value
`

		inputReader := strings.NewReader(input)
		outputBuffer := &bytes.Buffer{}
		errorsBuffer := &bytes.Buffer{}

		result, err := engine.Migrate(MigrationOptions{
			Input:  inputReader,
			Output: outputBuffer,
			Errors: errorsBuffer,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).ToNot(BeNil())
		g.Expect(result.TotalResources).To(Equal(1))
		g.Expect(result.ConvertedCount).To(Equal(0))
		g.Expect(result.SkippedCount).To(Equal(1))
		g.Expect(result.ErrorCount).To(Equal(0))

		output := outputBuffer.String()
		g.Expect(output).To(ContainSubstring("apiVersion: v1"))
		g.Expect(output).To(ContainSubstring("kind: ConfigMap"))

		errorsOutput := errorsBuffer.String()
		g.Expect(errorsOutput).To(ContainSubstring("INFO: Passing through non-CAPI resource"))
	})

	t.Run("should handle multi-document YAML with mixed resources", func(t *testing.T) {
		g := NewWithT(t)

		input := `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: test-cluster
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
      - 192.168.0.0/16
---
apiVersion: v1
kind: Namespace
metadata:
  name: test-namespace
---
apiVersion: cluster.x-k8s.io/v1beta1
kind: Machine
metadata:
  name: test-machine
  namespace: default
spec:
  clusterName: test-cluster
  bootstrap:
    dataSecretName: test-bootstrap
`

		inputReader := strings.NewReader(input)
		outputBuffer := &bytes.Buffer{}
		errorsBuffer := &bytes.Buffer{}

		result, err := engine.Migrate(MigrationOptions{
			Input:  inputReader,
			Output: outputBuffer,
			Errors: errorsBuffer,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).ToNot(BeNil())
		g.Expect(result.TotalResources).To(Equal(3))
		g.Expect(result.ConvertedCount).To(Equal(2))
		g.Expect(result.SkippedCount).To(Equal(1))
		g.Expect(result.ErrorCount).To(Equal(0))

		output := outputBuffer.String()
		// Check that converted resources are at v1beta2
		g.Expect(strings.Count(output, "apiVersion: cluster.x-k8s.io/v1beta2")).To(Equal(2))
		// Check that non-CAPI resource is unchanged
		g.Expect(output).To(ContainSubstring("apiVersion: v1"))
		g.Expect(output).To(ContainSubstring("kind: Namespace"))
		// Check document separators are present
		g.Expect(strings.Count(output, "---")).To(Equal(2))
	})

	t.Run("should skip resources already at target version", func(t *testing.T) {
		g := NewWithT(t)

		input := `apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: test-cluster
  namespace: default
spec:
  topology:
    class: test-class
    version: v1.28.0
`

		inputReader := strings.NewReader(input)
		outputBuffer := &bytes.Buffer{}
		errorsBuffer := &bytes.Buffer{}

		result, err := engine.Migrate(MigrationOptions{
			Input:  inputReader,
			Output: outputBuffer,
			Errors: errorsBuffer,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).ToNot(BeNil())
		g.Expect(result.TotalResources).To(Equal(1))
		g.Expect(result.ConvertedCount).To(Equal(0))
		g.Expect(result.SkippedCount).To(Equal(1))
		g.Expect(result.ErrorCount).To(Equal(0))

		output := outputBuffer.String()
		g.Expect(output).To(ContainSubstring("apiVersion: cluster.x-k8s.io/v1beta2"))

		errorsOutput := errorsBuffer.String()
		g.Expect(errorsOutput).To(ContainSubstring("INFO:"))
		g.Expect(errorsOutput).To(ContainSubstring("already at version"))
		g.Expect(errorsOutput).To(ContainSubstring("v1beta2"))
	})

	t.Run("should handle empty input", func(t *testing.T) {
		g := NewWithT(t)

		input := ""
		inputReader := strings.NewReader(input)
		outputBuffer := &bytes.Buffer{}
		errorsBuffer := &bytes.Buffer{}

		result, err := engine.Migrate(MigrationOptions{
			Input:  inputReader,
			Output: outputBuffer,
			Errors: errorsBuffer,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).ToNot(BeNil())
		g.Expect(result.TotalResources).To(Equal(0))
		g.Expect(result.ConvertedCount).To(Equal(0))
		g.Expect(result.SkippedCount).To(Equal(0))
		g.Expect(result.ErrorCount).To(Equal(0))
	})
}
