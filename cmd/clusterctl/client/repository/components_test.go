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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
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
			g.Expect(got).To(ContainElements(tt.want)) //skipping from test the automatically added namespace Object
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
		{
			name: "ClusterRoleBinding with subjects IN capi-webhook-system get fixed (without changing the subject namespace)",
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
									"namespace": "capi-webhook-system",
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
								"namespace": "capi-webhook-system", // Subjects namespace get preserved!
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
			name: "RoleBinding with subjects IN capi-webhook-system get fixed (without changing the subject namespace)",
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
									"namespace": "capi-webhook-system",
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
								"namespace": "capi-webhook-system", // Subjects namespace get preserved!
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

func fakeDeployment(watchNamespace string) unstructured.Unstructured {
	args := []string{}
	if watchNamespace != "" {
		args = append(args, fmt.Sprintf("%s%s", namespaceArgPrefix, watchNamespace))
	}
	return unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       deploymentKind,
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{
								"name": controllerContainerName,
								"args": args,
							},
						},
					},
				},
			},
		},
	}
}

func Test_inspectWatchNamespace(t *testing.T) {
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
			name: "get watchingNamespace if exists",
			args: args{
				objs: []unstructured.Unstructured{
					fakeDeployment("foo"),
				},
			},
			want: "foo",
		},
		{
			name: "get watchingNamespace if exists more than once, but it is consistent",
			args: args{
				objs: []unstructured.Unstructured{
					fakeDeployment("foo"),
					fakeDeployment("foo"),
				},
			},
			want: "foo",
		},
		{
			name: "return empty if there is no watchingNamespace",
			args: args{
				objs: []unstructured.Unstructured{},
			},
			want: "",
		},
		{
			name: "fails if inconsistent watchingNamespace",
			args: args{
				objs: []unstructured.Unstructured{
					fakeDeployment("foo"),
					fakeDeployment("bar"),
				},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := inspectWatchNamespace(tt.args.objs)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_fixWatchNamespace(t *testing.T) {
	type args struct {
		objs              []unstructured.Unstructured
		watchingNamespace string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "fix if existing",
			args: args{
				objs: []unstructured.Unstructured{
					fakeDeployment("foo"),
				},
				watchingNamespace: "bar",
			},
			wantErr: false,
		},
		{
			name: "set if not existing",
			args: args{
				objs: []unstructured.Unstructured{
					fakeDeployment(""),
				},
				watchingNamespace: "bar",
			},
			wantErr: false,
		},
		{
			name: "unset if existing",
			args: args{
				objs: []unstructured.Unstructured{
					fakeDeployment("foo"),
				},
				watchingNamespace: "",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := fixWatchNamespace(tt.args.objs, tt.args.watchingNamespace)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			wgot, err := inspectWatchNamespace(got)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(wgot).To(Equal(tt.args.watchingNamespace))
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

func Test_splitInstanceAndSharedResources(t *testing.T) {
	type args struct {
		objs []unstructured.Unstructured
	}
	tests := []struct {
		name             string
		args             args
		wantInstanceObjs []unstructured.Unstructured
		wantSharedObjs   []unstructured.Unstructured
	}{
		{
			name: "objects are split in two sets",
			args: args{
				objs: []unstructured.Unstructured{
					// Instance objs
					{
						Object: map[string]interface{}{
							"kind": "Namespace",
							"metadata": map[string]interface{}{
								"name": "capi-system",
							},
						},
					},
					{
						Object: map[string]interface{}{
							"kind": "Deployment",
							"metadata": map[string]interface{}{
								"name":      "capi-controller-manager",
								"namespace": "capi-system",
							},
						},
					},
					// Shared objs
					{
						Object: map[string]interface{}{
							"kind": "Namespace",
							"metadata": map[string]interface{}{
								"name": "capi-webhook-system",
							},
						},
					},
					{
						Object: map[string]interface{}{
							"kind": "Deployment",
							"metadata": map[string]interface{}{
								"name":      "capi-controller-manager",
								"namespace": "capi-webhook-system",
							},
						},
					},
					{
						Object: map[string]interface{}{
							"kind": "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "clusters.cluster.x-k8s.io",
							},
						},
					},
					{
						Object: map[string]interface{}{
							"kind": "MutatingWebhookConfiguration",
							"metadata": map[string]interface{}{
								"name": "capi-mutating-webhook-configuration",
							},
						},
					},
					{
						Object: map[string]interface{}{
							"kind": "ValidatingWebhookConfiguration",
							"metadata": map[string]interface{}{
								"name": "capi-validating-webhook-configuration",
							},
						},
					},
				},
			},
			wantInstanceObjs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind": "Namespace",
						"metadata": map[string]interface{}{
							"name": "capi-system",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"kind": "Deployment",
						"metadata": map[string]interface{}{
							"name":      "capi-controller-manager",
							"namespace": "capi-system",
						},
					},
				},
			},
			wantSharedObjs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind": "Namespace",
						"metadata": map[string]interface{}{
							"name": "capi-webhook-system",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"kind": "Deployment",
						"metadata": map[string]interface{}{
							"name":      "capi-controller-manager",
							"namespace": "capi-webhook-system",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"kind": "CustomResourceDefinition",
						"metadata": map[string]interface{}{
							"name": "clusters.cluster.x-k8s.io",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"kind": "MutatingWebhookConfiguration",
						"metadata": map[string]interface{}{
							"name": "capi-mutating-webhook-configuration",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"kind": "ValidatingWebhookConfiguration",
						"metadata": map[string]interface{}{
							"name": "capi-validating-webhook-configuration",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotInstanceObjs, gotWebHookObjs := splitInstanceAndSharedResources(tt.args.objs)
			g.Expect(gotInstanceObjs).To(ConsistOf(tt.wantInstanceObjs))
			g.Expect(gotWebHookObjs).To(ConsistOf(tt.wantSharedObjs))
		})
	}
}
