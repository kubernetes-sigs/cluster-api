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

package machinedeployment

import (
	"strconv"
	"testing"
	"time"

	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

const timeout = time.Second * 5
const pollingInterval = 10 * time.Millisecond

func TestReconcile(t *testing.T) {
	labels := map[string]string{"foo": "bar"}
	deployment := &clusterv1alpha1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: clusterv1alpha1.MachineDeploymentSpec{
			MinReadySeconds:      int32Ptr(0),
			Replicas:             int32Ptr(2),
			RevisionHistoryLimit: int32Ptr(0),
			Selector:             metav1.LabelSelector{MatchLabels: labels},
			Strategy: &clusterv1alpha1.MachineDeploymentStrategy{
				Type: common.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1alpha1.MachineRollingUpdateDeployment{
					MaxUnavailable: intstrPtr(0),
					MaxSurge:       intstrPtr(1),
				},
			},
			Template: clusterv1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: clusterv1alpha1.MachineSpec{
					Versions: clusterv1alpha1.MachineVersionInfo{Kubelet: "1.10.3"},
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Errorf("error creating new manager: %v", err)
	}
	c = mgr.GetClient()

	r := newReconciler(mgr)
	recFn, requests, errors := SetupTestReconcile(r)
	if err := add(mgr, recFn, r.MachineSetToDeployments); err != nil {
		t.Errorf("error adding controller to manager: %v", err)
	}
	defer close(StartTestManager(mgr, t))

	// Create the MachineDeployment object and expect Reconcile to be called.
	if err := c.Create(context.TODO(), deployment); err != nil {
		t.Errorf("error creating instance: %v", err)
	}
	defer c.Delete(context.TODO(), deployment)
	expectReconcile(t, requests, errors)

	// Verify that the MachineSet was created.
	machineSets := &clusterv1alpha1.MachineSetList{}
	expectInt(t, 1, func(ctx context.Context) int {
		if err := c.List(ctx, &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		return len(machineSets.Items)
	})

	ms := machineSets.Items[0]
	if r := *ms.Spec.Replicas; r != 2 {
		t.Errorf("replicas was %d not 2", r)
	}
	if k := ms.Spec.Template.Spec.Versions.Kubelet; k != "1.10.3" {
		t.Errorf("kubelet was %q not '1.10.3'", k)
	}

	// Delete a MachineSet and expect Reconcile to be called to replace it.
	if err := c.Delete(context.TODO(), &ms); err != nil {
		t.Errorf("error deleting machineset: %v", err)
	}
	expectReconcile(t, requests, errors)
	expectInt(t, 1, func(ctx context.Context) int {
		if err := c.List(ctx, &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		return len(machineSets.Items)
	})

	// Scale a MachineDeployment and expect Reconcile to be called
	if err := updateMachineDeployment(c, deployment, func(d *clusterv1alpha1.MachineDeployment) { d.Spec.Replicas = int32Ptr(5) }); err != nil {
		t.Errorf("error scaling machinedeployment: %v", err)
	}
	if err := c.Update(context.TODO(), deployment); err != nil {
		t.Errorf("error updating instance: %v", err)
	}
	expectReconcile(t, requests, errors)
	expectInt(t, 5, func(ctx context.Context) int {
		if err := c.List(ctx, &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		if len(machineSets.Items) != 1 {
			return -1
		}
		return int(*machineSets.Items[0].Spec.Replicas)
	})

	// Update a MachineDeployment, expect Reconcile to be called and a new MachineSet to appear
	if err := updateMachineDeployment(c, deployment, func(d *clusterv1alpha1.MachineDeployment) { d.Spec.Template.Labels["updated"] = "true" }); err != nil {
		t.Errorf("error scaling machinedeployment: %v", err)
	}
	if err := c.Update(context.TODO(), deployment); err != nil {
		t.Errorf("error updating instance: %v", err)
	}
	expectReconcile(t, requests, errors)
	expectInt(t, 2, func(ctx context.Context) int {
		if err := c.List(ctx, &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		return len(machineSets.Items)
	})

	// Wait for the new MachineSet to get scaled up and set .Status.Replicas and .Status.AvailableReplicas
	// at each step
	var newMachineSet, oldMachineSet *clusterv1alpha1.MachineSet
	resourceVersion0, err0 := strconv.Atoi(machineSets.Items[0].ResourceVersion)
	resourceVersion1, err1 := strconv.Atoi(machineSets.Items[1].ResourceVersion)
	if err0 != nil {
		t.Fatalf("Unable to convert MS %q ResourceVersion to a number: %v", machineSets.Items[0].Name, err0)
	}
	if err1 != nil {
		t.Fatalf("Unable to convert MS %q ResourceVersion to a number: %v", machineSets.Items[1].Name, err1)
	}

	if resourceVersion0 > resourceVersion1 {
		newMachineSet = &machineSets.Items[0]
		oldMachineSet = &machineSets.Items[1]
	} else {
		newMachineSet = &machineSets.Items[1]
		oldMachineSet = &machineSets.Items[0]
	}

	// Start off by setting .Status.Replicas and .Status.AvailableReplicas of the old MachineSet
	oldMachineSet.Status.AvailableReplicas = *oldMachineSet.Spec.Replicas
	oldMachineSet.Status.Replicas = *oldMachineSet.Spec.Replicas
	if err := c.Status().Update(context.TODO(), oldMachineSet); err != nil {
		t.Errorf("error updating machineset: %v", err)
	}
	expectReconcile(t, requests, errors)

	// Iterate over the scalesteps
	step := 1
	for step < 5 {
		// Wait for newMachineSet to be scaled up
		expectTrue(t, func(ctx context.Context) bool {
			if err = c.Get(ctx, types.NamespacedName{
				Namespace: newMachineSet.Namespace, Name: newMachineSet.Name}, newMachineSet); err != nil {
				return false
			}

			currentReplicas := int(*newMachineSet.Spec.Replicas)
			step = currentReplicas
			return currentReplicas >= step && currentReplicas < 6
		})

		newMachineSet.Status.Replicas = *newMachineSet.Spec.Replicas
		newMachineSet.Status.AvailableReplicas = *newMachineSet.Spec.Replicas
		if err := c.Status().Update(context.TODO(), newMachineSet); err != nil {
			t.Logf("error updating machineset: %v", err)
		}
		expectReconcile(t, requests, errors)

		// Wait for oldMachineSet to be scaled down
		updateOldMachineSet := true
		expectTrue(t, func(ctx context.Context) bool {
			if err := c.Get(ctx, types.NamespacedName{Namespace: oldMachineSet.Namespace, Name: oldMachineSet.Name}, oldMachineSet); err != nil {
				if apierrors.IsNotFound(err) {
					updateOldMachineSet = false
					return true
				}
				return false
			}

			return int(*oldMachineSet.Spec.Replicas) <= 5-step
		})

		if updateOldMachineSet {
			oldMachineSet.Status.Replicas = *oldMachineSet.Spec.Replicas
			oldMachineSet.Status.AvailableReplicas = *oldMachineSet.Spec.Replicas
			oldMachineSet.Status.ObservedGeneration = oldMachineSet.Generation
			if err := c.Status().Update(context.TODO(), oldMachineSet); err != nil {
				t.Logf("error updating machineset: %v", err)
			}
			expectReconcile(t, requests, errors)
		}
	}

	// Expect the old MachineSet to be removed
	expectInt(t, 1, func(ctx context.Context) int {
		if err := c.List(ctx, &client.ListOptions{}, machineSets); err != nil {
			return -1
		}
		return len(machineSets.Items)
	})
}

func int32Ptr(i int32) *int32 {
	return &i
}

func intstrPtr(i int32) *intstr.IntOrString {
	// FromInt takes an int that must not be greater than int32...
	intstr := intstr.FromInt(int(i))
	return &intstr
}

func expectReconcile(t *testing.T, requests chan reconcile.Request, errors chan error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

LOOP:
	for range time.Tick(pollingInterval) {
		select {
		case recv := <-requests:
			if recv == expectedRequest {
				break LOOP
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting reconcile request")
		}
	}

	for range time.Tick(pollingInterval) {
		select {
		case err := <-errors:
			if err == nil {
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting reconcile error")
		}
	}
}

func expectInt(t *testing.T, expect int, fn func(context.Context) int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	for range time.Tick(pollingInterval) {
		intCh := make(chan int)
		go func() { intCh <- fn(ctx) }()

		select {
		case n := <-intCh:
			if n == expect {
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for value: %d", expect)
			return
		}
	}
}

func expectTrue(t *testing.T, fn func(context.Context) bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	for range time.Tick(pollingInterval) {
		boolCh := make(chan bool)
		go func() { boolCh <- fn(ctx) }()

		select {
		case n := <-boolCh:
			if n {
				return
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for condition")
			return
		}
	}
}
