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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const validCluster = `
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Cluster
metadata:
  name: cluster1
spec:`

const validMachines1 = `
---
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Machine
metadata:
  name: machine1
---
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Machine
metadata:
  name: machine2`

const validUnified1 = `
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: MachineList
items:
- apiVersion: "cluster.sigs.k8s.io/v1alpha2"
  kind: Machine
  metadata:
    name: machine1`

const validUnified2 = `
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Machine
metadata:
  name: machine1
---
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
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
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Machine
metadata:
  name: machine1
---
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Machine
metadata:
  name: machine2`

const invalidMachines1 = `
items:
- apiVersion: "cluster.sigs.k8s.io/v1alpha2"
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
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Cluster
metadata:
  name: cluster1
---
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: MachineList
items:
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
apiVersion: "cluster.sigs.k8s.io/v1alpha2"
kind: Cluster
metadata:
  name: cluster1
---
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
			name:         "valid unified for cluster with invalid machinelist (only with type info) and a configmap",
			contents:     invalidUnified1,
			expectedName: "cluster1",
		},
		{
			name:      "valid unified for cluster with invalid machinelist (old top-level items list) and a configmap",
			contents:  invalidUnified2,
			expectErr: true,
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
			name:      "invalid file using MachineList",
			contents:  invalidMachines1,
			expectErr: true,
		},
		{
			name:      "invalid file using MachineList without type info",
			contents:  invalidMachines2,
			expectErr: true,
		},
		{
			name:      "valid unified for cluster with invalid machinelist (only with type info) and a configmap",
			contents:  invalidUnified1,
			expectErr: true,
		},
		{
			name:      "valid unified for cluster with invalid machinelist (old top-level items list) and a configmap",
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

func TestMachineToInfrastructureMapFunc(t *testing.T) {
	var testcases = []struct {
		name    string
		input   schema.GroupVersionKind
		request handler.MapObject
		output  []reconcile.Request
	}{
		{
			name: "should reconcile infra-1",
			input: schema.GroupVersionKind{
				Group:   "foo.cluster.sigs.k8s.io",
				Version: "v1alpha2",
				Kind:    "TestMachine",
			},
			request: handler.MapObject{
				Object: &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-1",
					},
					Spec: clusterv1.MachineSpec{
						InfrastructureRef: corev1.ObjectReference{
							APIVersion: "foo.cluster.sigs.k8s.io/v1alpha2",
							Kind:       "TestMachine",
							Name:       "infra-1",
						},
					},
				},
			},
			output: []reconcile.Request{
				{
					NamespacedName: client.ObjectKey{
						Namespace: "default",
						Name:      "infra-1",
					},
				},
			},
		},
		{
			name: "should return no matching reconcile requests",
			input: schema.GroupVersionKind{
				Group:   "foo.cluster.sigs.k8s.io",
				Version: "v1alpha2",
				Kind:    "TestMachine",
			},
			request: handler.MapObject{
				Object: &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-1",
					},
					Spec: clusterv1.MachineSpec{
						InfrastructureRef: corev1.ObjectReference{
							APIVersion: "bar.cluster.sigs.k8s.io/v1alpha2",
							Kind:       "TestMachine",
							Name:       "bar-1",
						},
					},
				},
			},
			output: nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fn := MachineToInfrastructureMapFunc(tc.input)
			out := fn(tc.request)
			if !reflect.DeepEqual(out, tc.output) {
				t.Fatalf("Unexpected output. Got: %v, Want: %v", out, tc.output)
			}
		})
	}
}
