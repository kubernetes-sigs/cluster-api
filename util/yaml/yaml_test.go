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
	"os"
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
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

func TestToUnstructured(t *testing.T) {
	type args struct {
		rawyaml []byte
	}
	tests := []struct {
		name          string
		args          args
		wantObjsCount int
		wantErr       bool
		err           string
	}{
		{
			name: "single object",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n"),
			},
			wantObjsCount: 1,
			wantErr:       false,
		},
		{
			name: "multiple objects are detected",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n"),
			},
			wantObjsCount: 2,
			wantErr:       false,
		},
		{
			name: "empty object are dropped",
			args: args{
				rawyaml: []byte("---\n" + //empty objects before
					"---\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" + // empty objects in the middle
					"---\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n" +
					"---\n" + //empty objects after
					"---\n" +
					"---\n"),
			},
			wantObjsCount: 2,
			wantErr:       false,
		},
		{
			name: "--- in the middle of objects are ignored",
			args: args{
				[]byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"data: \n" +
					" key: |\n" +
					"  ··Several lines of text,\n" +
					"  ··with some --- \n" +
					"  ---\n" +
					"  ··in the middle\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n"),
			},
			wantObjsCount: 2,
			wantErr:       false,
		},
		{
			name: "returns error for invalid yaml",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"foobar\n" +
					"kind: Secret\n"),
			},
			wantErr: true,
			err:     "failed to unmarshal the 2nd yaml document",
		},
		{
			name: "returns error for invalid yaml",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Pod\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Deployment\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"foobar\n" +
					"kind: ConfigMap\n"),
			},
			wantErr: true,
			err:     "failed to unmarshal the 4th yaml document",
		},
		{
			name: "returns error for invalid yaml",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"foobar\n" +
					"kind: ConfigMap\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n"),
			},
			wantErr: true,
			err:     "failed to unmarshal the 1st yaml document",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := ToUnstructured(tt.args.rawyaml)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if len(tt.err) != 0 {
					g.Expect(err.Error()).To(ContainSubstring(tt.err))
				}
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(HaveLen(tt.wantObjsCount))
		})
	}
}

func TestFromUnstructured(t *testing.T) {
	rawyaml := []byte("apiVersion: v1\n" +
		"kind: ConfigMap")

	unstructuredObj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		},
	}

	convertedyaml, err := FromUnstructured([]unstructured.Unstructured{unstructuredObj})
	g := NewWithT(t)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(rawyaml)).To(Equal(string(convertedyaml)))
}
