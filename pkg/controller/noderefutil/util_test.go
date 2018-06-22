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

package noderefutil

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsNodeAvaialble(t *testing.T) {
	tests := []struct {
		name              string
		node              *corev1.Node
		minReadySeconds   int32
		expectedAvailable bool
	}{
		{
			name:              "no node",
			expectedAvailable: false,
		},
		{
			name:              "no status",
			node:              &corev1.Node{},
			expectedAvailable: false,
		},
		{
			name:              "no condition",
			node:              &corev1.Node{Status: corev1.NodeStatus{}},
			expectedAvailable: false,
		},
		{
			name: "no ready condition",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeOutOfDisk,
						Status: corev1.ConditionTrue,
					},
				}},
			},
			expectedAvailable: false,
		},
		{
			name: "ready condition true, minReadySeconds = 0, lastTransitionTime now",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:               corev1.NodeReady,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				}},
			},
			expectedAvailable: true,
		},
		{
			name: "ready condition true, minReadySeconds = 0, lastTransitionTime past",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:               corev1.NodeReady,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-700) * time.Second)},
					},
				}},
			},
			expectedAvailable: true,
		},
		{
			name: "ready condition true, minReadySeconds = 300, lastTransitionTime now",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:               corev1.NodeReady,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				}},
			},
			minReadySeconds:   300,
			expectedAvailable: false,
		},
		{
			name: "ready condition true, minReadySeconds = 300, lastTransitionTime past",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:               corev1.NodeReady,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-700) * time.Second)},
					},
				}},
			},
			minReadySeconds:   300,
			expectedAvailable: true,
		},
		{
			name: "ready condition false",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				}},
			},
			expectedAvailable: false,
		},
		{
			name: "ready condition unknown",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionUnknown,
					},
				}},
			},
			expectedAvailable: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isAvailable := IsNodeAvailable(test.node, test.minReadySeconds, metav1.Now())
			if isAvailable != test.expectedAvailable {
				t.Fatalf("got %v available, expected %v available", isAvailable, test.expectedAvailable)
			}
		})
	}
}

func TestGetReadyCondition(t *testing.T) {
	tests := []struct {
		name              string
		nodeStatus        *corev1.NodeStatus
		expectedCondition *corev1.NodeCondition
	}{
		{
			name: "no status",
		},
		{
			name:       "no condition",
			nodeStatus: &corev1.NodeStatus{},
		},
		{
			name: "no ready condition",
			nodeStatus: &corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeOutOfDisk,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		{
			name: "ready condition true",
			nodeStatus: &corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
			expectedCondition: &corev1.NodeCondition{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
		{
			name: "ready condition false",
			nodeStatus: &corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
			expectedCondition: &corev1.NodeCondition{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			},
		},
		{
			name: "ready condition unknown",
			nodeStatus: &corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionUnknown,
					},
				},
			},
			expectedCondition: &corev1.NodeCondition{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionUnknown,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := GetReadyCondition(test.nodeStatus)
			if !reflect.DeepEqual(c, test.expectedCondition) {
				t.Fatalf("got %v condition, expected %v condition", c, test.expectedCondition)
			}
		})
	}
}

func TestIsNodeReady(t *testing.T) {
	tests := []struct {
		name          string
		node          *corev1.Node
		expectedReady bool
	}{
		{
			name:          "no node",
			expectedReady: false,
		},
		{
			name:          "no status",
			node:          &corev1.Node{},
			expectedReady: false,
		},
		{
			name:          "no condition",
			node:          &corev1.Node{Status: corev1.NodeStatus{}},
			expectedReady: false,
		},
		{
			name: "no ready condition",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeOutOfDisk,
						Status: corev1.ConditionTrue,
					},
				}},
			},
			expectedReady: false,
		},
		{
			name: "ready condition true",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				}},
			},
			expectedReady: true,
		},
		{
			name: "ready condition false",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				}},
			},
			expectedReady: false,
		},
		{
			name: "ready condition unknown",
			node: &corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					corev1.NodeCondition{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionUnknown,
					},
				}},
			},
			expectedReady: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isReady := IsNodeReady(test.node)
			if isReady != test.expectedReady {
				t.Fatalf("got %v ready, expected %v ready", isReady, test.expectedReady)
			}
		})
	}
}
