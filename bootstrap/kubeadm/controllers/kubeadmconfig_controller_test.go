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
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/klog/klogr"
	bootstrapv1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	internalcluster "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/internal/cluster"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := clusterv1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := bootstrapv1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	return scheme
}

// MachineToBootstrapMapFunc return kubeadm bootstrap configref name when configref exists
func TestKubeadmConfigReconciler_MachineToBootstrapMapFuncReturn(t *testing.T) {
	cluster := newCluster("my-cluster")
	objs := []runtime.Object{cluster}
	machineObjs := []runtime.Object{}
	var expectedConfigName string
	for i := 0; i < 3; i++ {
		m := newMachine(cluster, fmt.Sprintf("my-machine-%d", i))
		configName := fmt.Sprintf("my-config-%d", i)
		if i == 1 {
			c := newKubeadmConfig(m, configName)
			objs = append(objs, m, c)
			expectedConfigName = configName
		} else {
			objs = append(objs, m)
		}
		machineObjs = append(machineObjs, m)
	}
	fakeClient := fake.NewFakeClientWithScheme(setupScheme(), objs...)
	reconciler := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: fakeClient,
	}
	for i := 0; i < 3; i++ {
		o := handler.MapObject{
			Object: machineObjs[i],
		}
		configs := reconciler.MachineToBootstrapMapFunc(o)
		if i == 1 {
			if configs[0].Name != expectedConfigName {
				t.Fatalf("unexpected config name: %s", configs[0].Name)
			}
		} else {
			if configs[0].Name != "" {
				t.Fatalf("unexpected config name: %s", configs[0].Name)
			}
		}
	}
}

// Reconcile returns early if the kubeadm config is ready because it should never re-generate bootstrap data.
func TestKubeadmConfigReconciler_Reconcile_ReturnEarlyIfKubeadmConfigIsReady(t *testing.T) {
	config := newKubeadmConfig(nil, "cfg")
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
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if result.Requeue == true {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}
}

// Reconcile returns an error in this case because the owning machine should not go away before the things it owns.
func TestKubeadmConfigReconciler_Reconcile_ReturnErrorIfReferencedMachineIsNotFound(t *testing.T) {
	machine := newMachine(nil, "machine")
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

// If the machine has bootstrap data already then there is no need to generate more bootstrap data. The work is done.
func TestKubeadmConfigReconciler_Reconcile_ReturnEarlyIfMachineHasBootstrapData(t *testing.T) {
	machine := newMachine(nil, "machine")
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
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if result.Requeue == true {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}
}

// Return early If the owning machine does not have an associated cluster
func TestKubeadmConfigReconciler_Reconcile_ReturnEarlyIfMachineHasNoCluster(t *testing.T) {
	machine := newMachine(nil, "machine") // Machine without a cluster
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
	if err != nil {
		t.Fatalf("Not Expecting error, got an error: %+v", err)
	}
}

// This does not expect an error, hoping the machine gets updated with a cluster
func TestKubeadmConfigReconciler_Reconcile_ReturnNilIfMachineDoesNotHaveAssociatedCluster(t *testing.T) {
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
	if err != nil {
		t.Fatal("Not Expecting error, got an error")
	}
}

// This does not expect an error, hoping that the associated cluster will be created
func TestKubeadmConfigReconciler_Reconcile_ReturnNilIfAssociatedClusterIsNotFound(t *testing.T) {
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
	if err != nil {
		t.Fatal("Not Expecting error, got an error")
	}
}

// If the control plane isn't initialized then there is no cluster for either a worker or control plane node to join.
func TestKubeadmConfigReconciler_Reconcile_RequeueJoiningNodesIfControlPlaneNotInitialized(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

	workerMachine := newWorkerMachine(cluster)
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine)

	controlPlaneJoinMachine := newControlPlaneMachine(cluster, "control-plane-join-machine")
	controlPlaneJoinConfig := newControlPlaneJoinKubeadmConfig(controlPlaneJoinMachine, "control-plane-join-cfg")

	testcases := []struct {
		name    string
		request ctrl.Request
		objects []runtime.Object
	}{
		{
			name: "requeue worker when control plane is not yet initialiezd",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: workerJoinConfig.Namespace,
					Name:      workerJoinConfig.Name,
				},
			},
			objects: []runtime.Object{
				cluster,
				workerMachine,
				workerJoinConfig,
			},
		},
		{
			name: "requeue a secondary control plane when the control plane is not yet initialized",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: controlPlaneJoinConfig.Namespace,
					Name:      controlPlaneJoinConfig.Name,
				},
			},
			objects: []runtime.Object{
				cluster,
				controlPlaneJoinMachine,
				controlPlaneJoinConfig,
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			myclient := fake.NewFakeClientWithScheme(setupScheme(), tc.objects...)

			k := &KubeadmConfigReconciler{
				Log:             log.Log,
				Client:          myclient,
				KubeadmInitLock: &myInitLocker{},
			}

			result, err := k.Reconcile(tc.request)
			if err != nil {
				t.Fatalf("Failed to reconcile:\n %+v", err)
			}
			if result.Requeue == true {
				t.Fatal("did not expect to requeue")
			}
			if result.RequeueAfter != 30*time.Second {
				t.Fatal("expected to requeue after 30s")
			}
		})
	}
}

// This generates cloud-config data but does not test the validity of it.
func TestKubeadmConfigReconciler_Reconcile_GenerateCloudConfigData(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

	controlPlaneInitMachine := newControlPlaneMachine(cluster, "control-plane-init-machine")
	controlPlaneInitConfig := newControlPlaneInitKubeadmConfig(controlPlaneInitMachine, "control-plane-init-cfg")

	objects := []runtime.Object{
		cluster,
		controlPlaneInitMachine,
		controlPlaneInitConfig,
	}
	objects = append(objects, createSecrets(t, cluster, controlPlaneInitConfig)...)

	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:             log.Log,
		Client:          myclient,
		KubeadmInitLock: &myInitLocker{},
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "control-plane-init-cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if result.Requeue != false {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}

	cfg, err := getKubeadmConfig(myclient, "control-plane-init-cfg")
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if cfg.Status.Ready != true {
		t.Fatal("Expected status ready")
	}
	if cfg.Status.BootstrapData == nil {
		t.Fatal("Expected generated bootstrap data")
	}

	// Ensure that we don't fail trying to refresh any bootstrap tokens
	_, err = k.Reconcile(request)
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
}

// If a control plane has no JoinConfiguration, then we will create a default and no error will occur
func TestKubeadmConfigReconciler_Reconcile_ErrorIfJoiningControlPlaneHasInvalidConfiguration(t *testing.T) {
	// TODO: extract this kind of code into a setup function that puts the state of objects into an initialized controlplane (implies secrets exist)
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Status.ControlPlaneInitialized = true
	cluster.Status.APIEndpoints = []clusterv1.APIEndpoint{{Host: "100.105.150.1", Port: 6443}}
	controlPlaneInitMachine := newControlPlaneMachine(cluster, "control-plane-init-machine")
	controlPlaneInitConfig := newControlPlaneInitKubeadmConfig(controlPlaneInitMachine, "control-plane-init-cfg")

	controlPlaneJoinMachine := newControlPlaneMachine(cluster, "control-plane-join-machine")
	controlPlaneJoinConfig := newControlPlaneJoinKubeadmConfig(controlPlaneJoinMachine, "control-plane-join-cfg")
	controlPlaneJoinConfig.Spec.JoinConfiguration.ControlPlane = nil // Makes controlPlaneJoinConfig invalid for a control plane machine

	objects := []runtime.Object{
		cluster,
		controlPlaneJoinMachine,
		controlPlaneJoinConfig,
	}
	objects = append(objects, createSecrets(t, cluster, controlPlaneInitConfig)...)
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:                  log.Log,
		Client:               myclient,
		SecretsClientFactory: newFakeSecretFactory(),
		KubeadmInitLock:      &myInitLocker{},
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "control-plane-join-cfg",
		},
	}
	_, err := k.Reconcile(request)
	if err != nil {
		t.Fatalf("Expected no error but got %v", err)
	}
}

// If there is no APIEndpoint but everything is ready then requeue in hopes of a new APIEndpoint showing up eventually.
func TestKubeadmConfigReconciler_Reconcile_RequeueIfControlPlaneIsMissingAPIEndpoints(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Status.ControlPlaneInitialized = true
	controlPlaneInitMachine := newControlPlaneMachine(cluster, "control-plane-init-machine")
	controlPlaneInitConfig := newControlPlaneInitKubeadmConfig(controlPlaneInitMachine, "control-plane-init-cfg")

	workerMachine := newWorkerMachine(cluster)
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine)

	objects := []runtime.Object{
		cluster,
		workerMachine,
		workerJoinConfig,
	}
	objects = append(objects, createSecrets(t, cluster, controlPlaneInitConfig)...)

	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:             log.Log,
		Client:          myclient,
		KubeadmInitLock: &myInitLocker{},
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "worker-join-cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if result.Requeue == true {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != 10*time.Second {
		t.Fatal("expected to requeue after 10s")
	}
}

func TestReconcileIfJoinNodesAndControlPlaneIsReady(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Status.ControlPlaneInitialized = true
	cluster.Status.APIEndpoints = []clusterv1.APIEndpoint{{Host: "100.105.150.1", Port: 6443}}

	var useCases = []struct {
		name          string
		machine       *clusterv1.Machine
		configName    string
		configBuilder func(*clusterv1.Machine, string) *bootstrapv1.KubeadmConfig
	}{
		{
			name:       "Join a worker node with a fully compiled kubeadm config object",
			machine:    newWorkerMachine(cluster),
			configName: "worker-join-cfg",
			configBuilder: func(machine *clusterv1.Machine, name string) *bootstrapv1.KubeadmConfig {
				return newWorkerJoinKubeadmConfig(machine)
			},
		},
		{
			name:          "Join a worker node  with an empty kubeadm config object (defaults apply)",
			machine:       newWorkerMachine(cluster),
			configName:    "worker-join-cfg",
			configBuilder: newKubeadmConfig,
		},
		{
			name:          "Join a control plane node with a fully compiled kubeadm config object",
			machine:       newControlPlaneMachine(cluster, "control-plane-join-machine"),
			configName:    "control-plane-join-cfg",
			configBuilder: newControlPlaneJoinKubeadmConfig,
		},
		{
			name:          "Join a control plane node with an empty kubeadm config object (defaults apply)",
			machine:       newControlPlaneMachine(cluster, "control-plane-join-machine"),
			configName:    "control-plane-join-cfg",
			configBuilder: newKubeadmConfig,
		},
	}

	for _, rt := range useCases {
		rt := rt // pin!
		t.Run(rt.name, func(t *testing.T) {
			config := rt.configBuilder(rt.machine, rt.configName)

			objects := []runtime.Object{
				cluster,
				rt.machine,
				config,
			}
			objects = append(objects, createSecrets(t, cluster, config)...)
			myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)
			k := &KubeadmConfigReconciler{
				Log:                  log.Log,
				Client:               myclient,
				SecretsClientFactory: newFakeSecretFactory(),
				KubeadmInitLock:      &myInitLocker{},
			}

			request := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: config.GetNamespace(),
					Name:      rt.configName,
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

			cfg, err := getKubeadmConfig(myclient, rt.configName)
			if err != nil {
				t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
			}

			if cfg.Status.Ready != true {
				t.Fatal("Expected status ready")
			}

			if cfg.Status.BootstrapData == nil {
				t.Fatal("Expected status ready")
			}

			myremoteclient, _ := k.SecretsClientFactory.NewSecretsClient(nil, nil)
			l, err := myremoteclient.List(metav1.ListOptions{})
			if err != nil {
				t.Fatal(fmt.Sprintf("Failed to get secrets after reconcile:\n %+v", err))
			}

			if len(l.Items) != 1 {
				t.Fatal("Failed to get bootstrap token secret")
			}
		})

	}
}

func TestBootstrapTokenTTLExtension(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Status.ControlPlaneInitialized = true
	cluster.Status.APIEndpoints = []clusterv1.APIEndpoint{{Host: "100.105.150.1", Port: 6443}}

	controlPlaneInitMachine := newControlPlaneMachine(cluster, "control-plane-init-machine")
	initConfig := newControlPlaneInitKubeadmConfig(controlPlaneInitMachine, "control-plane-init-config")
	workerMachine := newWorkerMachine(cluster)
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine)
	controlPlaneJoinMachine := newControlPlaneMachine(cluster, "control-plane-join-machine")
	controlPlaneJoinConfig := newControlPlaneJoinKubeadmConfig(controlPlaneJoinMachine, "control-plane-join-cfg")
	objects := []runtime.Object{
		cluster,
		workerMachine,
		workerJoinConfig,
		controlPlaneJoinMachine,
		controlPlaneJoinConfig,
	}

	objects = append(objects, createSecrets(t, cluster, initConfig)...)
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)
	k := &KubeadmConfigReconciler{
		Log:                  log.Log,
		Client:               myclient,
		SecretsClientFactory: newFakeSecretFactory(),
		KubeadmInitLock:      &myInitLocker{},
	}
	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "worker-join-cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if result.Requeue == true {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}
	cfg, err := getKubeadmConfig(myclient, "worker-join-cfg")
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
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
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if result.Requeue == true {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}
	cfg, err = getKubeadmConfig(myclient, "control-plane-join-cfg")
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if cfg.Status.Ready != true {
		t.Fatal("Expected status ready")
	}
	if cfg.Status.BootstrapData == nil {
		t.Fatal("Expected status ready")
	}

	myremoteclient, _ := k.SecretsClientFactory.NewSecretsClient(nil, nil)
	l, err := myremoteclient.List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to read secrets:\n %+v", err)
	}

	if len(l.Items) != 2 {
		t.Fatalf("Expected two bootstrap tokens, saw:\n %+d", len(l.Items))
	}

	// ensure that the token is refreshed...
	tokenExpires := make([][]byte, len(l.Items))

	for i, item := range l.Items {
		tokenExpires[i] = item.Data[bootstrapapi.BootstrapTokenExpirationKey]
	}

	<-time.After(1 * time.Second)

	for _, req := range []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "worker-join-cfg",
			},
		},
		{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "control-plane-join-cfg",
			},
		},
	} {

		result, err := k.Reconcile(req)
		if err != nil {
			t.Fatalf("Failed to reconcile:\n %+v", err)
		}
		if result.RequeueAfter >= DefaultTokenTTL {
			t.Fatal("expected a requeue duration less than the token TTL")
		}
	}

	l, err = myremoteclient.List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to read secrets:\n %+v", err)
	}

	if len(l.Items) != 2 {
		t.Fatalf("Expected two bootstrap tokens, saw:\n %+d", len(l.Items))
	}

	for i, item := range l.Items {
		if bytes.Equal(tokenExpires[i], item.Data[bootstrapapi.BootstrapTokenExpirationKey]) {
			t.Fatal("Reconcile should have refreshed bootstrap token's expiration until the infrastructure was ready")
		}
		tokenExpires[i] = item.Data[bootstrapapi.BootstrapTokenExpirationKey]
	}

	// ...until the infrastructure is marked "ready"
	workerMachine.Status.InfrastructureReady = true
	err = myclient.Update(context.Background(), workerMachine)
	if err != nil {
		t.Fatalf("unable to set machine infrastructure ready: %v", err)
	}

	controlPlaneJoinMachine.Status.InfrastructureReady = true
	err = myclient.Update(context.Background(), controlPlaneJoinMachine)
	if err != nil {
		t.Fatalf("unable to set machine infrastructure ready: %v", err)
	}

	<-time.After(1 * time.Second)

	for _, req := range []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "worker-join-cfg",
			},
		},
		{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "control-plane-join-cfg",
			},
		},
	} {

		result, err := k.Reconcile(req)
		if err != nil {
			t.Fatalf("Failed to reconcile:\n %+v", err)
		}
		if result.Requeue == true {
			t.Fatal("did not expect to requeue")
		}
		if result.RequeueAfter != time.Duration(0) {
			t.Fatal("did not expect to requeue after")
		}
	}

	l, err = myremoteclient.List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to read secrets:\n %+v", err)
	}

	if len(l.Items) != 2 {
		t.Fatalf("Expected two bootstrap tokens, saw:\n %+d", len(l.Items))
	}

	for i, item := range l.Items {
		if !bytes.Equal(tokenExpires[i], item.Data[bootstrapapi.BootstrapTokenExpirationKey]) {
			t.Fatal("Reconcile should have let the bootstrap token expire after the infrastructure was ready")
		}
	}
}

// Ensure the discovery portion of the JoinConfiguration gets generated correctly.
func TestKubeadmConfigReconciler_Reconcile_DisocveryReconcileBehaviors(t *testing.T) {
	k := &KubeadmConfigReconciler{
		Log:                  log.Log,
		Client:               nil,
		SecretsClientFactory: newFakeSecretFactory(),
		KubeadmInitLock:      &myInitLocker{},
	}

	dummyCAHash := []string{"...."}
	bootstrapToken := kubeadmv1beta1.Discovery{
		BootstrapToken: &kubeadmv1beta1.BootstrapTokenDiscovery{
			CACertHashes: dummyCAHash,
		},
	}
	goodcluster := &clusterv1.Cluster{
		Status: clusterv1.ClusterStatus{
			APIEndpoints: []clusterv1.APIEndpoint{
				{
					Host: "example.com",
					Port: 6443,
				},
			},
		},
	}
	testcases := []struct {
		name              string
		cluster           *clusterv1.Cluster
		config            *bootstrapv1.KubeadmConfig
		validateDiscovery func(*bootstrapv1.KubeadmConfig) error
	}{
		{
			name:    "Automatically generate token if discovery not specified",
			cluster: goodcluster,
			config: &bootstrapv1.KubeadmConfig{
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: bootstrapToken,
					},
				},
			},
			validateDiscovery: func(c *bootstrapv1.KubeadmConfig) error {
				d := c.Spec.JoinConfiguration.Discovery
				if d.BootstrapToken == nil {
					return errors.Errorf("BootstrapToken expected, got nil")
				}
				if d.BootstrapToken.Token == "" {
					return errors.Errorf(("BootstrapToken.Token expected, got empty string"))
				}
				if d.BootstrapToken.APIServerEndpoint != "example.com:6443" {
					return errors.Errorf("BootstrapToken.APIServerEndpoint=example.com:6443 expected, got %q", d.BootstrapToken.APIServerEndpoint)
				}
				if d.BootstrapToken.UnsafeSkipCAVerification == true {
					return errors.Errorf("BootstrapToken.UnsafeSkipCAVerification=false expected, got true")
				}
				return nil
			},
		},
		{
			name:    "Respect discoveryConfiguration.File",
			cluster: goodcluster,
			config: &bootstrapv1.KubeadmConfig{
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: kubeadmv1beta1.Discovery{
							File: &kubeadmv1beta1.FileDiscovery{},
						},
					},
				},
			},
			validateDiscovery: func(c *bootstrapv1.KubeadmConfig) error {
				d := c.Spec.JoinConfiguration.Discovery
				if d.BootstrapToken != nil {
					return errors.Errorf("BootstrapToken should not be created when DiscoveryFile is defined")
				}
				return nil
			},
		},
		{
			name:    "Respect discoveryConfiguration.BootstrapToken.APIServerEndpoint",
			cluster: goodcluster,
			config: &bootstrapv1.KubeadmConfig{
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: kubeadmv1beta1.Discovery{
							BootstrapToken: &kubeadmv1beta1.BootstrapTokenDiscovery{
								CACertHashes:      dummyCAHash,
								APIServerEndpoint: "bar.com:6443",
							},
						},
					},
				},
			},
			validateDiscovery: func(c *bootstrapv1.KubeadmConfig) error {
				d := c.Spec.JoinConfiguration.Discovery
				if d.BootstrapToken.APIServerEndpoint != "bar.com:6443" {
					return errors.Errorf("BootstrapToken.APIServerEndpoint=https://bar.com:6443 expected, got %s", d.BootstrapToken.APIServerEndpoint)
				}
				return nil
			},
		},
		{
			name:    "Respect discoveryConfiguration.BootstrapToken.Token",
			cluster: goodcluster,
			config: &bootstrapv1.KubeadmConfig{
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: kubeadmv1beta1.Discovery{
							BootstrapToken: &kubeadmv1beta1.BootstrapTokenDiscovery{
								CACertHashes: dummyCAHash,
								Token:        "abcdef.0123456789abcdef",
							},
						},
					},
				},
			},
			validateDiscovery: func(c *bootstrapv1.KubeadmConfig) error {
				d := c.Spec.JoinConfiguration.Discovery
				if d.BootstrapToken.Token != "abcdef.0123456789abcdef" {
					return errors.Errorf("BootstrapToken.Token=abcdef.0123456789abcdef expected, got %s", d.BootstrapToken.Token)
				}
				return nil
			},
		},
		{
			name:    "Respect discoveryConfiguration.BootstrapToken.CACertHashes",
			cluster: goodcluster,
			config: &bootstrapv1.KubeadmConfig{
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: kubeadmv1beta1.Discovery{
							BootstrapToken: &kubeadmv1beta1.BootstrapTokenDiscovery{
								CACertHashes: dummyCAHash,
							},
						},
					},
				},
			},
			validateDiscovery: func(c *bootstrapv1.KubeadmConfig) error {
				d := c.Spec.JoinConfiguration.Discovery
				if !reflect.DeepEqual(d.BootstrapToken.CACertHashes, dummyCAHash) {
					return errors.Errorf("BootstrapToken.CACertHashes=%s expected, got %s", dummyCAHash, d.BootstrapToken.Token)
				}
				return nil
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := k.reconcileDiscovery(tc.cluster, tc.config, internalcluster.Certificates{})
			if err != nil {
				t.Errorf("expected nil, got error %v", err)
			}

			if err := tc.validateDiscovery(tc.config); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Test failure cases for the discovery reconcile function.
func TestKubeadmConfigReconciler_Reconcile_DisocveryReconcileFailureBehaviors(t *testing.T) {
	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: nil,
	}

	testcases := []struct {
		name    string
		cluster *clusterv1.Cluster
		config  *bootstrapv1.KubeadmConfig
	}{
		{
			name:    "Fail if cluster has not APIEndpoints",
			cluster: &clusterv1.Cluster{}, // cluster without endpoints
			config: &bootstrapv1.KubeadmConfig{
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: kubeadmv1beta1.Discovery{
							BootstrapToken: &kubeadmv1beta1.BootstrapTokenDiscovery{
								CACertHashes: []string{"item"},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := k.reconcileDiscovery(tc.cluster, tc.config, internalcluster.Certificates{})
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// Set cluster configuration defaults based on dynamic values from the cluster object.
func TestKubeadmConfigReconciler_Reconcile_DynamicDefaultsForClusterConfiguration(t *testing.T) {
	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: nil,
	}

	testcases := []struct {
		name    string
		cluster *clusterv1.Cluster
		machine *clusterv1.Machine
		config  *bootstrapv1.KubeadmConfig
	}{
		{
			name: "Config settings have precedence",
			config: &bootstrapv1.KubeadmConfig{
				Spec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
						ClusterName:       "mycluster",
						KubernetesVersion: "myversion",
						Networking: kubeadmv1beta1.Networking{
							PodSubnet:     "myPodSubnet",
							ServiceSubnet: "myServiceSubnet",
							DNSDomain:     "myDNSDomain",
						},
						ControlPlaneEndpoint: "myControlPlaneEndpoint:6443",
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "OtherName",
				},
				Spec: clusterv1.ClusterSpec{
					ClusterNetwork: &clusterv1.ClusterNetwork{
						Services:      &clusterv1.NetworkRanges{CIDRBlocks: []string{"otherServicesCidr"}},
						Pods:          &clusterv1.NetworkRanges{CIDRBlocks: []string{"otherPodsCidr"}},
						ServiceDomain: "otherServiceDomain",
					},
				},
				Status: clusterv1.ClusterStatus{
					APIEndpoints: []clusterv1.APIEndpoint{{Host: "otherVersion", Port: 0}},
				},
			},
			machine: &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					Version: stringPtr("otherVersion"),
				},
			},
		},
		{
			name: "Top level object settings are used in case config settings are missing",
			config: &bootstrapv1.KubeadmConfig{
				Spec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mycluster",
				},
				Spec: clusterv1.ClusterSpec{
					ClusterNetwork: &clusterv1.ClusterNetwork{
						Services:      &clusterv1.NetworkRanges{CIDRBlocks: []string{"myServiceSubnet"}},
						Pods:          &clusterv1.NetworkRanges{CIDRBlocks: []string{"myPodSubnet"}},
						ServiceDomain: "myDNSDomain",
					},
				},
				Status: clusterv1.ClusterStatus{
					APIEndpoints: []clusterv1.APIEndpoint{{Host: "myControlPlaneEndpoint", Port: 6443}},
				},
			},
			machine: &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					Version: stringPtr("myversion"),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			k.reconcileTopLevelObjectSettings(tc.cluster, tc.machine, tc.config)

			if tc.config.Spec.ClusterConfiguration.ControlPlaneEndpoint != "myControlPlaneEndpoint:6443" {
				t.Errorf("expected ClusterConfiguration.ControlPlaneEndpoint %q, got %q", "myControlPlaneEndpoint:6443", tc.config.Spec.ClusterConfiguration.ControlPlaneEndpoint)
			}
			if tc.config.Spec.ClusterConfiguration.ClusterName != "mycluster" {
				t.Errorf("expected ClusterConfiguration.ClusterName %q, got %q", "mycluster", tc.config.Spec.ClusterConfiguration.ClusterName)
			}
			if tc.config.Spec.ClusterConfiguration.Networking.PodSubnet != "myPodSubnet" {
				t.Errorf("expected ClusterConfiguration.Networking.PodSubnet  %q, got %q", "myPodSubnet", tc.config.Spec.ClusterConfiguration.Networking.PodSubnet)
			}
			if tc.config.Spec.ClusterConfiguration.Networking.ServiceSubnet != "myServiceSubnet" {
				t.Errorf("expected ClusterConfiguration.Networking.ServiceSubnet  %q, got %q", "myServiceSubnet", tc.config.Spec.ClusterConfiguration.Networking.ServiceSubnet)
			}
			if tc.config.Spec.ClusterConfiguration.Networking.DNSDomain != "myDNSDomain" {
				t.Errorf("expected ClusterConfiguration.Networking.DNSDomain  %q, got %q", "myDNSDomain", tc.config.Spec.ClusterConfiguration.Networking.DNSDomain)
			}
			if tc.config.Spec.ClusterConfiguration.KubernetesVersion != "myversion" {
				t.Errorf("expected ClusterConfiguration.KubernetesVersion %q, got %q", "myversion", tc.config.Spec.ClusterConfiguration.KubernetesVersion)
			}
		})
	}
}

// Allow users to skip CA Verification if they *really* want to.
func TestKubeadmConfigReconciler_Reconcile_AlwaysCheckCAVerificationUnlessRequestedToSkip(t *testing.T) {
	// Setup work for an initialized cluster
	clusterName := "my-cluster"
	cluster := newCluster(clusterName)
	cluster.Status.ControlPlaneInitialized = true
	cluster.Status.InfrastructureReady = true
	cluster.Status.APIEndpoints = []clusterv1.APIEndpoint{
		{
			Host: "example.com",
			Port: 6443,
		},
	}
	controlPlaneInitMachine := newControlPlaneMachine(cluster, "my-control-plane-init-machine")
	initConfig := newControlPlaneInitKubeadmConfig(controlPlaneInitMachine, "my-control-plane-init-config")

	controlPlaneMachineName := "my-machine"
	machine := newMachine(cluster, controlPlaneMachineName)

	workerMachineName := "my-worker"
	workerMachine := newMachine(cluster, workerMachineName)

	controlPlaneConfigName := "my-config"
	config := newKubeadmConfig(machine, controlPlaneConfigName)

	objects := []runtime.Object{
		cluster, machine, workerMachine, config,
	}
	objects = append(objects, createSecrets(t, cluster, initConfig)...)

	testcases := []struct {
		name               string
		discovery          *kubeadmv1beta1.BootstrapTokenDiscovery
		skipCAVerification bool
	}{
		{
			name:               "Do not skip CA verification by default",
			discovery:          &kubeadmv1beta1.BootstrapTokenDiscovery{},
			skipCAVerification: false,
		},
		{
			name: "Skip CA verification if requested by the user",
			discovery: &kubeadmv1beta1.BootstrapTokenDiscovery{
				UnsafeSkipCAVerification: true,
			},
			skipCAVerification: true,
		},
		{
			// skipCAVerification should be true since no Cert Hashes are provided, but reconcile will *always* get or create certs.
			// TODO: Certificate get/create behavior needs to be mocked to enable this test.
			name: "cannot test for defaulting behavior through the reconcile function",
			discovery: &kubeadmv1beta1.BootstrapTokenDiscovery{
				CACertHashes: []string{""},
			},
			skipCAVerification: false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)
			reconciler := KubeadmConfigReconciler{
				Client:               myclient,
				SecretsClientFactory: newFakeSecretFactory(),
				KubeadmInitLock:      &myInitLocker{},
				Log:                  klogr.New(),
			}

			wc := newWorkerJoinKubeadmConfig(workerMachine)
			wc.Spec.JoinConfiguration.Discovery.BootstrapToken = tc.discovery
			key := types.NamespacedName{Namespace: wc.Namespace, Name: wc.Name}
			if err := myclient.Create(context.Background(), wc); err != nil {
				t.Fatal(err)
			}
			req := ctrl.Request{NamespacedName: key}
			if _, err := reconciler.Reconcile(req); err != nil {
				t.Fatalf("reconciled an error: %v", err)
			}
			cfg := &bootstrapv1.KubeadmConfig{}
			if err := myclient.Get(context.Background(), key, cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Spec.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification != tc.skipCAVerification {
				t.Fatalf("Expected skip CA verification: %v but was %v", tc.skipCAVerification, !tc.skipCAVerification)
			}
		})
	}
}

// If a cluster object changes then all associated KubeadmConfigs should be re-reconciled.
// This allows us to not requeue a kubeadm config while we wait for InfrastructureReady.
func TestKubeadmConfigReconciler_ClusterToKubeadmConfigs(t *testing.T) {
	cluster := newCluster("my-cluster")
	objs := []runtime.Object{cluster}
	expectedNames := []string{}
	for i := 0; i < 3; i++ {
		m := newMachine(cluster, fmt.Sprintf("my-machine-%d", i))
		configName := fmt.Sprintf("my-config-%d", i)
		c := newKubeadmConfig(m, configName)
		expectedNames = append(expectedNames, configName)
		objs = append(objs, m, c)
	}
	fakeClient := fake.NewFakeClientWithScheme(setupScheme(), objs...)
	reconciler := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: fakeClient,
	}
	o := handler.MapObject{
		Object: cluster,
	}
	configs := reconciler.ClusterToKubeadmConfigs(o)
	names := make([]string, 3)
	for i := range configs {
		names[i] = configs[i].Name
	}
	for _, name := range expectedNames {
		found := false
		for _, foundName := range names {
			if foundName == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("did not find %s in %v", name, names)
		}
	}
}

// Reconcile should not fail if the Etcd CA Secret already exists
func TestKubeadmConfigReconciler_Reconcile_DoesNotFailIfCASecretsAlreadyExist(t *testing.T) {
	cluster := newCluster("my-cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Status.ControlPlaneInitialized = false
	m := newControlPlaneMachine(cluster, "control-plane-machine")
	configName := "my-config"
	c := newControlPlaneInitKubeadmConfig(m, configName)
	scrt := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", cluster.Name, internalcluster.EtcdCA),
			Namespace: "default",
		},
		Data: map[string][]byte{
			"tls.crt": []byte("hello world"),
			"tls.key": []byte("hello world"),
		},
	}
	fakec := fake.NewFakeClientWithScheme(setupScheme(), []runtime.Object{cluster, m, c, scrt}...)
	reconciler := &KubeadmConfigReconciler{
		Log:             log.Log,
		Client:          fakec,
		KubeadmInitLock: &myInitLocker{},
	}
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: configName},
	}
	if _, err := reconciler.Reconcile(req); err != nil {
		t.Fatal(err)
	}
}

// Exactly one control plane machine initializes if there are multiple control plane machines defined
func TestKubeadmConfigReconciler_Reconcile_ExactlyOneControlPlaneMachineInitializes(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

	controlPlaneInitMachineFirst := newControlPlaneMachine(cluster, "control-plane-init-machine-first")
	controlPlaneInitConfigFirst := newControlPlaneInitKubeadmConfig(controlPlaneInitMachineFirst, "control-plane-init-cfg-first")

	controlPlaneInitMachineSecond := newControlPlaneMachine(cluster, "control-plane-init-machine-second")
	controlPlaneInitConfigSecond := newControlPlaneInitKubeadmConfig(controlPlaneInitMachineSecond, "control-plane-init-cfg-second")

	objects := []runtime.Object{
		cluster,
		controlPlaneInitMachineFirst,
		controlPlaneInitConfigFirst,
		controlPlaneInitMachineSecond,
		controlPlaneInitConfigSecond,
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)
	k := &KubeadmConfigReconciler{
		Log:                  log.Log,
		Client:               myclient,
		SecretsClientFactory: newFakeSecretFactory(),
		KubeadmInitLock:      &myInitLocker{},
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "control-plane-init-cfg-first",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if result.Requeue == true {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}

	request = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "control-plane-init-cfg-second",
		},
	}
	result, err = k.Reconcile(request)
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if result.Requeue == true {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != 30*time.Second {
		t.Fatal("expected to requeue after 30s")
	}
}

// No patch should be applied if there is an error in reconcile
func TestKubeadmConfigReconciler_Reconcile_DoNotPatchWhenErrorOccurred(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

	controlPlaneInitMachine := newControlPlaneMachine(cluster, "control-plane-init-machine")
	controlPlaneInitConfig := newControlPlaneInitKubeadmConfig(controlPlaneInitMachine, "control-plane-init-cfg")

	// set InitConfiguration as nil, we will check this to determine if the kubeadm config has been patched
	controlPlaneInitConfig.Spec.InitConfiguration = nil

	objects := []runtime.Object{
		cluster,
		controlPlaneInitMachine,
		controlPlaneInitConfig,
	}

	secrets := createSecrets(t, cluster, controlPlaneInitConfig)
	for _, obj := range secrets {
		s := obj.(*corev1.Secret)
		delete(s.Data, secret.TLSCrtDataName) // destroy the secrets, which will cause Reconcile to fail
		objects = append(objects, s)
	}

	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)
	k := &KubeadmConfigReconciler{
		Log:                  log.Log,
		Client:               myclient,
		SecretsClientFactory: newFakeSecretFactory(),
		KubeadmInitLock:      &myInitLocker{},
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "control-plane-init-cfg",
		},
	}

	result, err := k.Reconcile(request)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if result.Requeue != false {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}

	cfg, err := getKubeadmConfig(myclient, "control-plane-init-cfg")
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}

	// check if the kubeadm config has been patched
	if cfg.Spec.InitConfiguration != nil {
		t.Fatal("did not expect to patch the kubeadm config if there was an error in Reconcile")
	}
}

// test utils

// newCluster return a CAPI cluster object
func newCluster(name string) *clusterv1.Cluster {
	return &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
	}
}

// newMachine return a CAPI machine object; if cluster is not nil, the machine is linked to the cluster as well
func newMachine(cluster *clusterv1.Cluster, name string) *clusterv1.Machine {
	machine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
			},
		},
	}
	if cluster != nil {
		machine.ObjectMeta.Labels = map[string]string{
			clusterv1.MachineClusterLabelName: cluster.Name,
		}
	}
	return machine
}

func newWorkerMachine(cluster *clusterv1.Cluster) *clusterv1.Machine {
	return newMachine(cluster, "worker-machine") // machine by default is a worker node (not the bootstrapNode)
}

func newControlPlaneMachine(cluster *clusterv1.Cluster, name string) *clusterv1.Machine {
	m := newMachine(cluster, name)
	m.Labels[clusterv1.MachineControlPlaneLabelName] = "true"
	return m
}

// newKubeadmConfig return a CABPK KubeadmConfig object; if machine is not nil, the KubeadmConfig is linked to the machine as well
func newKubeadmConfig(machine *clusterv1.Machine, name string) *bootstrapv1.KubeadmConfig {
	config := &bootstrapv1.KubeadmConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfig",
			APIVersion: bootstrapv1.GroupVersion.String(),
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
				APIVersion: clusterv1.GroupVersion.String(),
				Name:       machine.Name,
				UID:        types.UID(fmt.Sprintf("%s uid", machine.Name)),
			},
		}
		machine.Spec.Bootstrap.ConfigRef.Name = config.Name
		machine.Spec.Bootstrap.ConfigRef.Namespace = config.Namespace
	}
	return config
}

func newWorkerJoinKubeadmConfig(machine *clusterv1.Machine) *bootstrapv1.KubeadmConfig {
	c := newKubeadmConfig(machine, "worker-join-cfg")
	c.Spec.JoinConfiguration = &kubeadmv1beta1.JoinConfiguration{
		ControlPlane: nil,
	}
	return c
}

func newControlPlaneJoinKubeadmConfig(machine *clusterv1.Machine, name string) *bootstrapv1.KubeadmConfig {
	c := newKubeadmConfig(machine, name)
	c.Spec.JoinConfiguration = &kubeadmv1beta1.JoinConfiguration{
		ControlPlane: &kubeadmv1beta1.JoinControlPlane{},
	}
	return c
}

func newControlPlaneInitKubeadmConfig(machine *clusterv1.Machine, name string) *bootstrapv1.KubeadmConfig {
	c := newKubeadmConfig(machine, name)
	c.Spec.ClusterConfiguration = &kubeadmv1beta1.ClusterConfiguration{}
	c.Spec.InitConfiguration = &kubeadmv1beta1.InitConfiguration{}
	return c
}

func createSecrets(t *testing.T, cluster *clusterv1.Cluster, owner *bootstrapv1.KubeadmConfig) []runtime.Object {
	out := []runtime.Object{}
	if owner.Spec.ClusterConfiguration == nil {
		owner.Spec.ClusterConfiguration = &kubeadmv1beta1.ClusterConfiguration{}
	}
	certificates := internalcluster.NewCertificatesForInitialControlPlane(owner.Spec.ClusterConfiguration)
	if err := certificates.Generate(); err != nil {
		t.Fatal(err)
	}
	for _, certificate := range certificates {
		out = append(out, certificate.AsSecret(cluster, owner))
	}
	return out
}

func stringPtr(s string) *string {
	return &s
}

// TODO this is not a fake but an actual client whose behavior we cannot control.
// TODO remove this, probably when https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm/issues/127 is closed.
func newFakeSecretFactory() FakeSecretFactory {
	return FakeSecretFactory{
		client: fakeclient.NewSimpleClientset().CoreV1().Secrets(metav1.NamespaceSystem),
	}
}

type FakeSecretFactory struct {
	client typedcorev1.SecretInterface
}

func (f FakeSecretFactory) NewSecretsClient(client client.Client, cluster *clusterv1.Cluster) (typedcorev1.SecretInterface, error) {
	return f.client, nil
}

type myInitLocker struct {
	locked bool
}

func (m *myInitLocker) Lock(_ context.Context, _ *clusterv1.Cluster, _ *clusterv1.Machine) bool {
	if !m.locked {
		m.locked = true
		return true
	}
	return false
}

func (m *myInitLocker) Unlock(_ context.Context, _ *clusterv1.Cluster) bool {
	if m.locked {
		m.locked = false
	}
	return true
}
