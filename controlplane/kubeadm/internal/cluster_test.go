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

package internal

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func podReady(isReady corev1.ConditionStatus) corev1.PodCondition {
	return corev1.PodCondition{
		Type:   corev1.PodReady,
		Status: isReady,
	}
}

type checkStaticPodReadyConditionTest struct {
	name       string
	conditions []corev1.PodCondition
}

func TestCheckStaticPodReadyCondition(t *testing.T) {
	table := []checkStaticPodReadyConditionTest{
		{
			name:       "pod is ready",
			conditions: []corev1.PodCondition{podReady(corev1.ConditionTrue)},
		},
	}
	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec:   corev1.PodSpec{},
				Status: corev1.PodStatus{Conditions: test.conditions},
			}
			if err := checkStaticPodReadyCondition(pod); err != nil {
				t.Fatalf("should not have gotten an error: %v", err)
			}
		})
	}
}

func TestCheckStaticPodNotReadyCondition(t *testing.T) {
	table := []checkStaticPodReadyConditionTest{
		{
			name: "no pod status",
		},
		{
			name:       "not ready pod status",
			conditions: []corev1.PodCondition{podReady(corev1.ConditionFalse)},
		},
	}
	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec:   corev1.PodSpec{},
				Status: corev1.PodStatus{Conditions: test.conditions},
			}
			if err := checkStaticPodReadyCondition(pod); err == nil {
				t.Fatal("should have returned an error")
			}
		})
	}
}

func TestControlPlaneIsHealthy(t *testing.T) {
	readyStatus := corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	workloadCluster := &cluster{
		client: &fakeClient{
			list: nodeListForTestControlPlaneIsHealthy(),
			get: map[string]interface{}{
				"kube-system/kube-apiserver-first-control-plane":           &corev1.Pod{Status: readyStatus},
				"kube-system/kube-apiserver-second-control-plane":          &corev1.Pod{Status: readyStatus},
				"kube-system/kube-apiserver-third-control-plane":           &corev1.Pod{Status: readyStatus},
				"kube-system/kube-controller-manager-first-control-plane":  &corev1.Pod{Status: readyStatus},
				"kube-system/kube-controller-manager-second-control-plane": &corev1.Pod{Status: readyStatus},
				"kube-system/kube-controller-manager-third-control-plane":  &corev1.Pod{Status: readyStatus},
			},
		},
	}

	health, err := workloadCluster.controlPlaneIsHealthy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(health) == 0 {
		t.Fatal("no nodes were checked")
	}
	if len(health) != len(nodeListForTestControlPlaneIsHealthy().Items) {
		t.Fatal("not all nodes were checked")
	}
}

func nodeListForTestControlPlaneIsHealthy() *corev1.NodeList {
	nodeNamed := func(name string) corev1.Node {
		return corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
	}
	return &corev1.NodeList{
		Items: []corev1.Node{
			nodeNamed("first-control-plane"),
			nodeNamed("second-control-plane"),
			nodeNamed("third-control-plane"),
		},
	}
}

func TestGetMachinesForCluster(t *testing.T) {
	m := ManagementCluster{Client: &fakeClient{
		list: machineListForTestGetMachinesForCluster(),
	}}
	clusterKey := types.NamespacedName{
		Namespace: "my-namespace",
		Name:      "my-cluster",
	}
	machines, err := m.GetMachinesForCluster(context.Background(), clusterKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 3 {
		t.Fatalf("expected 3 machines but found %d", len(machines))
	}

	// Test the OwnedControlPlaneMachines works
	machines, err = m.GetMachinesForCluster(context.Background(), clusterKey, OwnedControlPlaneMachines("my-control-plane"))
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 {
		t.Fatalf("expected 1 control plane machine but got %d", len(machines))
	}

	// Test that the filters use AND logic instead of OR logic
	nameFilter := func(cluster *clusterv1.Machine) bool {
		return cluster.Name == "first-machine"
	}
	machines, err = m.GetMachinesForCluster(context.Background(), clusterKey, OwnedControlPlaneMachines("my-control-plane"), nameFilter)
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 {
		t.Fatalf("expected 1 control plane machine but got %d", len(machines))
	}
}

func machineListForTestGetMachinesForCluster() *clusterv1.MachineList {
	owned := true
	ownedRef := []metav1.OwnerReference{
		{
			Kind:       "KubeadmControlPlane",
			Name:       "my-control-plane",
			Controller: &owned,
		},
	}
	machine := func(name string) clusterv1.Machine {
		return clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "my-namespace",
				Labels: map[string]string{
					clusterv1.ClusterLabelName: "my-cluster",
				},
			},
		}
	}
	controlPlaneMachine := machine("first-machine")
	controlPlaneMachine.ObjectMeta.Labels[clusterv1.MachineControlPlaneLabelName] = ""
	controlPlaneMachine.OwnerReferences = ownedRef

	return &clusterv1.MachineList{
		Items: []clusterv1.Machine{
			controlPlaneMachine,
			machine("second-machine"),
			machine("third-machine"),
		},
	}
}

type fakeClient struct {
	client.Client
	list interface{}
	get  map[string]interface{}
}

func (f *fakeClient) Get(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
	item := f.get[key.String()]
	switch l := item.(type) {
	case *corev1.Pod:
		l.DeepCopyInto(obj.(*corev1.Pod))
	default:
		return fmt.Errorf("unknown type: %s", l)
	}
	return nil
}

func (f *fakeClient) List(_ context.Context, list runtime.Object, _ ...client.ListOption) error {
	switch l := f.list.(type) {
	case *clusterv1.MachineList:
		l.DeepCopyInto(list.(*clusterv1.MachineList))
	case *corev1.NodeList:
		l.DeepCopyInto(list.(*corev1.NodeList))
	default:
		return fmt.Errorf("unknown type: %s", l)
	}
	return nil
}
