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

package mergepatch

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
)

func Test_ManagedFieldAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		obj         client.Object
		ignorePaths []contract.Path
		wantPaths   []contract.Path
	}{
		{
			name: "Does not add managed fields annotation for typed objects",
			obj: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					ClusterNetwork: &clusterv1.ClusterNetwork{
						ServiceDomain: "foo.bar",
					},
				},
			},
			wantPaths: nil,
		},
		{
			name: "Add empty managed fields annotation in case we are not setting fields in spec",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			wantPaths: []contract.Path{},
		},
		{
			name: "Add managed fields annotation in case we are not setting fields in spec",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "foo",
						"bar": map[string]interface{}{
							"baz": "baz",
						},
					},
				},
			},
			wantPaths: []contract.Path{
				{"spec", "foo"},
				{"spec", "bar", "baz"},
			},
		},
		{
			name: "Handle label names properly",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "foo",
						"bar": map[string]interface{}{
							"foo.bar.baz": "baz",
						},
					},
				},
			},
			wantPaths: []contract.Path{
				{"spec", "foo"},
				{"spec", "bar", "foo.bar.baz"},
			},
		},
		{
			name: "Add managed fields annotation handling properly deep nesting in spec",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"replicas": int64(4),
						"version":  "1.17.3",
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"imageRepository": "foo",
								"version":         "v2.0.1",
							},
							"initConfiguration": map[string]interface{}{
								"bootstrapToken": []interface{}{"abcd", "defg"},
								"nodeRegistration": map[string]interface{}{
									"criSocket": "foo",
									"kubeletExtraArgs": map[string]interface{}{
										"cgroup-driver": "foo",
										"eviction-hard": "foo",
									},
								},
							},
							"joinConfiguration": map[string]interface{}{
								"nodeRegistration": map[string]interface{}{
									"criSocket": "foo",
									"kubeletExtraArgs": map[string]interface{}{
										"cgroup-driver": "foo",
										"eviction-hard": "foo",
									},
								},
							},
						},
						"machineTemplate": map[string]interface{}{
							"infrastructureRef": map[string]interface{}{
								"apiVersion": "foo",
								"kind":       "foo",
								"name":       "foo",
								"namespace":  "foo",
							},
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"cluster.x-k8s.io/cluster-name":   "foo",
									"topology.cluster.x-k8s.io/owned": "foo",
								},
							},
						},
					},
				},
			},
			wantPaths: []contract.Path{
				{"spec", "replicas"},
				{"spec", "version"},
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "imageRepository"},
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "version"},
				{"spec", "kubeadmConfigSpec", "initConfiguration", "bootstrapToken"},
				{"spec", "kubeadmConfigSpec", "initConfiguration", "nodeRegistration", "criSocket"},
				{"spec", "kubeadmConfigSpec", "initConfiguration", "nodeRegistration", "kubeletExtraArgs", "cgroup-driver"},
				{"spec", "kubeadmConfigSpec", "initConfiguration", "nodeRegistration", "kubeletExtraArgs", "eviction-hard"},
				{"spec", "kubeadmConfigSpec", "joinConfiguration", "nodeRegistration", "criSocket"},
				{"spec", "kubeadmConfigSpec", "joinConfiguration", "nodeRegistration", "kubeletExtraArgs", "cgroup-driver"},
				{"spec", "kubeadmConfigSpec", "joinConfiguration", "nodeRegistration", "kubeletExtraArgs", "eviction-hard"},
				{"spec", "machineTemplate", "infrastructureRef", "namespace"},
				{"spec", "machineTemplate", "infrastructureRef", "apiVersion"},
				{"spec", "machineTemplate", "infrastructureRef", "kind"},
				{"spec", "machineTemplate", "infrastructureRef", "name"},
				{"spec", "machineTemplate", "metadata", "labels", "cluster.x-k8s.io/cluster-name"},
				{"spec", "machineTemplate", "metadata", "labels", "topology.cluster.x-k8s.io/owned"},
			},
		},
		{
			name: "Managed fields annotation does not include ignorePaths",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"replicas": int64(4),
						"version":  "1.17.3",
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"version": "v2.0.1",
							},
							"initConfiguration": map[string]interface{}{
								"bootstrapToken": []interface{}{"abcd", "defg"},
							},
							"joinConfiguration": nil,
						},
					},
				},
			},
			ignorePaths: []contract.Path{
				{"spec", "version"}, // exact match (drops a single path)
				{"spec", "kubeadmConfigSpec", "initConfiguration"}, // prefix match (drops everything below a path)
			},
			wantPaths: []contract.Path{
				{"spec", "replicas"},
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "version"},
				{"spec", "kubeadmConfigSpec", "joinConfiguration"},
			},
		},
		{
			name: "Managed fields annotation ignore empty maps",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"version": "v2.0.1",
							},
							"initConfiguration": map[string]interface{}{},
						},
					},
				},
			},
			wantPaths: []contract.Path{
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "version"},
			},
		},
		{
			name: "Managed fields annotation ignore empty maps - excluding ignore paths",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"version": "v2.0.1",
							},
							"initConfiguration": map[string]interface{}{},
						},
					},
				},
			},
			ignorePaths: []contract.Path{
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "version"},
			},
			wantPaths: []contract.Path{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := storeManagedPaths(tt.obj, tt.ignorePaths)
			g.Expect(err).ToNot(HaveOccurred())

			_, hasAnnotation := tt.obj.GetAnnotations()[clusterv1.ClusterTopologyManagedFieldsAnnotation]
			g.Expect(hasAnnotation).To(Equal(tt.wantPaths != nil))

			if hasAnnotation {
				gotPaths, err := getManagedPaths(tt.obj)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(gotPaths).To(HaveLen(len(tt.wantPaths)), fmt.Sprintf("%v", gotPaths))
				for _, w := range tt.wantPaths {
					g.Expect(gotPaths).To(ContainElement(w))
				}
			}
		})
	}
}
