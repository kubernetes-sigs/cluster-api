/*
Copyright 2026 The Kubernetes Authors.

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

package ssa

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1beta1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMitigateManagedFieldsIssue(t *testing.T) {
	testFieldManager := "ssa-manager"
	otherFieldManager := "other-manager"

	machineDeployment := &clusterv1.MachineDeployment{
		// Have to set TypeMeta explicitly when using SSA with typed objects.
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineDeployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "md-1",
			Namespace: "default",
			Labels: map[string]string{
				"label-1": "value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "value-1",
			},
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName: "cluster-1",
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					ClusterName: "cluster-1",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: builder.InfrastructureGroupVersion.Group,
						Kind:     builder.TestInfrastructureMachineKind,
						Name:     "inframachine",
					},
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: ptr.To(""),
					},
				},
			},
		},
	}

	kubeadmControlPlane := &controlplanev1.KubeadmControlPlane{
		// Have to set TypeMeta explicitly when using SSA with typed objects.
		TypeMeta: metav1.TypeMeta{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "KubeadmControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kcp-1",
			Namespace: "default",
			Labels: map[string]string{
				"label-1": "value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "value-1",
			},
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				Format: bootstrapv1.CloudConfig,
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					APIServer: bootstrapv1.APIServer{
						ExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "v",
								Value: ptr.To("1"),
							},
							{
								Name:  "allow-privileged",
								Value: ptr.To("true"),
							},
						},
					},
					ControllerManager: bootstrapv1.ControllerManager{
						ExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "v",
								Value: ptr.To("2"),
							},
							{
								Name:  "profiling",
								Value: ptr.To("false"),
							},
						},
					},
					Scheduler: bootstrapv1.Scheduler{
						ExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "profiling",
								Value: ptr.To("false"),
							},
							{
								Name:  "v",
								Value: ptr.To("3"),
							},
						},
					},
					Etcd: bootstrapv1.Etcd{
						Local: bootstrapv1.LocalEtcd{
							ExtraArgs: []bootstrapv1.Arg{
								{
									Name:  "v",
									Value: ptr.To("4"),
								},
								{
									Name:  "auto-tls",
									Value: ptr.To("false"),
								},
							},
						},
					},
				},
				InitConfiguration: bootstrapv1.InitConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						KubeletExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "node-labels",
								Value: ptr.To("kubernetesVersion=1-34-0"),
							},
							{
								Name:  "v",
								Value: ptr.To("5"),
							},
						},
					},
				},
			},
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: builder.InfrastructureGroupVersion.Group,
						Kind:     builder.TestInfrastructureMachineTemplateKind,
						Name:     "inframachinetemplate1",
					},
				},
			},
			Replicas: ptr.To[int32](1),
			Version:  "v1.34.0",
			Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
				Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
					Type: controlplanev1.RollingUpdateStrategyType,
					RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
						MaxSurge: &intstr.IntOrString{
							IntVal: 1,
						},
					},
				},
			},
		},
	}
	kubeadmControlPlaneManagedFieldsAfterMitigation := trimSpaces(`{
"f:spec":{
	"f:kubeadmConfigSpec":{
		"f:clusterConfiguration":{
			"f:apiServer":{
				"f:extraArgs":{
					"k:{\"name\":\"allow-privileged\",\"value\":\"true\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					},
					"k:{\"name\":\"v\",\"value\":\"1\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					}
				}
			},
			"f:controllerManager":{
				"f:extraArgs":{
					"k:{\"name\":\"profiling\",\"value\":\"false\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					},
					"k:{\"name\":\"v\",\"value\":\"2\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					}
				}
			},
			"f:etcd":{
				"f:local":{
					"f:extraArgs":{
						"k:{\"name\":\"auto-tls\",\"value\":\"false\"}":{
							".":{},
							"f:name":{},
							"f:value":{}
						},
						"k:{\"name\":\"v\",\"value\":\"4\"}":{
							".":{},
							"f:name":{},
							"f:value":{}
						}
					}
				}
			},
			"f:scheduler":{
				"f:extraArgs":{
					"k:{\"name\":\"profiling\",\"value\":\"false\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					},
					"k:{\"name\":\"v\",\"value\":\"3\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					}
				}
			}
		},
		"f:initConfiguration":{
			"f:nodeRegistration":{
				"f:kubeletExtraArgs":{
					"k:{\"name\":\"node-labels\",\"value\":\"kubernetesVersion=1-34-0\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					},
					"k:{\"name\":\"v\",\"value\":\"5\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					}
				}
			}
		}
	}
}}`)
	kubeadmControlPlaneApply := &controlplanev1.KubeadmControlPlane{
		// Have to set TypeMeta explicitly when using SSA with typed objects.
		TypeMeta: metav1.TypeMeta{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "KubeadmControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kcp-1",
			Namespace: "default",
			Labels: map[string]string{
				"label-1": "value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "value-1",
			},
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				Format: bootstrapv1.CloudConfig,
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					APIServer: bootstrapv1.APIServer{
						ExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "allow-privileged",
								Value: ptr.To("true"),
							},
							{
								Name:  "profiling",
								Value: ptr.To("false"),
							},
						},
					},
					ControllerManager: bootstrapv1.ControllerManager{
						ExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "v",
								Value: ptr.To("2"),
							},
						},
					},
					Scheduler: bootstrapv1.Scheduler{
						ExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "profiling",
								Value: ptr.To("false"),
							},
							{
								Name:  "v",
								Value: ptr.To("6"),
							},
						},
					},
					Etcd: bootstrapv1.Etcd{
						Local: bootstrapv1.LocalEtcd{
							ExtraArgs: []bootstrapv1.Arg{
								{
									Name:  "v",
									Value: ptr.To("4"),
								},
							},
						},
					},
				},
				InitConfiguration: bootstrapv1.InitConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						KubeletExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "node-labels",
								Value: ptr.To("kubernetesVersion=1-35-0"),
							},
							{
								Name:  "v",
								Value: ptr.To("5"),
							},
						},
					},
				},
				JoinConfiguration: bootstrapv1.JoinConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						KubeletExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "node-labels",
								Value: ptr.To("kubernetesVersion=1-35-0"),
							},
							{
								Name:  "v",
								Value: ptr.To("6"),
							},
						},
					},
				},
			},
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: builder.InfrastructureGroupVersion.Group,
						Kind:     builder.TestInfrastructureMachineTemplateKind,
						Name:     "inframachinetemplate1",
					},
				},
			},
			Replicas: ptr.To[int32](1),
			Version:  "v1.35.0",
			Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
				Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
					Type: controlplanev1.RollingUpdateStrategyType,
					RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
						MaxSurge: &intstr.IntOrString{
							IntVal: 1,
						},
					},
				},
			},
		},
	}
	kubeadmControlPlaneManagedFieldsAfterApply := trimSpaces(`{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
},
"f:spec":{
	"f:kubeadmConfigSpec":{
		"f:clusterConfiguration":{
			"f:apiServer":{
				"f:extraArgs":{
					"k:{\"name\":\"allow-privileged\",\"value\":\"true\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					},
					"k:{\"name\":\"profiling\",\"value\":\"false\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					}
				}
			},
			"f:controllerManager":{
				"f:extraArgs":{
					"k:{\"name\":\"v\",\"value\":\"2\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					}
				}
			},
			"f:etcd":{
				"f:local":{
					"f:extraArgs":{
						"k:{\"name\":\"v\",\"value\":\"4\"}":{
							".":{},
							"f:name":{},
							"f:value":{}
						}
					}
				}
			},
			"f:scheduler":{
				"f:extraArgs":{
					"k:{\"name\":\"profiling\",\"value\":\"false\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					},
					"k:{\"name\":\"v\",\"value\":\"6\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					}
				}
			}
		},
		"f:format":{},
		"f:initConfiguration":{
			"f:nodeRegistration":{
				"f:kubeletExtraArgs":{
					"k:{\"name\":\"node-labels\",\"value\":\"kubernetesVersion=1-35-0\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					},
					"k:{\"name\":\"v\",\"value\":\"5\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					}
				}
			}
		},
		"f:joinConfiguration":{
			"f:nodeRegistration":{
				"f:kubeletExtraArgs":{
					"k:{\"name\":\"node-labels\",\"value\":\"kubernetesVersion=1-35-0\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					},
					"k:{\"name\":\"v\",\"value\":\"6\"}":{
						".":{},
						"f:name":{},
						"f:value":{}
					}
				}
			}
		}
	},
	"f:machineTemplate":{
		"f:spec":{
			"f:infrastructureRef":{
				"f:apiGroup":{},
				"f:kind":{},
				"f:name":{}
			}
		}
	},
	"f:replicas":{},
	"f:rollout":{
		"f:strategy":{
			"f:rollingUpdate":{
				"f:maxSurge":{}
			},
			"f:type":{}
		}
	},
	"f:version":{}
}}`)

	kubeadmControlPlaneV1Beta1 := &controlplanev1beta1.KubeadmControlPlane{
		// Have to set TypeMeta explicitly when using SSA with typed objects.
		TypeMeta: metav1.TypeMeta{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "KubeadmControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kcp-1",
			Namespace: "default",
			Labels: map[string]string{
				"label-1": "value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "value-1",
			},
		},
		Spec: controlplanev1beta1.KubeadmControlPlaneSpec{
			KubeadmConfigSpec: bootstrapv1beta1.KubeadmConfigSpec{
				Format: bootstrapv1beta1.CloudConfig,
				ClusterConfiguration: &bootstrapv1beta1.ClusterConfiguration{
					APIServer: bootstrapv1beta1.APIServer{
						ControlPlaneComponent: bootstrapv1beta1.ControlPlaneComponent{
							ExtraArgs: map[string]string{
								"v":                "1",
								"allow-privileged": "true",
							},
						},
					},
					ControllerManager: bootstrapv1beta1.ControlPlaneComponent{
						ExtraArgs: map[string]string{
							"v":         "2",
							"profiling": "false",
						},
					},
					Scheduler: bootstrapv1beta1.ControlPlaneComponent{
						ExtraArgs: map[string]string{
							"v":         "3",
							"profiling": "false",
						},
					},
					Etcd: bootstrapv1beta1.Etcd{
						Local: &bootstrapv1beta1.LocalEtcd{
							ExtraArgs: map[string]string{
								"v":        "4",
								"auto-tls": "false",
							},
						},
					},
				},
				InitConfiguration: &bootstrapv1beta1.InitConfiguration{
					NodeRegistration: bootstrapv1beta1.NodeRegistrationOptions{
						KubeletExtraArgs: map[string]string{
							"v":           "5",
							"node-labels": "kubernetesVersion=1-34-0",
						},
					},
				},
			},
			MachineTemplate: controlplanev1beta1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: builder.InfrastructureGroupVersion.String(),
					Kind:       builder.TestInfrastructureMachineTemplateKind,
					Name:       "inframachinetemplate1",
					Namespace:  "default",
				},
			},
			Replicas: ptr.To[int32](1),
			Version:  "v1.34.0",
			RolloutStrategy: &controlplanev1beta1.RolloutStrategy{
				Type: controlplanev1beta1.RollingUpdateStrategyType,
				RollingUpdate: &controlplanev1beta1.RollingUpdate{
					MaxSurge: &intstr.IntOrString{
						IntVal: 1,
					},
				},
			},
		},
	}
	kubeadmControlPlaneV1Beta1ManagedFieldsAfterMitigation := trimSpaces(`{
"f:spec":{
	"f:kubeadmConfigSpec":{
		"f:clusterConfiguration":{
			"f:apiServer":{
				"f:extraArgs":{
					"f:allow-privileged":{},
					"f:v":{}
				}
			},
			"f:controllerManager":{
				"f:extraArgs":{
					"f:profiling":{},
					"f:v":{}
				}
			},
			"f:etcd":{
				"f:local":{
					"f:extraArgs":{
						"f:auto-tls":{},
						"f:v":{}
					}
				}
			},
			"f:scheduler":{
				"f:extraArgs":{
					"f:profiling":{},
					"f:v":{}
				}
			}
		},
		"f:initConfiguration":{
			"f:nodeRegistration":{
				"f:kubeletExtraArgs":{
					"f:node-labels":{},
					"f:v":{}
				}
			}
		}
	}
}}`)
	kubeadmControlPlaneV1Beta1Apply := &controlplanev1beta1.KubeadmControlPlane{
		// Have to set TypeMeta explicitly when using SSA with typed objects.
		TypeMeta: metav1.TypeMeta{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "KubeadmControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kcp-1",
			Namespace: "default",
			Labels: map[string]string{
				"label-1": "value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "value-1",
			},
		},
		Spec: controlplanev1beta1.KubeadmControlPlaneSpec{
			KubeadmConfigSpec: bootstrapv1beta1.KubeadmConfigSpec{
				Format: bootstrapv1beta1.CloudConfig,
				ClusterConfiguration: &bootstrapv1beta1.ClusterConfiguration{
					APIServer: bootstrapv1beta1.APIServer{
						ControlPlaneComponent: bootstrapv1beta1.ControlPlaneComponent{
							ExtraArgs: map[string]string{
								"allow-privileged": "true",
								"profiling":        "false",
							},
						},
					},
					ControllerManager: bootstrapv1beta1.ControlPlaneComponent{
						ExtraArgs: map[string]string{
							"v": "2",
						},
					},
					Scheduler: bootstrapv1beta1.ControlPlaneComponent{
						ExtraArgs: map[string]string{
							"v":         "6",
							"profiling": "false",
						},
					},
					Etcd: bootstrapv1beta1.Etcd{
						Local: &bootstrapv1beta1.LocalEtcd{
							ExtraArgs: map[string]string{
								"v": "4",
							},
						},
					},
				},
				InitConfiguration: &bootstrapv1beta1.InitConfiguration{
					NodeRegistration: bootstrapv1beta1.NodeRegistrationOptions{
						KubeletExtraArgs: map[string]string{
							"v":           "5",
							"node-labels": "kubernetesVersion=1-35-0",
						},
					},
				},
				JoinConfiguration: &bootstrapv1beta1.JoinConfiguration{
					NodeRegistration: bootstrapv1beta1.NodeRegistrationOptions{
						KubeletExtraArgs: map[string]string{
							"v":           "6",
							"node-labels": "kubernetesVersion=1-35-0",
						},
					},
				},
			},
			MachineTemplate: controlplanev1beta1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: builder.InfrastructureGroupVersion.String(),
					Kind:       builder.TestInfrastructureMachineTemplateKind,
					Name:       "inframachinetemplate1",
					Namespace:  "default",
				},
			},
			Replicas: ptr.To[int32](1),
			Version:  "v1.35.0",
			RolloutStrategy: &controlplanev1beta1.RolloutStrategy{
				Type: controlplanev1beta1.RollingUpdateStrategyType,
				RollingUpdate: &controlplanev1beta1.RollingUpdate{
					MaxSurge: &intstr.IntOrString{
						IntVal: 1,
					},
				},
			},
		},
	}
	kubeadmControlPlaneManagedFieldsV1Beta1AfterApply := trimSpaces(`{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
},
"f:spec":{
	"f:kubeadmConfigSpec":{
		"f:clusterConfiguration":{
			"f:apiServer":{
				"f:extraArgs":{
					"f:allow-privileged":{},
					"f:profiling":{}
				}
			},
			"f:controllerManager":{
				"f:extraArgs":{
					"f:v":{}
				}
			},
			"f:dns":{},
			"f:etcd":{
				"f:local":{
					"f:extraArgs":{
						"f:v":{}
					}
				}
			},
			"f:networking":{},
			"f:scheduler":{
				"f:extraArgs":{
					"f:profiling":{},
					"f:v":{}
				}
			}
		},
		"f:format":{},
		"f:initConfiguration":{
			"f:localAPIEndpoint":{},
			"f:nodeRegistration":{
				"f:kubeletExtraArgs":{
					"f:node-labels":{},
					"f:v":{}
				}
			}
		},
		"f:joinConfiguration":{
			"f:discovery":{},
			"f:nodeRegistration":{
				"f:kubeletExtraArgs":{
					"f:node-labels":{},
					"f:v":{}
				}
			}
		}
	},
	"f:machineTemplate":{
		"f:infrastructureRef":{},
		"f:metadata":{}
	},
	"f:replicas":{},
	"f:rolloutStrategy":{
		"f:rollingUpdate":{
			"f:maxSurge":{}
		},
		"f:type":{}
	},
	"f:version":{}
}}`)

	type objectApply struct {
		object                client.Object
		expectedManagedFields []metav1.ManagedFieldsEntry
	}
	tests := []struct {
		name                             string
		object                           client.Object
		managedFields                    []metav1.ManagedFieldsEntry
		expectManagedFieldIssueMitigated bool
		expectedManagedFields            []metav1.ManagedFieldsEntry
		objectApplies                    []objectApply
	}{
		{
			name:   "Case 0: [fieldManager] => [fieldManager]",
			object: kubeadmControlPlane.DeepCopy(),
			managedFields: []metav1.ManagedFieldsEntry{{
				Manager:    testFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1.GroupVersion.String(),
				Time:       ptr.To(metav1.Now()),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
}}`))},
			}},
			expectManagedFieldIssueMitigated: false,
			// managedFields are not changed
			expectedManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:    testFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1.GroupVersion.String(),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
}}`))},
			}},
		},
		{
			name:                             "Case 1: [] => [fieldManager]",
			object:                           machineDeployment.DeepCopy(),
			managedFields:                    []metav1.ManagedFieldsEntry{{}}, // One empty entry sets managedFields on the object to empty
			expectManagedFieldIssueMitigated: true,
			expectedManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:    testFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: clusterv1.GroupVersion.String(),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:name":{}
}}`))},
			}},
		},
		{
			name:   "Case 2a: [before-first-apply] => [fieldManager]",
			object: kubeadmControlPlane.DeepCopy(),
			managedFields: []metav1.ManagedFieldsEntry{{
				Manager:    beforeFirstApplyManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1.GroupVersion.String(),
				Time:       ptr.To(metav1.Now()),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
}}`))},
			}},
			expectManagedFieldIssueMitigated: true,
			expectedManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:    testFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1.GroupVersion.String(),
				FieldsType: "FieldsV1",
				FieldsV1:   &metav1.FieldsV1{Raw: []byte(kubeadmControlPlaneManagedFieldsAfterMitigation)},
			}},
			objectApplies: []objectApply{{
				object: kubeadmControlPlaneApply.DeepCopy(),
				expectedManagedFields: []metav1.ManagedFieldsEntry{{
					Manager:    testFieldManager,
					Operation:  metav1.ManagedFieldsOperationApply,
					APIVersion: controlplanev1.GroupVersion.String(),
					FieldsType: "FieldsV1",
					FieldsV1:   &metav1.FieldsV1{Raw: []byte(kubeadmControlPlaneManagedFieldsAfterApply)},
				}}},
			},
		},
		{
			name:   "Case 2b: [before-first-apply,other managers ...] => [fieldManager,other managers ...] (+ transition to v1beta2)",
			object: kubeadmControlPlaneV1Beta1.DeepCopy(),
			managedFields: []metav1.ManagedFieldsEntry{{
				Manager:    beforeFirstApplyManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1beta1.GroupVersion.String(),
				Time:       ptr.To(metav1.Now()),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	}
}}`))},
			}, {
				Manager:    otherFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1beta1.GroupVersion.String(),
				Time:       ptr.To(metav1.Now()),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:labels":{
		"f:label-1":{}
	}
}}`))}}},
			expectManagedFieldIssueMitigated: true,
			expectedManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:    otherFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1beta1.GroupVersion.String(),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:labels":{
		"f:label-1":{}
	}
}}`))},
			}, {
				Manager:    testFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1beta1.GroupVersion.String(),
				FieldsType: "FieldsV1",
				FieldsV1:   &metav1.FieldsV1{Raw: []byte(kubeadmControlPlaneV1Beta1ManagedFieldsAfterMitigation)},
			}},
			objectApplies: []objectApply{
				{ // Apply v1beta1
					object: kubeadmControlPlaneV1Beta1Apply,
					expectedManagedFields: []metav1.ManagedFieldsEntry{{
						Manager:    otherFieldManager,
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: controlplanev1beta1.GroupVersion.String(),
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:labels":{
		"f:label-1":{}
	}
}}`))},
					}, {
						Manager:    testFieldManager,
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: controlplanev1beta1.GroupVersion.String(),
						FieldsType: "FieldsV1",
						FieldsV1:   &metav1.FieldsV1{Raw: []byte(kubeadmControlPlaneManagedFieldsV1Beta1AfterApply)},
					}},
				},
				{ // Apply v1beta2
					object: kubeadmControlPlaneApply,
					expectedManagedFields: []metav1.ManagedFieldsEntry{{
						Manager:    otherFieldManager,
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: controlplanev1beta1.GroupVersion.String(),
						FieldsType: "FieldsV1",
						FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:labels":{
		"f:label-1":{}
	}
}}`))},
					}, {
						Manager:    testFieldManager,
						Operation:  metav1.ManagedFieldsOperationApply,
						APIVersion: controlplanev1.GroupVersion.String(),
						FieldsType: "FieldsV1",
						FieldsV1:   &metav1.FieldsV1{Raw: []byte(kubeadmControlPlaneManagedFieldsAfterApply)},
					}},
				},
			},
		},
		{
			name:   "Case 2c: [before-first-apply,fieldManager,other managers ...] => [fieldManager,other managers ...]",
			object: kubeadmControlPlane.DeepCopy(),
			managedFields: []metav1.ManagedFieldsEntry{{
				Manager:    beforeFirstApplyManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1.GroupVersion.String(),
				Time:       ptr.To(metav1.Now()),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	}
}}`))},
			}, {
				Manager:    otherFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1.GroupVersion.String(),
				Time:       ptr.To(metav1.Now()),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:labels":{
		"f:label-1":{}
	}
}}`))}}, {
				Manager:    testFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1.GroupVersion.String(),
				Time:       ptr.To(metav1.Now()),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:finalizers":{
		".":{},
		"v:\"test.com/finalizer\"":{}
	}
}}`))}}},
			expectManagedFieldIssueMitigated: true,
			expectedManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:    otherFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1.GroupVersion.String(),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:labels":{
		"f:label-1":{}
	}
}}`))},
			}, {
				Manager:    testFieldManager,
				Operation:  metav1.ManagedFieldsOperationApply,
				APIVersion: controlplanev1.GroupVersion.String(),
				FieldsType: "FieldsV1",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(trimSpaces(`{
"f:metadata":{
	"f:finalizers":{
		".":{},
		"v:\"test.com/finalizer\"":{}
	}
}}`))}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			g := NewWithT(t)

			g.Expect(env.Client.Create(ctx, tt.object)).To(Succeed())
			t.Cleanup(func() {
				ctx := context.Background()
				g.Expect(env.CleanupAndWait(ctx, tt.object)).To(Succeed())
			})

			// Overwrite managedFields on the object with tt.managedFields.
			jsonPatch := []map[string]interface{}{
				{
					"op":    "replace",
					"path":  "/metadata/managedFields",
					"value": tt.managedFields,
				},
				{
					"op":    "replace",
					"path":  "/metadata/resourceVersion",
					"value": tt.object.GetResourceVersion(),
				},
			}
			patch, err := json.Marshal(jsonPatch)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(env.Client.Patch(ctx, tt.object, client.RawPatch(types.JSONPatchType, patch))).To(Succeed())
			// Verify managedField patch above worked.
			if reflect.DeepEqual(tt.managedFields, []metav1.ManagedFieldsEntry{{}}) {
				g.Expect(tt.object.GetManagedFields()).To(BeEmpty())
			} else {
				g.Expect(cleanupTime(tt.object.GetManagedFields())).To(BeComparableTo(cleanupTime(tt.managedFields)))
			}

			// Run mitigation code and verify managedFields are as expected afterward.
			managedFieldIssueMitigated, err := MitigateManagedFieldsIssue(ctx, env.GetClient(), tt.object, testFieldManager)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(managedFieldIssueMitigated).To(Equal(tt.expectManagedFieldIssueMitigated))
			g.Expect(cleanupTime(tt.object.GetManagedFields())).To(BeComparableTo(tt.expectedManagedFields))

			for _, apply := range tt.objectApplies {
				origObject := apply.object.DeepCopyObject()

				// Apply object and verify managedFields are as expected afterward.
				g.Expect(Patch(ctx, env.GetClient(), testFieldManager, apply.object)).To(Succeed())
				g.Expect(cleanupTime(apply.object.GetManagedFields())).To(BeComparableTo(apply.expectedManagedFields))

				// Verify spec is as expected
				origJSON, err := json.Marshal(origObject)
				g.Expect(err).ToNot(HaveOccurred())
				origJSONMap := map[string]any{}
				g.Expect(json.Unmarshal(origJSON, &origJSONMap)).To(Succeed())
				modifiedJSON, err := json.Marshal(apply.object)
				g.Expect(err).ToNot(HaveOccurred())
				modifiedJSONMap := map[string]any{}
				g.Expect(json.Unmarshal(modifiedJSON, &modifiedJSONMap)).To(Succeed())
				g.Expect(modifiedJSONMap["spec"]).To(BeComparableTo(origJSONMap["spec"]))
			}
		})
	}
}
