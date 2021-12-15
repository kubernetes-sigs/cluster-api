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
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
)

func Test_getManagedPaths(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		want []contract.Path
	}{
		{
			name: "Return no paths if the annotation does not exists",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			want: []contract.Path{},
		},
		{
			name: "Return paths from the annotation",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							clusterv1.ClusterTopologyManagedFieldsAnnotation: "foo, bar.baz",
						},
					},
				},
			},
			want: []contract.Path{
				{"spec", "foo"},
				{"spec", "bar", "baz"},
			},
		},
		{
			name: "Handle label names properly",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							clusterv1.ClusterTopologyManagedFieldsAnnotation: "bar.foo%bar%baz, foo",
						},
					},
				},
			},
			want: []contract.Path{
				{"spec", "bar", "foo.bar.baz"},
				{"spec", "foo"},
			},
		},
		{
			name: "Handle deep nesting ",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							clusterv1.ClusterTopologyManagedFieldsAnnotation: "kubeadmConfigSpec.clusterConfiguration.imageRepository, " +
								"kubeadmConfigSpec.clusterConfiguration.version, " +
								"kubeadmConfigSpec.initConfiguration.bootstrapToken, " +
								"kubeadmConfigSpec.initConfiguration.nodeRegistration.criSocket, " +
								"kubeadmConfigSpec.initConfiguration.nodeRegistration.kubeletExtraArgs.cgroup-driver, " +
								"kubeadmConfigSpec.initConfiguration.nodeRegistration.kubeletExtraArgs.eviction-hard, " +
								"kubeadmConfigSpec.joinConfiguration.nodeRegistration.criSocket, " +
								"kubeadmConfigSpec.joinConfiguration.nodeRegistration.kubeletExtraArgs.cgroup-driver, " +
								"kubeadmConfigSpec.joinConfiguration.nodeRegistration.kubeletExtraArgs.eviction-hard, " +
								"machineTemplate.infrastructureRef.apiVersion, " +
								"machineTemplate.infrastructureRef.kind, " +
								"machineTemplate.infrastructureRef.name, " +
								"machineTemplate.infrastructureRef.namespace, " +
								"machineTemplate.metadata.labels.cluster%x-k8s%io/cluster-name, " +
								"machineTemplate.metadata.labels.topology%cluster%x-k8s%io/owned, " +
								"replicas, " +
								"version",
						},
					},
				},
			},
			want: []contract.Path{
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "imageRepository"},
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "version"},
				{"spec", "kubeadmConfigSpec", "initConfiguration", "bootstrapToken"},
				{"spec", "kubeadmConfigSpec", "initConfiguration", "nodeRegistration", "criSocket"},
				{"spec", "kubeadmConfigSpec", "initConfiguration", "nodeRegistration", "kubeletExtraArgs", "cgroup-driver"},
				{"spec", "kubeadmConfigSpec", "initConfiguration", "nodeRegistration", "kubeletExtraArgs", "eviction-hard"},
				{"spec", "kubeadmConfigSpec", "joinConfiguration", "nodeRegistration", "criSocket"},
				{"spec", "kubeadmConfigSpec", "joinConfiguration", "nodeRegistration", "kubeletExtraArgs", "cgroup-driver"},
				{"spec", "kubeadmConfigSpec", "joinConfiguration", "nodeRegistration", "kubeletExtraArgs", "eviction-hard"},
				{"spec", "machineTemplate", "infrastructureRef", "apiVersion"},
				{"spec", "machineTemplate", "infrastructureRef", "kind"},
				{"spec", "machineTemplate", "infrastructureRef", "name"},
				{"spec", "machineTemplate", "infrastructureRef", "namespace"},
				{"spec", "machineTemplate", "metadata", "labels", "cluster.x-k8s.io/cluster-name"},
				{"spec", "machineTemplate", "metadata", "labels", "topology.cluster.x-k8s.io/owned"},
				{"spec", "replicas"},
				{"spec", "version"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := getManagedPaths(tt.obj)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_storeManagedPaths(t *testing.T) {
	tests := []struct {
		name           string
		obj            client.Object
		IgnorePaths    []contract.Path
		wantAnnotation string
	}{
		{
			name: "Does not add annotation for typed objects",
			obj: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					ClusterNetwork: &clusterv1.ClusterNetwork{
						ServiceDomain: "foo.bar",
					},
				},
			},
			wantAnnotation: "",
		},
		{
			name: "Add empty annotation in case there are no changes to spec",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			wantAnnotation: "",
		},
		{
			name: "Add annotation in case of changes to spec",
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
			wantAnnotation: "bar.baz, foo",
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
			wantAnnotation: "bar.foo%bar%baz, foo",
		},
		{
			name: "Add annotation handling properly deep nesting in spec",
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
			wantAnnotation: "kubeadmConfigSpec.clusterConfiguration.imageRepository, " +
				"kubeadmConfigSpec.clusterConfiguration.version, " +
				"kubeadmConfigSpec.initConfiguration.bootstrapToken, " +
				"kubeadmConfigSpec.initConfiguration.nodeRegistration.criSocket, " +
				"kubeadmConfigSpec.initConfiguration.nodeRegistration.kubeletExtraArgs.cgroup-driver, " +
				"kubeadmConfigSpec.initConfiguration.nodeRegistration.kubeletExtraArgs.eviction-hard, " +
				"kubeadmConfigSpec.joinConfiguration.nodeRegistration.criSocket, " +
				"kubeadmConfigSpec.joinConfiguration.nodeRegistration.kubeletExtraArgs.cgroup-driver, " +
				"kubeadmConfigSpec.joinConfiguration.nodeRegistration.kubeletExtraArgs.eviction-hard, " +
				"machineTemplate.infrastructureRef.apiVersion, " +
				"machineTemplate.infrastructureRef.kind, " +
				"machineTemplate.infrastructureRef.name, " +
				"machineTemplate.infrastructureRef.namespace, " +
				"machineTemplate.metadata.labels.cluster%x-k8s%io/cluster-name, " +
				"machineTemplate.metadata.labels.topology%cluster%x-k8s%io/owned, " +
				"replicas, " +
				"version",
		},
		{
			name: "Annotation does not include ignorePaths",
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
			IgnorePaths: []contract.Path{
				{"spec", "version"}, // exact match (drops a single path)
				{"spec", "kubeadmConfigSpec", "initConfiguration"}, // prefix match (drops everything below a path)
			},
			wantAnnotation: "kubeadmConfigSpec.clusterConfiguration.version, kubeadmConfigSpec.joinConfiguration, replicas",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := storeManagedPaths(tt.obj, tt.IgnorePaths)
			g.Expect(err).ToNot(HaveOccurred())

			gotAnnotation := tt.obj.GetAnnotations()[clusterv1.ClusterTopologyManagedFieldsAnnotation]
			g.Expect(gotAnnotation).To(Equal(tt.wantAnnotation))
		})
	}
}
