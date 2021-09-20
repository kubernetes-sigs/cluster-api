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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
	tests := []struct {
		name            string
		objs            []unstructured.Unstructured
		targetNamespace string
		want            []unstructured.Unstructured
		wantErr         bool
	}{
		{
			name: "fix Namespace object if exists",
			objs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       namespaceKind,
						"metadata": map[string]interface{}{
							"name": "foo",
						},
					},
				},
			},
			targetNamespace: "bar",
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       namespaceKind,
						"metadata": map[string]interface{}{
							"name": "bar",
						},
					},
				},
			},
		},
		{
			name: "fix namespaced objects",
			objs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "system",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Service",
						"metadata": map[string]interface{}{
							"name":      "capa-controller-manager-metrics-service",
							"namespace": "capa-system",
						},
					},
				},
			},
			targetNamespace: "bar",
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "bar",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Service",
						"metadata": map[string]interface{}{
							"name":      "capa-controller-manager-metrics-service",
							"namespace": "bar",
						},
					},
				},
			},
		},
		{
			name: "ignore global objects",
			objs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind": "ClusterRole",
					},
				},
			},
			targetNamespace: "bar",
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"kind": "ClusterRole",
						// no namespace
					},
				},
			},
		},
		{
			name: "fix v1beta1 webhook configs",
			objs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "admissionregistration.k8s.io/v1beta1",
						"kind":       "MutatingWebhookConfiguration",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "capi-webhook-system/capm3-serving-cert",
							},
							"name": "capm3-mutating-webhook-configuration",
						},
						"webhooks": []interface{}{
							map[string]interface{}{
								"clientConfig": map[string]interface{}{
									"caBundle": "Cg==",
									"service": map[string]interface{}{
										"name":      "capm3-webhook-service",
										"namespace": "capi-webhook-system",
										"path":      "/mutate-infrastructure-cluster-x-k8s-io-v1alpha4-metal3cluster",
									},
								},
							},
						},
					},
				},
			},
			targetNamespace: "bar",
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "admissionregistration.k8s.io/v1beta1",
						"kind":       "MutatingWebhookConfiguration",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "bar/capm3-serving-cert",
							},
							"creationTimestamp": nil,
							"name":              "capm3-mutating-webhook-configuration",
						},
						"webhooks": []interface{}{
							map[string]interface{}{
								"name": "",
								"clientConfig": map[string]interface{}{
									"service": map[string]interface{}{
										"name":      "capm3-webhook-service",
										"path":      "/mutate-infrastructure-cluster-x-k8s-io-v1alpha4-metal3cluster",
										"namespace": "bar",
									},
									"caBundle": "Cg==",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "unable to fix v1beta2 webhook configs",
			objs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "admissionregistration.k8s.io/v1beta2",
						"kind":       "MutatingWebhookConfiguration",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "capi-webhook-system/capm3-serving-cert",
							},
							"name": "capm3-mutating-webhook-configuration",
						},
						"webhooks": []interface{}{
							map[string]interface{}{
								"clientConfig": map[string]interface{}{
									"caBundle": "Cg==",
									"service": map[string]interface{}{
										"name":      "capm3-webhook-service",
										"namespace": "capi-webhook-system",
										"path":      "/mutate-infrastructure-cluster-x-k8s-io-v1alpha4-metal3cluster",
									},
								},
							},
						},
					},
				},
			},
			targetNamespace: "bar",
			wantErr:         true,
		}, {
			name: "fix v1 webhook configs",
			objs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "admissionregistration.k8s.io/v1",
						"kind":       "MutatingWebhookConfiguration",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "capi-webhook-system/capm3-serving-cert",
							},
							"name": "capm3-mutating-webhook-configuration",
						},
						"webhooks": []interface{}{
							map[string]interface{}{
								"clientConfig": map[string]interface{}{
									"caBundle": "Cg==",
									"service": map[string]interface{}{
										"name":      "capm3-webhook-service",
										"namespace": "capi-webhook-system",
										"path":      "/mutate-infrastructure-cluster-x-k8s-io-v1alpha4-metal3cluster",
									},
								},
							},
						},
					},
				},
			},
			targetNamespace: "bar",
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "admissionregistration.k8s.io/v1",
						"kind":       "MutatingWebhookConfiguration",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "bar/capm3-serving-cert",
							},
							"creationTimestamp": nil,
							"name":              "capm3-mutating-webhook-configuration",
						},
						"webhooks": []interface{}{
							map[string]interface{}{
								"name":                    "",
								"admissionReviewVersions": nil,
								"clientConfig": map[string]interface{}{
									"service": map[string]interface{}{
										"name":      "capm3-webhook-service",
										"path":      "/mutate-infrastructure-cluster-x-k8s-io-v1alpha4-metal3cluster",
										"namespace": "bar",
									},
									"caBundle": "Cg==",
								},
								"sideEffects": nil,
							},
						},
					},
				},
			},
		},
		{
			name: "fix v1beta1 crd webhook namespace",
			objs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apiextensions.k8s.io/v1beta1",
						"kind":       "CustomResourceDefinition",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "capi-webhook-system/capm3-serving-cert",
							},
							"name": "aCoolName",
						},
						"spec": map[string]interface{}{
							"conversion": map[string]interface{}{
								"strategy": "Webhook",
								"webhookClientConfig": map[string]interface{}{
									"caBundle": "Cg==",
									"service": map[string]interface{}{
										"name":      "capa-webhook-service",
										"namespace": "capi-webhook-system",
										"path":      "/convert",
									},
								},
							},
						},
					},
				},
			},
			targetNamespace: "bar",
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apiextensions.k8s.io/v1beta1",
						"kind":       "CustomResourceDefinition",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "bar/capm3-serving-cert",
							},
							"creationTimestamp": nil,
							"name":              "aCoolName",
						},
						"spec": map[string]interface{}{
							"group": "",
							"names": map[string]interface{}{"plural": "", "kind": ""},
							"scope": "",
							"conversion": map[string]interface{}{
								"strategy": "Webhook",
								"webhookClientConfig": map[string]interface{}{
									"caBundle": "Cg==",
									"service": map[string]interface{}{
										"name":      "capa-webhook-service",
										"namespace": "bar",
										"path":      "/convert",
									},
								},
							},
						},
						"status": map[string]interface{}{
							"storedVersions": nil,
							"conditions":     nil,
							"acceptedNames":  map[string]interface{}{"kind": "", "plural": ""},
						},
					},
				},
			},
		},
		{
			name: "unable to fix v1beta2 crd webhook namespace",
			objs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apiextensions.k8s.io/v1beta2",
						"kind":       "CustomResourceDefinition",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "capi-webhook-system/capm3-serving-cert",
							},
							"name": "aCoolName",
						},
						"spec": map[string]interface{}{
							"conversion": map[string]interface{}{
								"strategy": "Webhook",
								"webhookClientConfig": map[string]interface{}{
									"caBundle": "Cg==",
									"service": map[string]interface{}{
										"name":      "capa-webhook-service",
										"namespace": "capi-webhook-system",
										"path":      "/convert",
									},
								},
							},
						},
					},
				},
			},
			targetNamespace: "bar",
			wantErr:         true,
		},
		{
			name: "fix v1 crd webhook namespace",
			objs: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apiextensions.k8s.io/v1",
						"kind":       "CustomResourceDefinition",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "capi-webhook-system/capm3-serving-cert",
							},
							"name": "aCoolName",
						},
						"spec": map[string]interface{}{
							"conversion": map[string]interface{}{
								"strategy": "Webhook",
								"webhook": map[string]interface{}{
									"clientConfig": map[string]interface{}{
										"caBundle": "Cg==",
										"service": map[string]interface{}{
											"name":      "capa-webhook-service",
											"namespace": "capi-webhook-system",
											"path":      "/convert",
										},
									},
								},
							},
						},
					},
				},
			},
			targetNamespace: "bar",
			want: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apiextensions.k8s.io/v1",
						"kind":       "CustomResourceDefinition",
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"cert-manager.io/inject-ca-from": "bar/capm3-serving-cert",
							},
							"creationTimestamp": nil,
							"name":              "aCoolName",
						},
						"spec": map[string]interface{}{
							"group":    "",
							"names":    map[string]interface{}{"plural": "", "kind": ""},
							"scope":    "",
							"versions": nil,
							"conversion": map[string]interface{}{
								"strategy": "Webhook",
								"webhook": map[string]interface{}{
									"conversionReviewVersions": nil,
									"clientConfig": map[string]interface{}{
										"caBundle": "Cg==",
										"service": map[string]interface{}{
											"name":      "capa-webhook-service",
											"namespace": "bar",
											"path":      "/convert",
										},
									},
								},
							},
						},
						"status": map[string]interface{}{
							"storedVersions": nil,
							"conditions":     nil,
							"acceptedNames":  map[string]interface{}{"kind": "", "plural": ""},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := fixTargetNamespace(tt.objs, tt.targetNamespace)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
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
