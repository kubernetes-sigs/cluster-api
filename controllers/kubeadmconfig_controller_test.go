/*
Copyright 2019 The Kubernetes Authors.

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
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	cabpkV1alpha2 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
	capiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	capiv1alpha2.AddToScheme(scheme)
	cabpkV1alpha2.AddToScheme(scheme)
	return scheme
}

// Tests for misconfigurations / initial checks

func TestBailIfKubeadmConfigStatusReady(t *testing.T) {
	config := newKubeadmConfig(nil, "cfg") // NB. passing a machine is not relevant for this test
	config.Status.Ready = true

	objects := []runtime.Object{
		config,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expected to requeue after")
	}
}

func TestRequeueIfNoMachineRefIsSet(t *testing.T) {
	config := newKubeadmConfig(nil, "cfg") // intentionally omitting machine
	objects := []runtime.Object{
		config,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter == time.Duration(0) {
		t.Fatal("Expected a requeue but did not get one")
	}
}

func TestFailsIfMachineRefIsNotFound(t *testing.T) {
	machine := newMachine(nil, "machine") // NB. passing a cluster is not relevant for this test
	config := newKubeadmConfig(machine, "cfg")

	objects := []runtime.Object{
		// intentionally omitting machine
		config,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "cfg",
		},
	}
	_, err := k.Reconcile(request)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestBailIfMachineAlreadyHasBootstrapData(t *testing.T) {
	machine := newMachine(nil, "machine") // NB. passing a cluster is not relevant for this test
	machine.Spec.Bootstrap.Data = stringPtr("something")

	config := newKubeadmConfig(machine, "cfg")
	objects := []runtime.Object{
		machine,
		config,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expected to requeue after")
	}
}

func TestFailsNoClusterRefIsSet(t *testing.T) {
	machine := newMachine(nil, "machine") // intentionally omitting cluster
	config := newKubeadmConfig(machine, "cfg")

	objects := []runtime.Object{
		machine,
		config,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "cfg",
		},
	}
	_, err := k.Reconcile(request)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestFailsIfClusterIsNotFound(t *testing.T) {
	cluster := newCluster("cluster")
	machine := newMachine(cluster, "machine")
	config := newKubeadmConfig(machine, "cfg")

	objects := []runtime.Object{
		// intentionally omitting cluster
		machine,
		config,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "cfg",
		},
	}
	_, err := k.Reconcile(request)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

// Tests for cluster with infrastructure not ready yet

func TestRequeueIfInfrastructureIsNotReady(t *testing.T) {
	cluster := newCluster("cluster") // cluster by default has infrastructure not ready
	machine := newMachine(cluster, "machine")
	config := newKubeadmConfig(machine, "cfg")

	objects := []runtime.Object{
		cluster,
		machine,
		config,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(30*time.Second) {
		t.Fatal("expected to requeue after 30s")
	}
}

// Tests for cluster with infrastructure ready, but control pane not ready yet

func TestRequeueKubeadmConfigForJoinNodesIfControlPlaneIsNotReady(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

	workerMachine := newWorkerMachine(cluster, "worker-machine")
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine, "worker-join-cfg")

	controlPaneMachine := newControlPlaneMachine(cluster, "control-plane-machine")
	controlPaneJoinConfig := newControlPlaneJoinKubeadmConfig(controlPaneMachine, "control-plane-join-cfg")

	objects := []runtime.Object{
		cluster,
		workerMachine,
		workerJoinConfig,
		controlPaneMachine,
		controlPaneJoinConfig,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "worker-join-cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(30*time.Second) {
		t.Fatal("expected to requeue after 30s")
	}

	request = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "control-plane-join-cfg",
		},
	}
	result, err = k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(30*time.Second) {
		t.Fatal("expected to requeue after 30s")
	}
}

func TestReconcileKubeadmConfigForInitNodesIfControlPlaneIsNotReady(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

	controlPaneMachine := newControlPlaneMachine(cluster, "control-plane-machine")
	controlPaneInitConfig := newControlPlaneInitKubeadmConfig(controlPaneMachine, "control-plane-init-cfg")

	objects := []runtime.Object{
		cluster,
		controlPaneMachine,
		controlPaneInitConfig,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "control-plane-init-cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expected to requeue after")
	}

	cfg, err := getKubeadmConfig(myclient, "control-plane-init-cfg")
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}

	if cfg.Status.Ready != true {
		t.Fatal("Expected status ready")
	}

	if cfg.Status.BootstrapData == nil {
		t.Fatal("Expected status ready")
	}
}

// Tests for cluster with infrastructure ready, control pane ready

func TestFailIfNotJoinConfigurationAndControlPlaneIsReady(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Annotations = map[string]string{ControlPlaneReadyAnnotationKey: "true"}

	workerMachine := newWorkerMachine(cluster, "worker-machine")
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine, "worker-join-cfg")
	workerJoinConfig.Spec.JoinConfiguration = nil // Makes workerJoinConfig invalid

	objects := []runtime.Object{
		cluster,
		workerMachine,
		workerJoinConfig,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "worker-join-cfg",
		},
	}
	_, err := k.Reconcile(request)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestFailIfJoinConfigurationInconsistentWithMachineRole(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Annotations = map[string]string{ControlPlaneReadyAnnotationKey: "true"}

	controlPaneMachine := newControlPlaneMachine(cluster, "control-plane-machine")
	controlPaneJoinConfig := newControlPlaneJoinKubeadmConfig(controlPaneMachine, "control-plane-join-cfg")
	controlPaneJoinConfig.Spec.JoinConfiguration.ControlPlane = nil // Makes controlPaneJoinConfig invalid for a control plane machine

	objects := []runtime.Object{
		cluster,
		controlPaneMachine,
		controlPaneJoinConfig,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "control-plane-join-cfg",
		},
	}
	_, err := k.Reconcile(request)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestFailIfMissingControlPaneEndpointAndControlPlaneIsReady(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Annotations = map[string]string{ControlPlaneReadyAnnotationKey: "true"}

	workerMachine := newWorkerMachine(cluster, "worker-machine")
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine, "worker-join-cfg")

	objects := []runtime.Object{
		cluster,
		workerMachine,
		workerJoinConfig,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "worker-join-cfg",
		},
	}
	_, err := k.Reconcile(request)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestReconcileIfJoinNodesAndControlPlaneIsReady(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Annotations = map[string]string{ControlPlaneReadyAnnotationKey: "true"}
	cluster.Status.APIEndpoints = []capiv1alpha2.APIEndpoint{{Host: "100.105.150.1", Port: 6443}}

	workerMachine := newWorkerMachine(cluster, "worker-machine")
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine, "worker-join-cfg")

	controlPaneMachine := newControlPlaneMachine(cluster, "control-plane-machine")
	controlPaneJoinConfig := newControlPlaneJoinKubeadmConfig(controlPaneMachine, "control-plane-join-cfg")

	objects := []runtime.Object{
		cluster,
		workerMachine,
		workerJoinConfig,
		controlPaneMachine,
		controlPaneJoinConfig,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "worker-join-cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expected to requeue after")
	}

	cfg, err := getKubeadmConfig(myclient, "worker-join-cfg")
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}

	if cfg.Status.Ready != true {
		t.Fatal("Expected status ready")
	}

	if cfg.Status.BootstrapData == nil {
		t.Fatal("Expected status ready")
	}

	request = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "control-plane-join-cfg",
		},
	}
	result, err = k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expected to requeue after")
	}

	cfg, err = getKubeadmConfig(myclient, "control-plane-join-cfg")
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}

	if cfg.Status.Ready != true {
		t.Fatal("Expected status ready")
	}

	if cfg.Status.BootstrapData == nil {
		t.Fatal("Expected status ready")
	}
}

// test utils

// newCluster return a CAPI cluster object
func newCluster(name string) *capiv1alpha2.Cluster {
	return &capiv1alpha2.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: capiv1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
	}
}

// newMachine return a CAPI machine object; if cluster is not nil, the machine is linked to the cluster as well
func newMachine(cluster *capiv1alpha2.Cluster, name string) *capiv1alpha2.Machine {
	machine := &capiv1alpha2.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: capiv1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: capiv1alpha2.MachineSpec{
			Bootstrap: capiv1alpha2.Bootstrap{
				ConfigRef: &v1.ObjectReference{
					Kind:       "KubeadmConfig",
					APIVersion: "v1alpha2",
				},
			},
		},
	}
	if cluster != nil {
		machine.ObjectMeta.Labels = map[string]string{
			capiv1alpha2.MachineClusterLabelName: cluster.Name,
		}
	}
	return machine
}

func newWorkerMachine(cluster *capiv1alpha2.Cluster, name string) *capiv1alpha2.Machine {
	return newMachine(cluster, name) // machine by default is a worker node (not the boostrapNode)
}

func newControlPlaneMachine(cluster *capiv1alpha2.Cluster, name string) *capiv1alpha2.Machine {
	m := newMachine(cluster, "control-plane-machine")
	m.Labels[capiv1alpha2.MachineControlPlaneLabelName] = "true"
	return m
}

// newKubeadmConfig return a CABPK KubeadmConfig object; if machine is not nil, the KubeadmConfig is linked to the machine as well
func newKubeadmConfig(machine *capiv1alpha2.Machine, name string) *cabpkV1alpha2.KubeadmConfig {
	config := &cabpkV1alpha2.KubeadmConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfig",
			APIVersion: cabpkV1alpha2.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
	}
	if machine != nil {
		config.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				Kind:       "Machine",
				APIVersion: capiv1alpha2.SchemeGroupVersion.String(),
				Name:       machine.Name,
				UID:        types.UID(fmt.Sprintf("%s uid", machine.Name)),
			},
		}
	}
	return config
}

func newWorkerJoinKubeadmConfig(machine *capiv1alpha2.Machine, name string) *cabpkV1alpha2.KubeadmConfig {
	c := newKubeadmConfig(machine, name)
	c.Spec.JoinConfiguration = &kubeadmv1beta1.JoinConfiguration{
		ControlPlane: nil,
	}
	return c
}

func newControlPlaneJoinKubeadmConfig(machine *capiv1alpha2.Machine, name string) *cabpkV1alpha2.KubeadmConfig {
	c := newKubeadmConfig(machine, name)
	c.Spec.JoinConfiguration = &kubeadmv1beta1.JoinConfiguration{
		ControlPlane: &kubeadmv1beta1.JoinControlPlane{},
	}
	return c
}

func newControlPlaneInitKubeadmConfig(machine *capiv1alpha2.Machine, name string) *cabpkV1alpha2.KubeadmConfig {
	c := newKubeadmConfig(machine, name)
	c.Spec.ClusterConfiguration = &kubeadmv1beta1.ClusterConfiguration{}
	c.Spec.InitConfiguration = &kubeadmv1beta1.InitConfiguration{}
	return c
}

func stringPtr(s string) *string {
	return &s
}
