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

package client

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestClusterClassExists(t *testing.T) {
	tests := []struct {
		name         string
		objs         []client.Object
		clusterClass string
		want         bool
	}{
		{
			name: "should return true when checking for an installed cluster class",
			objs: []client.Object{
				&clusterv1.ClusterClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "clusterclass-installed",
						Namespace: metav1.NamespaceDefault,
					},
				},
			},
			clusterClass: "clusterclass-installed",
			want:         true,
		},
		{
			name:         "should return false when checking for a not-installed cluster class",
			objs:         []client.Object{},
			clusterClass: "clusterclass-not-installed",
			want:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			config := newFakeConfig()
			client := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config).WithObjs(tt.objs...)
			c, _ := client.Proxy().NewClient()

			actual, err := clusterClassExists(c, tt.clusterClass, metav1.NamespaceDefault)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(actual).To(Equal(tt.want))
		})
	}
}

func TestAddClusterClassIfMissing(t *testing.T) {
	infraClusterTemplateNS4 := unstructured.Unstructured{}
	infraClusterTemplateNS4.SetNamespace("ns4")
	infraClusterTemplateNS4.SetGroupVersionKind(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "InfrastructureClusterTemplate",
	})
	infraClusterTemplateNS4.SetName("testInfraClusterTemplate")
	infraClusterTemplateNS4Bytes, err := utilyaml.FromUnstructured([]unstructured.Unstructured{infraClusterTemplateNS4})
	if err != nil {
		panic("failed to convert template to bytes")
	}

	tests := []struct {
		name                        string
		clusterInitialized          bool
		objs                        []client.Object
		clusterClassTemplateContent []byte
		targetNamespace             string
		listVariablesOnly           bool
		wantClusterClassInTemplate  bool
		wantError                   bool
	}{
		{
			name:                        "should add the cluster class to the template if cluster is not initialized",
			clusterInitialized:          false,
			objs:                        []client.Object{},
			targetNamespace:             "ns1",
			clusterClassTemplateContent: clusterClassYAML("ns1", "dev"),
			listVariablesOnly:           false,
			wantClusterClassInTemplate:  true,
			wantError:                   false,
		},
		{
			name:                        "should add the cluster class to the template if cluster is initialized and cluster class is not installed",
			clusterInitialized:          true,
			objs:                        []client.Object{},
			targetNamespace:             "ns2",
			clusterClassTemplateContent: clusterClassYAML("ns2", "dev"),
			listVariablesOnly:           false,
			wantClusterClassInTemplate:  true,
			wantError:                   false,
		},
		{
			name:               "should NOT add the cluster class to the template if cluster is initialized and cluster class is installed",
			clusterInitialized: true,
			objs: []client.Object{&clusterv1.ClusterClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dev",
					Namespace: "ns3",
				},
			}},
			targetNamespace:             "ns3",
			clusterClassTemplateContent: clusterClassYAML("ns3", "dev"),
			listVariablesOnly:           false,
			wantClusterClassInTemplate:  false,
			wantError:                   false,
		},
		{
			name:               "should throw error if the cluster is initialized and templates from the cluster class template already exist in the cluster",
			clusterInitialized: true,
			objs: []client.Object{
				&infraClusterTemplateNS4,
			},
			clusterClassTemplateContent: utilyaml.JoinYaml(clusterClassYAML("ns4", "dev"), infraClusterTemplateNS4Bytes),
			targetNamespace:             "ns4",
			listVariablesOnly:           false,
			wantClusterClassInTemplate:  false,
			wantError:                   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config1 := newFakeConfig().WithProvider(infraProviderConfig)
			repository1 := newFakeRepository(infraProviderConfig, config1).
				WithPaths("root", "").
				WithDefaultVersion("v1.0.0").
				WithFile("v1.0.0", "clusterclass-dev.yaml", tt.clusterClassTemplateContent)
			cluster := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgt-cluster"}, config1).WithObjs(tt.objs...)

			if tt.clusterInitialized {
				cluster.WithObjs(&apiextensionsv1.CustomResourceDefinition{
					TypeMeta: metav1.TypeMeta{
						APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
						Kind:       "CustomResourceDefinition",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("clusters.%s", clusterv1.GroupVersion.Group),
						Labels: map[string]string{
							clusterv1.GroupVersion.String(): "v1beta1",
						},
					},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{
								Storage: true,
								Name:    clusterv1.GroupVersion.Version,
							},
						},
					},
				})
			}

			clusterClassClient := repository1.ClusterClasses("v1.0.0")

			clusterWithTopology := []byte(fmt.Sprintf("apiVersion: %s\n", clusterv1.GroupVersion.String()) +
				"kind: Cluster\n" +
				"metadata:\n" +
				"  name: cluster-dev\n" +
				fmt.Sprintf("  namespace: %s\n", tt.targetNamespace) +
				"spec:\n" +
				"  topology:\n" +
				"    class: dev")

			baseTemplate, err := repository.NewTemplate(repository.TemplateInput{
				RawArtifact:           clusterWithTopology,
				ConfigVariablesClient: test.NewFakeVariableClient(),
				Processor:             yaml.NewSimpleProcessor(),
				TargetNamespace:       tt.targetNamespace,
				SkipTemplateProcess:   false,
			})
			if err != nil {
				t.Fatalf("failed to create template %v", err)
			}

			g := NewWithT(t)
			template, err := addClusterClassIfMissing(baseTemplate, clusterClassClient, cluster, tt.targetNamespace, tt.listVariablesOnly)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
			} else {
				if tt.wantClusterClassInTemplate {
					g.Expect(template.Objs()).To(ContainElement(MatchClusterClass("dev", tt.targetNamespace)))
				} else {
					g.Expect(template.Objs()).NotTo(ContainElement(MatchClusterClass("dev", tt.targetNamespace)))
				}
			}
		})
	}
}

func MatchClusterClass(name, namespace string) types.GomegaMatcher {
	return &clusterClassMatcher{name, namespace}
}

type clusterClassMatcher struct {
	name      string
	namespace string
}

func (cm *clusterClassMatcher) Match(actual interface{}) (bool, error) {
	uClass := actual.(unstructured.Unstructured)

	// check name
	name, ok, err := unstructured.NestedString(uClass.UnstructuredContent(), "metadata", "name")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if name != cm.name {
		return false, nil
	}

	// check namespace
	namespace, ok, err := unstructured.NestedString(uClass.UnstructuredContent(), "metadata", "namespace")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if namespace != cm.namespace {
		return false, nil
	}

	return true, nil
}

func (cm *clusterClassMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected ClusterClass of name %v in namespace %v to be present", cm.name, cm.namespace)
}

func (cm *clusterClassMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected ClusterClass of name %v in namespace %v not to be present", cm.name, cm.namespace)
}
