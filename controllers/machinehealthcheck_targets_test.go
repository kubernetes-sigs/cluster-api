/*
Copyright 2020 The Kubernetes Authors.

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

package controllers

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestGetTargetsFromMHC(t *testing.T) {
	namespace := "test-mhc"
	clusterName := "test-cluster"
	mhcSelector := map[string]string{"cluster": clusterName, "machine-group": "foo"}

	// Create a namespace and cluster for the tests
	testNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "mhc-test"}}
	testCluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: clusterName}}

	// Create a test MHC
	testMHC := &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mhc",
			Namespace: namespace,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: mhcSelector,
			},
			ClusterName: testCluster.ObjectMeta.Name,
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
		},
	}

	baseObjects := []runtime.Object{testNS, testCluster, testMHC}

	// Initialise some test machines and nodes for use in the test cases

	testNode1 := newTestNode("node1")
	testMachine1 := newTestMachine("machine1", namespace, clusterName, testNode1.Name, mhcSelector)
	testNode2 := newTestNode("node2")
	testMachine2 := newTestMachine("machine2", namespace, clusterName, testNode2.Name, map[string]string{"cluster": clusterName})
	testNode3 := newTestNode("node3")
	testMachine3 := newTestMachine("machine3", namespace, clusterName, testNode3.Name, mhcSelector)
	testNode4 := newTestNode("node4")
	testMachine4 := newTestMachine("machine4", namespace, clusterName, testNode4.Name, mhcSelector)

	testCases := []struct {
		desc            string
		toCreate        []runtime.Object
		expectedTargets []healthCheckTarget
	}{
		{
			desc:            "with no matching machines",
			toCreate:        baseObjects,
			expectedTargets: nil,
		},
		{
			desc:     "when a machine's node is missing",
			toCreate: append(baseObjects, testMachine1),
			expectedTargets: []healthCheckTarget{
				{
					Machine: testMachine1,
					MHC:     testMHC,
					Node: &corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
						},
					},
				},
			},
		},
		{
			desc:     "when a machine's labels do not match",
			toCreate: append(baseObjects, testMachine1, testMachine2, testNode1),
			expectedTargets: []healthCheckTarget{
				{
					Machine: testMachine1,
					MHC:     testMHC,
					Node:    testNode1,
				},
			},
		},
		{
			desc:     "with multiple machines, should match correct nodes",
			toCreate: append(baseObjects, testNode1, testMachine1, testNode3, testMachine3, testNode4, testMachine4),
			expectedTargets: []healthCheckTarget{
				{
					Machine: testMachine1,
					MHC:     testMHC,
					Node:    testNode1,
				},
				{
					Machine: testMachine3,
					MHC:     testMHC,
					Node:    testNode3,
				},
				{
					Machine: testMachine4,
					MHC:     testMHC,
					Node:    testNode4,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			gs := NewGomegaWithT(t)

			gs.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
			k8sClient := fake.NewFakeClientWithScheme(scheme.Scheme, tc.toCreate...)

			// Create a test reconciler
			reconciler := &MachineHealthCheckReconciler{
				Client: k8sClient,
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			targets, err := reconciler.getTargetsFromMHC(k8sClient, testCluster, testMHC)
			gs.Expect(err).ToNot(HaveOccurred())

			gs.Expect(targets).To(ConsistOf(tc.expectedTargets))
		})
	}
}

func newTestMachine(name, namespace, clusterName, nodeName string, labels map[string]string) *clusterv1.Machine {
	bootstrap := "bootstrap"
	return &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
			Bootstrap: clusterv1.Bootstrap{
				Data: &bootstrap,
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name:      nodeName,
				Namespace: namespace,
			},
		},
	}
}

func newTestNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
