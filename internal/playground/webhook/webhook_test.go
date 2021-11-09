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

package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apiserver/pkg/storage/names"
	utilfeature "k8s.io/component-base/featuregate/testing"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: this is not intended to be merged. Just easier to play around with then manually via command-line

const (
	clusterWebhookNamespace = "test-cluster-webhook"
)

func TestWebhook(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	ns, err := env.CreateNamespace(ctx, clusterWebhookNamespace)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := env.Delete(ctx, ns); err != nil {
			t.Fatal(err)
		}
	}()

	if err := env.Create(ctx, builder.GenericControlPlaneTemplateCRD); err != nil && !apierrors.IsAlreadyExists(err) {
		panic(err)
	}
	if err := env.Create(ctx, builder.GenericInfrastructureClusterTemplateCRD); err != nil && !apierrors.IsAlreadyExists(err) {
		panic(err)
	}
	if err := env.Create(ctx, myControlPlane(ns.Name)); err != nil && !apierrors.IsAlreadyExists(err) {
		panic(err)
	}
	if err := env.Create(ctx, myInfraClusterTemplate(ns.Name)); err != nil && !apierrors.IsAlreadyExists(err) {
		panic(err)
	}

	tests := []struct {
		name               string
		variableClasses    string
		variableTopologies string
		// If wantDefaultedVariableTopologies is unset, it is the same as variables.
		wantDefaultedVariableTopologies string
		wantErrCreateClusterClass       bool
		wantErrCreateCluster            bool
	}{
		// Schema validation.
		{
			name: "integer",
			variableClasses: `[{
				"name": "cpu",
				"required": true,
				"schema": {
					"openAPIV3Schema": {
    					"type": "integer",
						"minimum": 1
					}
				}
			}]`,
			variableTopologies: `[{
				"name": "cpu",
				"value": 1
			}]`,
		},
		{
			name: "integer, fails because of exclusiveMinimum",
			variableClasses: `[{
				"name": "cpu",
				"required": true,
				"schema": {
					"openAPIV3Schema": {
    					"type": "integer",
						"minimum": 1,
						"exclusiveMinimum": true
					}
				}
			}]`,
			variableTopologies: `[{
				"name": "cpu",
				"value": 1
			}]`,
			wantErrCreateCluster: true,
		},
		{
			name: "integer, fails because of minimum",
			variableClasses: `[{
				"name": "cpu",
				"required": true,
				"schema": {
					"openAPIV3Schema": {
    					"type": "integer",
						"minimum": 2
					}
				}
			}]`,
			variableTopologies: `[{
				"name": "cpu",
				"value": 1
			}]`,
			wantErrCreateCluster: true,
		},
		{
			name: "integer, but got string",
			variableClasses: `[{
				"name": "cpu",
				"required": true,
				"schema": {
					"openAPIV3Schema": {
						"type": "integer",
						"minimum": 1
					}
				}
			}]`,
			variableTopologies: `[{
				"name": "cpu",
				"value": "1"
			}]`,
			wantErrCreateCluster: true,
		},
		{
			name: "string",
			variableClasses: `[{
				"name": "location",
				"required": true,
				"schema": {
					"openAPIV3Schema": {
						"type": "string",
						"minLength": 1
					}
				}
			}]`,
			variableTopologies: `[{
				"name": "location",
				"value": "us-east"
			}]`,
		},
		//{ //{ FIXME: objets are not supported anymore
		//	name: "object",
		//	variableClasses: `[{
		//		"name": "machineType",
		//		"required": true,
		//		"schema": {
		//			"openAPIV3Schema": {
		//				"type": "object",
		//				"properties": {
		//					"cpu": {
		//						"type": "integer",
		//						"minimum": 1
		//					},
		//					"gpu": {
		//						"type": "integer",
		//						"minimum": 1
		//					}
		//				}
		//			}
		//		}
		//	}]`,
		//	variableTopologies: `[{
		//		"name": "machineType",
		//	    "value": {
		//			"cpu": 8,
		//			"gpu": 1
		//		}
		//	}]`,
		//},
		//{ //{ FIXME: objets are not supported anymore
		//	name: "object, but got nested string instead of integer",
		//	variableClasses: `[{
		//		"name": "machineType",
		//		"required": true,
		//		"schema": {
		//			"openAPIV3Schema": {
		//				"type": "object",
		//				"properties": {
		//					"cpu": {
		//						"type": "integer",
		//						"minimum": 1
		//					},
		//					"gpu": {
		//						"type": "integer",
		//						"minimum": 1
		//					}
		//				}
		//			}
		//		}
		//	}]`,
		//	variableTopologies: `[{
		//		"name": "machineType",
		//		"value": {
		//			"cpu": 8,
		//			"gpu": "1"
		//		}
		//	}]`,
		//	wantErrCreateCluster: true,
		//},
		// Defaulting.
		//{ //{ FIXME: objets are not supported anymore
		//	name: "object with defaulting",
		//	variableClasses: `[{
		//		"name": "defaultObject",
		//		"required": true,
		//		"schema": {
		//			"openAPIV3Schema": {
		//				"type": "object",
		//				"default": {
		//					"foo": 5,
		//					"bar": "defaultStringValueObject"
		//				},
		//				"properties": {
		//					"foo": {
		//						"type": "integer"
		//					},
		//					"bar": {
		//						"type": "string",
		//						"default": "defaultStringValueNested"
		//					},
		//					"baz": {
		//						"type": "string",
		//						"default": "defaultStringValueNested"
		//					}
		//				}
		//			}
		//		}
		//	}]`,
		//	variableTopologies: `[]`,
		//	wantDefaultedVariableTopologies: `[{
		//		"name": "defaultObject",
		//		"value": {
		//			"bar": "defaultStringValueObject",
		//			"baz": "defaultStringValueNested",
		//			"foo": 5
		//		}
		//	}]`,
		//},
		//{ //{ FIXME: objets are not supported anymore
		//	name: "object with nested default, but variable not set",
		//	// Note: defaulting should only default if the parent exists.
		//	variableClasses: `[{
		//		"name": "defaultObject",
		//		"required": false,
		//		"schema": {
		//			"openAPIV3Schema": {
		//				"type": "object",
		//				"properties": {
		//					"bar": {
		//						"type": "string",
		//						"default": "defaultStringValueNested"
		//					}
		//				}
		//			}
		//		}
		//	}]`,
		//	variableTopologies:              `[]`,
		//	wantDefaultedVariableTopologies: `[]`,
		//},
		//{ //{ FIXME: objets are not supported anymore
		//	name: "object with nested defaults and pruning",
		//	// Note: https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#defaulting-and-nullable
		//	variableClasses: `[{
		//		"name": "defaultObject",
		//		"required": false,
		//		"schema": {
		//			"openAPIV3Schema": {
		//				"type": "object",
		//				"properties": {
		//					"foo": {
		//						"type": "string",
		//						"nullable": false,
		//						"default": "default"
		//					},
		//					"bar": {
		//						"type": "string",
		//						"nullable": true,
		//						"default": "default"
		//					},
		//					"bat": {
		//						"type": "string",
		//						"nullable": true
		//					},
		//					"baz": {
		//						"type": "string",
		//						"nullable": false
		//					}
		//				}
		//			}
		//		}
		//	}]`,
		//	variableTopologies: `[{
		//		"name": "defaultObject",
		//		"value": {
		//			"foo": null,
		//			"bar": null,
		//			"bat": null,
		//			"baz": null
		//		}
		//	}]`,
		//	wantDefaultedVariableTopologies: `[{
		//		"name": "defaultObject",
		//		"value": {
		//			"bar": null,
		//			"bat": null,
		//			"foo": "default"
		//		}
		//	}]`,
		//},
	}

	// Enable kube-openapi debug log
	_ = os.Setenv("SWAGGER_DEBUG", "true")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			clusterClass := myClusterClass(ns.Name)
			clusterClass.Spec.Variables = toVariableClasses(g, tt.variableClasses)

			cluster := myCluster(ns.Name)
			cluster.Spec.Topology.Class = clusterClass.Name
			cluster.Spec.Topology.Variables = toVariableTopologies(g, tt.variableTopologies)

			err := env.Create(ctx, clusterClass)
			if tt.wantErrCreateClusterClass {
				g.Expect(err).ToNot(BeNil())
			} else {
				g.Expect(err).To(BeNil())
			}

			err = env.Create(ctx, cluster)
			if tt.wantErrCreateCluster {
				g.Expect(err).ToNot(BeNil())
				return
			}

			g.Expect(err).To(BeNil())

			var gotCluster clusterv1.Cluster
			t.Log("get Cluster")
			g.Eventually(func() error {
				return env.Get(ctx, client.ObjectKeyFromObject(cluster), &gotCluster)
			}, time.Second*30).Should(Succeed())
			t.Log("got Cluster")

			wantVariables := tt.wantDefaultedVariableTopologies
			if tt.wantDefaultedVariableTopologies == "" {
				wantVariables = tt.variableTopologies
			}
			// Remove whitespace from wantDefaultedVariableTopologies JSON.
			var buffer bytes.Buffer
			if err := json.Compact(&buffer, []byte(wantVariables)); err != nil {
				fmt.Println(err)
			}
			wantVariablesCompact := buffer.String()
			// When we want an empty variable array it will actually be nil because of omitempty.
			if wantVariablesCompact == "[]" {
				wantVariablesCompact = "null"
			}

			gotVariables, err := json.Marshal(gotCluster.Spec.Topology.Variables)
			g.Expect(err).To(BeNil())
			g.Expect(string(gotVariables)).To(Equal(wantVariablesCompact))

			t.Log("delete Cluster")
			g.Expect(env.Delete(ctx, cluster)).To(Succeed())
			t.Log("delete ClusterClass")
			g.Expect(env.Delete(ctx, clusterClass)).To(Succeed())
		})
	}
}

func toVariableClasses(g *WithT, rawJSON string) []clusterv1.ClusterClassVariable {
	var ret []clusterv1.ClusterClassVariable
	g.Expect(json.Unmarshal([]byte(rawJSON), &ret)).To(Succeed())
	return ret
}

func toVariableTopologies(g *WithT, rawJSON string) []clusterv1.ClusterVariable {
	var ret []clusterv1.ClusterVariable
	g.Expect(json.Unmarshal([]byte(rawJSON), &ret)).To(Succeed())
	return ret
}

// Note: only for manual testing, obviously not intended to be merged, because we'll have testtypes instead

func myCluster(namespace string) *clusterv1.Cluster {
	return &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName("my-cluster-"),
			Namespace: namespace,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: &clusterv1.Topology{
				Version: "v1.22.0",
			},
		},
	}
}

func myClusterClass(namespace string) *clusterv1.ClusterClass {
	return &clusterv1.ClusterClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName("my-cluster-class-"),
			Namespace: namespace,
		},
		Spec: clusterv1.ClusterClassSpec{
			ControlPlane: clusterv1.ControlPlaneClass{
				LocalObjectTemplate: clusterv1.LocalObjectTemplate{
					Ref: &corev1.ObjectReference{
						APIVersion: builder.ControlPlaneGroupVersion.String(),
						Kind:       "GenericControlPlaneTemplate",
						Name:       "my-controlplane",
					},
				},
			},
			Infrastructure: clusterv1.LocalObjectTemplate{
				Ref: &corev1.ObjectReference{
					APIVersion: builder.InfrastructureGroupVersion.String(),
					Kind:       "GenericInfrastructureClusterTemplate",
					Name:       "my-infra-cluster-template",
				},
			},
		},
	}
}

func myControlPlane(namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(builder.ControlPlaneGroupVersion.String())
	obj.SetKind("GenericControlPlaneTemplate")
	obj.SetNamespace(namespace)
	obj.SetName("my-controlplane")
	return obj
}

func myInfraClusterTemplate(namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(builder.InfrastructureGroupVersion.String())
	obj.SetKind("GenericInfrastructureClusterTemplate")
	obj.SetNamespace(namespace)
	obj.SetName("my-infra-cluster-template")
	return obj
}
