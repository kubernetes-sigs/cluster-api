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

package machine

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	v1alpha1listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
)

func TestReconcileNode(t *testing.T) {
	tests := []struct {
		name                     string
		nodeHasMachineAnnotation bool
		nodeIsDeleting           bool
		nodeLinked               bool
		nodeCached               bool
		nodeCachedReady          bool
		nodeReady                bool
		nodeNotPresent           bool
		machineNotPresent        bool
		nodeRefName              string
		expectedErr              bool
		expectLinked             bool
		expectedActions          []string
	}{
		{
			name: "node doesn't exist, noop",
			nodeHasMachineAnnotation: true,
			nodeNotPresent:           true,
		},
		{
			name: "node with machine annotations, link",
			nodeHasMachineAnnotation: true,
			expectLinked:             true,
			expectedActions:          []string{"get", "update"},
		},
		{
			name: "node with no machine annotations, noop",
		},
		{
			name: "node with machine annotations, missing machine, err",
			nodeHasMachineAnnotation: true,
			machineNotPresent:        true,
			expectedErr:              true,
			expectedActions:          []string{"get"},
		},
		{
			name: "node being deleted, unlink",
			nodeHasMachineAnnotation: true,
			nodeIsDeleting:           true,
			nodeRefName:              "bar",
			nodeCached:               true,
			expectedActions:          []string{"get", "update"},
		},
		{
			name:           "node being deleted, no machine annotations, noop",
			nodeIsDeleting: true,
		},
		{
			name: "node being deleted, missing machine, err",
			nodeHasMachineAnnotation: true,
			nodeIsDeleting:           true,
			machineNotPresent:        true,
			expectedActions:          []string{"get"},
			expectedErr:              true,
		},
		{
			name: "node being deleted, no node ref, noop",
			nodeHasMachineAnnotation: true,
			nodeIsDeleting:           true,
			expectedActions:          []string{"get"},
		},
		{
			name: "node being deleted, node ref mismatch, noop",
			nodeHasMachineAnnotation: true,
			nodeIsDeleting:           true,
			nodeRefName:              "random",
			expectedActions:          []string{"get"},
		},
		{
			name:       "node cached and no change to ready state, noop",
			nodeCached: true,
			nodeReady:  true,
		},
		{
			name:                     "node cached ready and change to not ready state, update",
			nodeCached:               true,
			nodeCachedReady:          true,
			nodeReady:                false,
			nodeHasMachineAnnotation: true,
			expectLinked:             true,
			expectedActions:          []string{"get", "update"},
		},
		{
			name:                     "node cached not ready and change to ready state, update",
			nodeCached:               true,
			nodeCachedReady:          false,
			nodeReady:                true,
			nodeHasMachineAnnotation: true,
			expectLinked:             true,
			expectedActions:          []string{"get", "update"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := getMachine("foo", nil, false, false)
			if test.nodeRefName != "" {
				m.Status.NodeRef = &corev1.ObjectReference{
					Kind: "Node",
					Name: test.nodeRefName,
				}
			}
			mrObjects := []runtime.Object{}
			if !test.machineNotPresent {
				mrObjects = append(mrObjects, m)
			}

			machinesIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			err := machinesIndexer.Add(m)
			if err != nil {
				t.Fatal(err)
			}
			machineLister := v1alpha1listers.NewMachineLister(machinesIndexer)

			fakeClient := fake.NewSimpleClientset(mrObjects...)

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "bar",
					UID:         "the-uid",
					Annotations: make(map[string]string),
				},
			}
			if test.nodeReady {
				node.Status = corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						corev1.NodeCondition{
							Type:               corev1.NodeReady,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metav1.Now(),
						},
					},
				}
			}
			if test.nodeHasMachineAnnotation {
				node.ObjectMeta.Annotations[machineAnnotationKey] = "default/foo"
			}
			if test.nodeIsDeleting {
				node.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			}

			krObjects := []runtime.Object{}
			if !test.nodeNotPresent {
				krObjects = append(krObjects, node)
			}
			fakeK8sClient := k8sfake.NewSimpleClientset(krObjects...)

			target := &MachineControllerImpl{}
			target.clientSet = fakeClient
			target.kubernetesClientSet = fakeK8sClient
			target.lister = machineLister
			target.linkedNodes = make(map[string]bool)
			target.cachedReadiness = make(map[string]bool)
			if test.nodeCached {
				target.linkedNodes["bar"] = true
				target.cachedReadiness["bar"] = test.nodeCachedReady
			}

			nodeKey, err := cache.MetaNamespaceKeyFunc(node)
			if err != nil {
				t.Fatalf("unable to get key for test node, %v", err)
			}
			err = target.reconcileNode(nodeKey)

			if (err != nil) != test.expectedErr {
				t.Fatalf("got %v error, expected %v error", err, test.expectedErr)
			}

			cached := target.linkedNodes["bar"]
			if cached != test.expectLinked && !test.nodeCached {
				t.Errorf("got %v node cache result, expected %v node cache result", cached, test.expectLinked)
			}
			// Successful unlink should clear cache.
			if test.nodeIsDeleting && len(test.expectedActions) == 2 {
				if cached {
					t.Errorf("got %v node cache result, expected no node cache result", cached)
				}
			}
			if cached {
				isReady := target.cachedReadiness["bar"]
				// If successful, isRead
				if len(test.expectedActions) == 2 {
					if isReady != test.nodeReady {
						t.Errorf("got %v cached node ready, expected %v cached node ready", isReady, test.nodeReady)
					}
				} else {
					if isReady != test.nodeCachedReady {
						t.Errorf("got %v cached node ready, expected %v cached node ready", isReady, test.nodeCachedReady)
					}
				}
			}

			var actualMachine *v1alpha1.Machine
			actions := fakeClient.Actions()
			if len(actions) != len(test.expectedActions) {
				t.Fatalf("got %v actions, expected %v actions; got actions %v, expected actions %v", len(actions), len(test.expectedActions), actions, test.expectedActions)
			}

			for i, action := range test.expectedActions {
				if actions[i].GetVerb() != action {
					t.Errorf("got %v action verb, expected %v action verb", actions[i].GetVerb(), action)
				}
				if actions[i].GetVerb() == "update" {
					updateAction, ok := actions[i].(clienttesting.UpdateAction)
					if !ok {
						t.Fatalf("unexpected action %#v", action)
					}
					actualMachine, ok = updateAction.GetObject().(*v1alpha1.Machine)
					if !ok {
						t.Fatalf("unexpected object %#v", actualMachine)
					}
				}
			}
			if actualMachine != nil && actualMachine.Status.LastUpdated.IsZero() {
				t.Errorf("got %v last updated time, expected non-zero last updated time.", actualMachine.Status.LastUpdated)
			}
			if test.expectLinked {
				if actualMachine.Status.NodeRef == nil {
					t.Fatalf("got nil node ref, expected non-nil node ref")
				}
				if actualMachine.Status.NodeRef.Name != "bar" {
					t.Errorf("got %v node ref name, expected bar node ref name.", actualMachine.Status.NodeRef.Name)
				}
				if actualMachine.Status.NodeRef.UID != "the-uid" {
					t.Errorf("got '%v' node ref uid, expected 'the-uid' node ref uid", actualMachine.Status.NodeRef.UID)
				}
			}
			if !test.expectLinked {
				if actualMachine != nil && actualMachine.Status.NodeRef != nil {
					t.Fatalf("got non-nil node ref, expected nil node ref")
				}
			}
		})
	}
}
