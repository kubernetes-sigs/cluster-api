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

package tree

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestNodeObjectDeepCopyObject(t *testing.T) {
	g := NewWithT(t)
	controller := true
	deletionTimestamp := metav1.NewTime(time.Unix(1, 0))
	transitionTime := metav1.NewTime(time.Unix(2, 0))
	legacyTransitionTime := metav1.NewTime(time.Unix(3, 0))

	node := VirtualObject("default", "WorkerGroup", "workers")
	node.Labels = map[string]string{"app": "cluster-api"}
	node.Annotations["description"] = "original"
	node.OwnerReferences = []metav1.OwnerReference{{Name: "cluster", Controller: &controller}}
	node.DeletionTimestamp = &deletionTimestamp
	node.Finalizers = []string{"tree.cluster.x-k8s.io/finalizer"}
	node.Status = NodeStatus{
		Conditions: []metav1.Condition{{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: transitionTime,
			Message:            "ready",
		}},
		Deprecated: &NodeDeprecatedStatus{
			V1Beta1: &NodeV1Beta1DeprecatedStatus{
				Conditions: []clusterv1.Condition{{
					Type:               "Ready",
					Status:             corev1.ConditionTrue,
					LastTransitionTime: legacyTransitionTime,
					Message:            "legacy ready",
				}},
			},
		},
	}

	copied := node.DeepCopyObject().(*NodeObject)
	g.Expect(copied).To(Equal(node))

	copied.Labels["app"] = "changed"
	copied.Annotations["description"] = "changed"
	copied.OwnerReferences[0].Controller = nil
	copied.DeletionTimestamp.Time = time.Unix(10, 0)
	copied.Finalizers[0] = "changed"
	copied.Status.Conditions[0].Message = "changed"
	copied.Status.Deprecated.V1Beta1.Conditions[0].Message = "changed"

	g.Expect(node.Labels["app"]).To(Equal("cluster-api"))
	g.Expect(node.Annotations["description"]).To(Equal("original"))
	g.Expect(node.OwnerReferences[0].Controller).NotTo(BeNil())
	g.Expect(node.DeletionTimestamp.Time).To(Equal(time.Unix(1, 0)))
	g.Expect(node.Finalizers[0]).To(Equal("tree.cluster.x-k8s.io/finalizer"))
	g.Expect(node.Status.Conditions[0].Message).To(Equal("ready"))
	g.Expect(node.Status.Deprecated.V1Beta1.Conditions[0].Message).To(Equal("legacy ready"))
}

func TestNodeObjectDeepCopyObjectNil(t *testing.T) {
	var node *NodeObject

	if got := node.DeepCopyObject(); got != nil {
		t.Fatalf("DeepCopyObject() returned a non-nil runtime.Object for a nil receiver: %#v", got)
	}
}
