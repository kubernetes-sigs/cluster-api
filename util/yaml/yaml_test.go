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

package yaml

import (
	"io/ioutil"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"os"
	"testing"

	. "github.com/onsi/gomega"
)

const validCluster = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: cluster1
spec:`

const validMachines1 = `
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Machine
metadata:
  name: machine1
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Machine
metadata:
  name: machine2`

const validUnified1 = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Machine
metadata:
  name: machine1`

const validUnified2 = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Machine
metadata:
  name: machine1
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Machine
metadata:
  name: machine2`

const validUnified3 = `
apiVersion: v1
data:
  cluster_name: cluster1
  cluster_network_pods_cidrBlock: 192.168.0.0/16
  cluster_network_services_cidrBlock: 10.96.0.0/12
  cluster_sshKeyName: default
kind: ConfigMap
metadata:
  name: cluster-api-shared-configuration
  namespace: cluster-api-test
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Machine
metadata:
  name: machine1
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Machine
metadata:
  name: machine2`

const invalidMachines1 = `
items:
- apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: machine1
  spec:`

const invalidMachines2 = `
items:
- metadata:
    name: machine1
  spec:
- metadata:
    name: machine2
`

const invalidUnified1 = `
apiVersion: v1
data:
  cluster_name: cluster1
  cluster_network_pods_cidrBlock: 192.168.0.0/16
  cluster_network_services_cidrBlock: 10.96.0.0/12
  cluster_sshKeyName: default
kind: ConfigMap
metadata:
  name: cluster-api-shared-configuration
  namespace: cluster-api-test
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Machine
- metadata:
    name: machine1
  spec:
- metadata:
    name: machine2
`

const invalidUnified2 = `
apiVersion: v1
data:
  cluster_name: cluster1
  cluster_network_pods_cidrBlock: 192.168.0.0/16
  cluster_network_services_cidrBlock: 10.96.0.0/12
  cluster_sshKeyName: default
kind: ConfigMap
metadata:
  name: cluster-api-shared-configuration
  namespace: cluster-api-test
---
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: cluster1
---
- metadata:
    name: machine1
  spec:
- metadata:
		name: machine2
`

func TestParseInvalidFile(t *testing.T) {
	g := NewWithT(t)

	_, err := Parse(ParseInput{
		File: "filedoesnotexist",
	})
	g.Expect(err).To(HaveOccurred())
}

func TestParseClusterYaml(t *testing.T) {
	g := NewWithT(t)

	var testcases = []struct {
		name         string
		contents     string
		expectedName string
		expectErr    bool
	}{
		{
			name:         "valid file",
			contents:     validCluster,
			expectedName: "cluster1",
		},
		{
			name:         "valid unified file",
			contents:     validUnified1,
			expectedName: "cluster1",
		},
		{
			name:         "valid unified file with separate machines",
			contents:     validUnified2,
			expectedName: "cluster1",
		},
		{
			name:         "valid unified file with separate machines and a configmap",
			contents:     validUnified3,
			expectedName: "cluster1",
		},
		{
			name:      "gibberish in file",
			contents:  `blah ` + validCluster + ` blah`,
			expectErr: true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			file, err := createTempFile(testcase.contents)
			g.Expect(err).NotTo(HaveOccurred())
			defer os.Remove(file)

			c, err := Parse(ParseInput{File: file})
			if testcase.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(c.Clusters).NotTo(HaveLen(0))
			g.Expect(c.Clusters[0].Name).To(Equal(testcase.expectedName))
		})
	}
}

func TestParseMachineYaml(t *testing.T) {
	g := NewWithT(t)

	var testcases = []struct {
		name                 string
		contents             string
		expectErr            bool
		expectedMachineCount int
	}{
		{
			name:                 "valid file using Machines",
			contents:             validMachines1,
			expectedMachineCount: 2,
		},
		{
			name:                 "valid unified file with machine list",
			contents:             validUnified1,
			expectedMachineCount: 1,
		},
		{
			name:                 "valid unified file with separate machines",
			contents:             validUnified2,
			expectedMachineCount: 2,
		},
		{
			name:                 "valid unified file with separate machines and a configmap",
			contents:             validUnified3,
			expectedMachineCount: 2,
		},
		{
			name:      "invalid file",
			contents:  invalidMachines1,
			expectErr: true,
		},
		{
			name:      "invalid file without type info",
			contents:  invalidMachines2,
			expectErr: true,
		},
		{
			name:      "valid unified for cluster with invalid Machine (only with type info) and a configmap",
			contents:  invalidUnified1,
			expectErr: true,
		},
		{
			name:      "valid unified for cluster with invalid Machine (old top-level items list) and a configmap",
			contents:  invalidUnified2,
			expectErr: true,
		},
		{
			name:      "gibberish in file",
			contents:  `!@#blah ` + validMachines1 + ` blah!@#`,
			expectErr: true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			file, err := createTempFile(testcase.contents)
			g.Expect(err).NotTo(HaveOccurred())
			defer os.Remove(file)

			out, err := Parse(ParseInput{File: file})
			if testcase.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out.Machines).To(HaveLen(testcase.expectedMachineCount))
		})
	}
}

func createTempFile(contents string) (filename string, reterr error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := f.Close(); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()
	_, _ = f.WriteString(contents)
	return f.Name(), nil
}
