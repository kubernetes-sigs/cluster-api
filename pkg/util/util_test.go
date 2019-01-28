/*
Copyright 2018 The Kubernetes Authors.

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

package util

import (
	"io/ioutil"
	"os"
	"testing"
)

const validCluster = `
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: cluster1
spec:`

const validMachines1 = `
items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    name: machine1
  spec:`

const validMachines2 = `
---
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Machine
metadata:
  name: machine1
---
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Machine
metadata:
  name: machine2`

const validMachines3 = `
items:
- metadata:
    name: machine1
  spec:
- metadata:
    name: machine2
`

const validUnified1 = `
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.k8s.io/v1alpha1"
kind: MachineList
items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    name: machine1`

const validUnified2 = `
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Machine
metadata:
  name: machine1
---
apiVersion: "cluster.k8s.io/v1alpha1"
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
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Machine
metadata:
  name: machine1
---
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Machine
metadata:
  name: machine2`

const validUnified4 = `
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
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.k8s.io/v1alpha1"
kind: MachineList
items:
- metadata:
    name: machine1
  spec:
- metadata:
    name: machine2
`

func TestParseClusterYaml(t *testing.T) {
	t.Run("File does not exist", func(t *testing.T) {
		_, err := ParseClusterYaml("fileDoesNotExist")
		if err == nil {
			t.Fatal("Was able to parse a file that does not exist")
		}
	})
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
			name:         "valid unified file with machine list",
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
			name:         "valid unified file with machinelist (only with type info) and a configmap",
			contents:     validUnified4,
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
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(file)

			c, err := ParseClusterYaml(file)
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
			if err != nil {
				return
			}
			if c == nil {
				t.Fatalf("No cluster returned in success case.")
			}
			if c.Name != testcase.expectedName {
				t.Fatalf("Unexpected name. Got: %v, Want:%v", c.Name, testcase.expectedName)
			}
		})
	}
}

func TestParseMachineYaml(t *testing.T) {
	t.Run("File does not exist", func(t *testing.T) {
		_, err := ParseMachinesYaml("fileDoesNotExist")
		if err == nil {
			t.Fatal("Was able to parse a file that does not exist")
		}
	})
	var testcases = []struct {
		name                 string
		contents             string
		expectErr            bool
		expectedMachineCount int
	}{
		{
			name:                 "valid file using MachineList",
			contents:             validMachines1,
			expectedMachineCount: 1,
		},
		{
			name:                 "valid file using Machines",
			contents:             validMachines2,
			expectedMachineCount: 2,
		},
		{
			name:                 "valid file using MachineList without type info",
			contents:             validMachines3,
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
			name:                 "valid unified file with machinelist (only with type info) and a configmap",
			contents:             validUnified4,
			expectedMachineCount: 2,
		},
		{
			name:      "gibberish in file",
			contents:  `blah ` + validMachines1 + ` blah`,
			expectErr: true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			file, err := createTempFile(testcase.contents)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(file)

			m, err := ParseMachinesYaml(file)
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
			if err != nil {
				return
			}
			if m == nil {
				t.Fatalf("No machines returned in success case.")
			}
			if len(m) != testcase.expectedMachineCount {
				t.Fatalf("Unexpected machine count. Got: %v, Want: %v", len(m), testcase.expectedMachineCount)
			}
		})
	}
}

func createTempFile(contents string) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	defer f.Close()
	f.WriteString(contents)
	return f.Name(), nil
}
