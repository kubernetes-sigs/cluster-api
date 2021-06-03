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

package repository

import (
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
)

func Test_inspectTargetNamespace(t *testing.T) {
	type args struct {
		objs []unstructured.Unstructured
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "get targetNamespace if exists",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind": "Namespace",
							"metadata": map[string]interface{}{
								"name": "foo",
							},
						},
					},
				},
			},
			want: "foo",
		},
		{
			name: "return empty if there is no targetNamespace",
			args: args{
				objs: []unstructured.Unstructured{},
			},
			want: "",
		},
		{
			name: "fails if two Namespace exists",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind": "Namespace",
							"metadata": map[string]interface{}{
								"name": "foo",
							},
						},
					},
					{
						Object: map[string]interface{}{
							"kind": "Namespace",
							"metadata": map[string]interface{}{
								"name": "bar",
							},
						},
					},
				},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := inspectTargetNamespace(tt.args.objs)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_fixTargetNamespace(t *testing.T) {
	type args struct {
		objs            []unstructured.Unstructured
		targetNamespace string
	}
	tests := []struct {
		name string
		args args
		want []unstructured.Unstructured
	}{
		{
			name: "fix Namespace object if exists",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind": namespaceKind,
							"metadata": map[string]interface{}{
								"name": "foo",
							},
						},
					},
				},
				targetNamespace: "bar",
			},
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind": namespaceKind,
						"metadata": map[string]interface{}{
							"name": "bar",
						},
					},
				},
			},
		},
		{
			name: "fix namespaced objects",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind": "Pod",
						},
					},
				},
				targetNamespace: "bar",
			},
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind": "Pod",
						"metadata": map[string]interface{}{
							"namespace": "bar",
						},
					},
				},
			},
		},
		{
			name: "ignore global objects",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind": "ClusterRole",
						},
					},
				},
				targetNamespace: "bar",
			},
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind": "ClusterRole",
						// no namespace
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := fixTargetNamespace(tt.args.objs, tt.args.targetNamespace)
			g.Expect(got).To(ContainElements(tt.want)) // skipping from test the automatically added namespace Object
		})
	}
}

func Test_addNamespaceIfMissing(t *testing.T) {
	type args struct {
		objs            []unstructured.Unstructured
		targetNamespace string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "don't add Namespace object if exists",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind": namespaceKind,
							"metadata": map[string]interface{}{
								"name": "foo",
							},
						},
					},
				},
				targetNamespace: "foo",
			},
		},
		{
			name: "add Namespace object if it does not exists",
			args: args{
				objs:            []unstructured.Unstructured{},
				targetNamespace: "bar",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := addNamespaceIfMissing(tt.args.objs, tt.args.targetNamespace)

			wgot, err := inspectTargetNamespace(got)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(wgot).To(Equal(tt.args.targetNamespace))
		})
	}
}

func Test_fixRBAC(t *testing.T) {
	type args struct {
		objs            []unstructured.Unstructured
		targetNamespace string
	}
	tests := []struct {
		name    string
		args    args
		want    []unstructured.Unstructured
		wantErr bool
	}{
		{
			name: "ClusterRole get fixed",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind":       "ClusterRole",
							"apiVersion": "rbac.authorization.k8s.io/v1",
							"metadata": map[string]interface{}{
								"name": "foo",
							},
						},
					},
				},
				targetNamespace: "target",
			},
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind":       "ClusterRole",
						"apiVersion": "rbac.authorization.k8s.io/v1",
						"metadata": map[string]interface{}{
							"name": "target-foo", // ClusterRole name fixed!
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "ClusterRoleBinding with roleRef NOT IN components YAML get fixed",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind":       "ClusterRoleBinding",
							"apiVersion": "rbac.authorization.k8s.io/v1",
							"metadata": map[string]interface{}{
								"name": "foo",
							},
							"roleRef": map[string]interface{}{
								"apiGroup": "",
								"kind":     "",
								"name":     "bar",
							},
							"subjects": []interface{}{
								map[string]interface{}{
									"kind":      "ServiceAccount",
									"name":      "baz",
									"namespace": "baz",
								},
								map[string]interface{}{
									"kind": "User",
									"name": "qux",
								},
							},
						},
					},
				},
				targetNamespace: "target",
			},
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind":       "ClusterRoleBinding",
						"apiVersion": "rbac.authorization.k8s.io/v1",
						"metadata": map[string]interface{}{
							"name":              "target-foo", // ClusterRoleBinding name fixed!
							"creationTimestamp": nil,
						},
						"roleRef": map[string]interface{}{
							"apiGroup": "",
							"kind":     "",
							"name":     "bar", // ClusterRole name NOT fixed (not in components YAML)!
						},
						"subjects": []interface{}{
							map[string]interface{}{
								"kind":      "ServiceAccount",
								"name":      "baz",
								"namespace": "target", // Subjects namespace fixed!
							},
							map[string]interface{}{
								"kind": "User",
								"name": "qux",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "ClusterRoleBinding with roleRef IN components YAML get fixed",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind":       "ClusterRoleBinding",
							"apiVersion": "rbac.authorization.k8s.io/v1",
							"metadata": map[string]interface{}{
								"name": "foo",
							},
							"roleRef": map[string]interface{}{
								"apiGroup": "",
								"kind":     "",
								"name":     "bar",
							},
							"subjects": []interface{}{
								map[string]interface{}{
									"kind":      "ServiceAccount",
									"name":      "baz",
									"namespace": "baz",
								},
								map[string]interface{}{
									"kind": "User",
									"name": "qux",
								},
							},
						},
					},
					{
						Object: map[string]interface{}{
							"kind":       "ClusterRole",
							"apiVersion": "rbac.authorization.k8s.io/v1",
							"metadata": map[string]interface{}{
								"name": "bar",
							},
						},
					},
				},
				targetNamespace: "target",
			},
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind":       "ClusterRoleBinding",
						"apiVersion": "rbac.authorization.k8s.io/v1",
						"metadata": map[string]interface{}{
							"name":              "target-foo", // ClusterRoleBinding name fixed!
							"creationTimestamp": nil,
						},
						"roleRef": map[string]interface{}{
							"apiGroup": "",
							"kind":     "",
							"name":     "target-bar", // ClusterRole name fixed!
						},
						"subjects": []interface{}{
							map[string]interface{}{
								"kind":      "ServiceAccount",
								"name":      "baz",
								"namespace": "target", // Subjects namespace fixed!
							},
							map[string]interface{}{
								"kind": "User",
								"name": "qux",
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"kind":       "ClusterRole",
						"apiVersion": "rbac.authorization.k8s.io/v1",
						"metadata": map[string]interface{}{
							"name": "target-bar", // ClusterRole fixed!
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "RoleBinding get fixed",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind":       "RoleBinding",
							"apiVersion": "rbac.authorization.k8s.io/v1",
							"metadata": map[string]interface{}{
								"name":      "foo",
								"namespace": "target",
							},
							"roleRef": map[string]interface{}{
								"apiGroup": "",
								"kind":     "",
								"name":     "bar",
							},
							"subjects": []interface{}{
								map[string]interface{}{
									"kind":      "ServiceAccount",
									"name":      "baz",
									"namespace": "baz",
								},
								map[string]interface{}{
									"kind": "User",
									"name": "qux",
								},
							},
						},
					},
				},
				targetNamespace: "target",
			},
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind":       "RoleBinding",
						"apiVersion": "rbac.authorization.k8s.io/v1",
						"metadata": map[string]interface{}{
							"name":              "foo",
							"namespace":         "target",
							"creationTimestamp": nil,
						},
						"roleRef": map[string]interface{}{
							"apiGroup": "",
							"kind":     "",
							"name":     "bar",
						},
						"subjects": []interface{}{
							map[string]interface{}{
								"kind":      "ServiceAccount",
								"name":      "baz",
								"namespace": "target", // Subjects namespace fixed!
							},
							map[string]interface{}{
								"kind": "User",
								"name": "qux",
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := fixRBAC(tt.args.objs, tt.args.targetNamespace)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_addCommonLabels(t *testing.T) {
	type args struct {
		objs         []unstructured.Unstructured
		name         string
		providerType clusterctlv1.ProviderType
	}
	tests := []struct {
		name string
		args args
		want []unstructured.Unstructured
	}{
		{
			name: "add labels",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind": "ClusterRole",
						},
					},
				},
				name:         "provider",
				providerType: clusterctlv1.InfrastructureProviderType,
			},
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind": "ClusterRole",
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								clusterctlv1.ClusterctlLabelName: "",
								clusterv1.ProviderLabelName:      "infrastructure-provider",
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := addCommonLabels(tt.args.objs, config.NewProvider(tt.args.name, "", tt.args.providerType))
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
