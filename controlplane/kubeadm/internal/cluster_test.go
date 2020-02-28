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
	"errors"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	workloadCluster := &Cluster{
		Client: &fakeClient{
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
	list      interface{}
	get       map[string]interface{}
	getErr    error
	createErr error
}

func (f *fakeClient) Get(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
	if f.getErr != nil {
		return f.getErr
	}
	item := f.get[key.String()]
	switch l := item.(type) {
	case *corev1.Pod:
		l.DeepCopyInto(obj.(*corev1.Pod))
	case *rbacv1.RoleBinding:
		l.DeepCopyInto(obj.(*rbacv1.RoleBinding))
	case *rbacv1.Role:
		l.DeepCopyInto(obj.(*rbacv1.Role))
	case nil:
		return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
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

func (f *fakeClient) Create(_ context.Context, _ runtime.Object, _ ...client.CreateOption) error {
	if f.createErr != nil {
		return f.createErr
	}
	return nil
}

func TestManagementCluster_healthCheck_NoError(t *testing.T) {
	tests := []struct {
		name             string
		machineList      *clusterv1.MachineList
		check            healthCheck
		clusterKey       types.NamespacedName
		controlPlaneName string
	}{
		{
			name: "simple",
			machineList: &clusterv1.MachineList{
				Items: []clusterv1.Machine{
					controlPlaneMachine("one"),
					controlPlaneMachine("two"),
					controlPlaneMachine("three"),
				},
			},
			check: func(ctx context.Context) (healthCheckResult, error) {
				return healthCheckResult{
					"one":   nil,
					"two":   nil,
					"three": nil,
				}, nil
			},
			clusterKey:       types.NamespacedName{Namespace: "default", Name: "cluster-name"},
			controlPlaneName: "control-plane-name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			m := &ManagementCluster{
				Client: &fakeClient{list: tt.machineList},
			}
			if err := m.healthCheck(ctx, tt.check, tt.clusterKey, tt.controlPlaneName); err != nil {
				t.Fatal("did not expect an error?")
			}
		})
	}
}

func TestManagementCluster_healthCheck_Errors(t *testing.T) {
	tests := []struct {
		name             string
		machineList      *clusterv1.MachineList
		check            healthCheck
		clusterKey       types.NamespacedName
		controlPlaneName string
		// expected errors will ensure the error contains this list of strings.
		// If not supplied, no check on the error's value will occur.
		expectedErrors []string
	}{
		{
			name: "machine's node was not checked for health",
			machineList: &clusterv1.MachineList{
				Items: []clusterv1.Machine{
					controlPlaneMachine("one"),
					controlPlaneMachine("two"),
					controlPlaneMachine("three"),
				},
			},
			check: func(ctx context.Context) (healthCheckResult, error) {
				return healthCheckResult{
					"one": nil,
				}, nil
			},
		},
		{
			name: "health check returns an error not related to the nodes health",
			machineList: &clusterv1.MachineList{
				Items: []clusterv1.Machine{
					controlPlaneMachine("one"),
					controlPlaneMachine("two"),
					controlPlaneMachine("three"),
				},
			},
			check: func(ctx context.Context) (healthCheckResult, error) {
				return healthCheckResult{
					"one":   nil,
					"two":   errors.New("two"),
					"three": errors.New("three"),
				}, errors.New("meta")
			},
			expectedErrors: []string{"two", "three", "meta"},
		},
		{
			name: "two nodes error on the check but no overall error occurred",
			machineList: &clusterv1.MachineList{
				Items: []clusterv1.Machine{
					controlPlaneMachine("one"),
					controlPlaneMachine("two"),
					controlPlaneMachine("three"),
				},
			},
			check: func(ctx context.Context) (healthCheckResult, error) {
				return healthCheckResult{
					"one":   nil,
					"two":   errors.New("two"),
					"three": errors.New("three"),
				}, nil
			},
			expectedErrors: []string{"two", "three"},
		},
		{
			name: "more nodes than machines were checked (out of band control plane nodes)",
			machineList: &clusterv1.MachineList{
				Items: []clusterv1.Machine{
					controlPlaneMachine("one"),
				},
			},
			check: func(ctx context.Context) (healthCheckResult, error) {
				return healthCheckResult{
					"one":   nil,
					"two":   nil,
					"three": nil,
				}, nil
			},
		},
		{
			name: "a machine that has a nil node reference",
			machineList: &clusterv1.MachineList{
				Items: []clusterv1.Machine{
					controlPlaneMachine("one"),
					controlPlaneMachine("two"),
					nilNodeRef(controlPlaneMachine("three")),
				},
			},
			check: func(ctx context.Context) (healthCheckResult, error) {
				return healthCheckResult{
					"one":   nil,
					"two":   nil,
					"three": nil,
				}, nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			clusterKey := types.NamespacedName{Namespace: "default", Name: "cluster-name"}
			controlPlaneName := "control-plane-name"

			m := &ManagementCluster{
				Client: &fakeClient{list: tt.machineList},
			}
			err := m.healthCheck(ctx, tt.check, clusterKey, controlPlaneName)
			if err == nil {
				t.Fatal("Expected an error")
			}
			for _, expectedError := range tt.expectedErrors {
				if !strings.Contains(err.Error(), expectedError) {
					t.Fatalf("Expected %q to contain %q", err.Error(), expectedError)
				}
			}
		})
	}
}

func controlPlaneMachine(name string) clusterv1.Machine {
	t := true
	return clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
			Labels:    ControlPlaneLabelsForCluster("cluster-name"),
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "KubeadmControlPlane",
					Name:       "control-plane-name",
					Controller: &t,
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: name,
			},
		},
	}
}

func nilNodeRef(machine clusterv1.Machine) clusterv1.Machine {
	machine.Status.NodeRef = nil
	return machine
}
