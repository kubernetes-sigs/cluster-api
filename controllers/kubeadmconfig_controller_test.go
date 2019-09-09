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
	"k8s.io/klog/klogr"
	bootstrapv1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/certs"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
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
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expected to requeue after")
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
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expected to requeue after")
	}
}

// This returns an error since nothing can proceed without an associated cluster.
// TODO: This should probably not return an error
func TestKubeadmConfigReconciler_Reconcile_ReturnErrorIfMachineDoesNotHaveAssociatedCluster(t *testing.T) {
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

// If the associated cluster is not found then there is no way to proceed.
// TODO: This should probably not be an error
func TestKubeadmConfigReconciler_Reconcile_ReturnErrorIfAssociatedClusterIsNotFound(t *testing.T) {
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

// If the control plane isn't initialized then there is no cluster for either a worker or control plane node to join.
func TestKubeadmConfigReconciler_Reconcile_RequeueJoiningNodesIfControlPlaneNotInitialized(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

	workerMachine := newWorkerMachine(cluster)
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine)

	controlPaneMachine := newControlPlaneMachine(cluster)
	controlPaneJoinConfig := newControlPlaneJoinKubeadmConfig(controlPaneMachine, "control-plane-join-cfg")

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
					Namespace: controlPaneJoinConfig.Namespace,
					Name:      controlPaneJoinConfig.Name,
				},
			},
			objects: []runtime.Object{
				cluster,
				controlPaneMachine,
				controlPaneJoinConfig,
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

func TestKubeadmConfigReconciler_Reconcile_GenerateCloudConfigData(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

	controlPlaneMachine := newControlPlaneMachine(cluster)
	controlPlaneInitConfig := newControlPlaneInitKubeadmConfig(controlPlaneMachine, "control-plane-init-cfg")

	objects := []runtime.Object{
		cluster,
		controlPlaneMachine,
		controlPlaneInitConfig,
	}
	objects = append(objects, createSecrets(t, cluster)...)

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
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expected to requeue after")
	}

	cfg, err := getKubeadmConfig(myclient, "control-plane-init-cfg")
	if err != nil {
		t.Fatalf("Failed to reconcile:\n %+v", err)
	}
	if cfg.Status.Ready != true {
		t.Fatal("Expected status ready")
	}
	if cfg.Status.BootstrapData == nil {
		t.Fatal("Expected status ready")
	}

	c, err := k.getClusterCertificates(cluster)
	if err != nil {
		t.Fatalf("Failed to locate certs secret:\n %+v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Failed to validate certs: %+v", err)
	}
}

func TestKubeadmConfigReconciler_Reconcile_ErrorIfAWorkerHasNoJoinConfigurationAndTheControlPlaneIsInitialized(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Status.ControlPlaneInitialized = true

	workerMachine := newWorkerMachine(cluster)
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine)
	workerJoinConfig.Spec.JoinConfiguration = nil // Makes workerJoinConfig invalid

	objects := []runtime.Object{
		cluster,
		workerMachine,
		workerJoinConfig,
	}
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
	_, err := k.Reconcile(request)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

// If a controlplane has an invalid JoinConfiguration then user intervention is required.
// TODO: Could potentially requeue instead of error.
func TestKubeadmConfigReconciler_Reconcile_ErrorIfJoiningControlPlaneHasInvalidConfiguration(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Status.ControlPlaneInitialized = true
	cluster.Status.APIEndpoints = []clusterv1.APIEndpoint{{Host: "100.105.150.1", Port: 6443}}

	controlPaneMachine := newControlPlaneMachine(cluster)
	controlPaneJoinConfig := newControlPlaneJoinKubeadmConfig(controlPaneMachine, "control-plane-join-cfg")
	controlPaneJoinConfig.Spec.JoinConfiguration.ControlPlane = nil // Makes controlPaneJoinConfig invalid for a control plane machine

	objects := []runtime.Object{
		cluster,
		controlPaneMachine,
		controlPaneJoinConfig,
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
			Name:      "control-plane-join-cfg",
		},
	}
	_, err := k.Reconcile(request)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

// If there is no APIEndpoint but everything is ready then requeue in hopes of a new APIEndpoint showing up eventually.
func TestKubeadmConfigReconciler_Reconcile_RequeueIfControlPlaneIsMissingAPIEndpoints(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true
	cluster.Status.ControlPlaneInitialized = true

	workerMachine := newWorkerMachine(cluster)
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine)

	objects := []runtime.Object{
		cluster,
		workerMachine,
		workerJoinConfig,
	}
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
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
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

	workerMachine := newWorkerMachine(cluster)
	workerJoinConfig := newWorkerJoinKubeadmConfig(workerMachine)

	controlPaneMachine := newControlPlaneMachine(cluster)
	controlPaneJoinConfig := newControlPlaneJoinKubeadmConfig(controlPaneMachine, "control-plane-join-cfg")

	objects := []runtime.Object{
		cluster,
		workerMachine,
		workerJoinConfig,
		controlPaneMachine,
		controlPaneJoinConfig,
	}
	objects = append(objects, createSecrets(t, cluster)...)
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

	myremoteclient, _ := k.SecretsClientFactory.NewSecretsClient(nil, nil)
	l, err := myremoteclient.List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}

	if len(l.Items) != 2 {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
}

func TestReconcileDiscoverySuccces(t *testing.T) {
	k := &KubeadmConfigReconciler{
		Log:                  log.Log,
		Client:               nil,
		SecretsClientFactory: newFakeSecretFactory(),
		KubeadmInitLock:      &myInitLocker{},
	}

	dummyCAHash := []string{"...."}
	goodcluster := &clusterv1.Cluster{
		Status: clusterv1.ClusterStatus{
			APIEndpoints: []clusterv1.APIEndpoint{
				{
					Host: "foo.com",
					Port: 6443,
				},
			},
		},
	}
	var useCases = []struct {
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
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
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
				if d.BootstrapToken.APIServerEndpoint != "foo.com:6443" {
					return errors.Errorf("BootstrapToken.APIServerEndpoint=foo.com:6443 expected, got %q", d.BootstrapToken.APIServerEndpoint)
				}
				if d.BootstrapToken.UnsafeSkipCAVerification != true {
					return errors.Errorf("BootstrapToken.UnsafeSkipCAVerification=true expected, got false")
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
								Token: "abcdef.0123456789abcdef",
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

	for _, rt := range useCases {
		rt := rt
		t.Run(rt.name, func(t *testing.T) {
			err := k.reconcileDiscovery(rt.cluster, rt.config)
			if err != nil {
				t.Errorf("expected nil, got error %v", err)
			}

			if err := rt.validateDiscovery(rt.config); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReconcileDiscoveryErrors(t *testing.T) {
	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: nil,
	}

	var useCases = []struct {
		name    string
		cluster *clusterv1.Cluster
		config  *bootstrapv1.KubeadmConfig
	}{
		{
			name:    "Fail if cluster has not APIEndpoints",
			cluster: &clusterv1.Cluster{}, // cluster without endpoints
			config: &bootstrapv1.KubeadmConfig{
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
				},
			},
		},
	}

	for _, rt := range useCases {
		rt := rt
		t.Run(rt.name, func(t *testing.T) {
			err := k.reconcileDiscovery(rt.cluster, rt.config)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestReconcileTopLevelObjectSettings(t *testing.T) {
	k := &KubeadmConfigReconciler{
		Log:    log.Log,
		Client: nil,
	}

	var useCases = []struct {
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

	for _, rt := range useCases {
		rt := rt
		t.Run(rt.name, func(t *testing.T) {
			k.reconcileTopLevelObjectSettings(rt.cluster, rt.machine, rt.config)

			if rt.config.Spec.ClusterConfiguration.ControlPlaneEndpoint != "myControlPlaneEndpoint:6443" {
				t.Fatalf("expected ClusterConfiguration.ControlPlaneEndpoint %q, got %q", "myControlPlaneEndpoint:6443", rt.config.Spec.ClusterConfiguration.ControlPlaneEndpoint)
			}
			if rt.config.Spec.ClusterConfiguration.ClusterName != "mycluster" {
				t.Fatalf("expected ClusterConfiguration.ClusterName %q, got %q", "mycluster", rt.config.Spec.ClusterConfiguration.ClusterName)
			}
			if rt.config.Spec.ClusterConfiguration.Networking.PodSubnet != "myPodSubnet" {
				t.Fatalf("expected ClusterConfiguration.Networking.PodSubnet  %q, got %q", "myPodSubnet", rt.config.Spec.ClusterConfiguration.Networking.PodSubnet)
			}
			if rt.config.Spec.ClusterConfiguration.Networking.ServiceSubnet != "myServiceSubnet" {
				t.Fatalf("expected ClusterConfiguration.Networking.ServiceSubnet  %q, got %q", "myServiceSubnet", rt.config.Spec.ClusterConfiguration.Networking.ServiceSubnet)
			}
			if rt.config.Spec.ClusterConfiguration.Networking.DNSDomain != "myDNSDomain" {
				t.Fatalf("expected ClusterConfiguration.Networking.DNSDomain  %q, got %q", "myDNSDomain", rt.config.Spec.ClusterConfiguration.Networking.DNSDomain)
			}
			if rt.config.Spec.ClusterConfiguration.KubernetesVersion != "myversion" {
				t.Fatalf("expected ClusterConfiguration.KubernetesVersion %q, got %q", "myversion", rt.config.Spec.ClusterConfiguration.KubernetesVersion)
			}
		})
	}
}

func TestCACertHashesAndUnsafeCAVerifySkip(t *testing.T) {
	namespace := "default" // hardcoded in the new* functions
	clusterName := "my-cluster"
	cluster := newCluster(clusterName)
	cluster.Status.ControlPlaneInitialized = true
	cluster.Status.InfrastructureReady = true

	controlPlaneMachineName := "my-machine"
	machine := newMachine(cluster, controlPlaneMachineName)

	workerMachineName := "my-worker"
	workerMachine := newMachine(cluster, workerMachineName)

	controlPlaneConfigName := "my-config"
	config := newKubeadmConfig(machine, controlPlaneConfigName)

	workerConfigName := "worker-join-cfg"
	workerConfig := newWorkerJoinKubeadmConfig(workerMachine)

	objects := []runtime.Object{
		cluster, machine, workerMachine, config, workerConfig,
	}
	objects = append(objects, createSecrets(t, cluster)...)
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	reconciler := KubeadmConfigReconciler{
		Client:               myclient,
		SecretsClientFactory: newFakeSecretFactory(),
		KubeadmInitLock:      &myInitLocker{},
		Log:                  klogr.New(),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Name: workerConfigName, Namespace: namespace},
	}
	if _, err := reconciler.Reconcile(req); err != nil {
		t.Fatalf("reconciled an error: %v", err)
	}
	cfg := &bootstrapv1.KubeadmConfig{}
	if err := myclient.Get(context.Background(), req.NamespacedName, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Spec.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification == true {
		t.Fatal("Should not skip unsafe")
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

func newControlPlaneMachine(cluster *clusterv1.Cluster) *clusterv1.Machine {
	m := newMachine(cluster, "control-plane-machine")
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

func createSecrets(t *testing.T, cluster *clusterv1.Cluster) []runtime.Object {
	certificates, err := certs.NewCertificates()
	if err != nil {
		t.Fatalf("Failed to create new certificates:\n %+v", err)
	}
	out := []runtime.Object{}
	for _, secret := range certs.NewSecretsFromCertificates(cluster, &bootstrapv1.KubeadmConfig{}, certificates) {
		out = append(out, secret)
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

type myInitLocker struct{}

func (m *myInitLocker) Lock(_ context.Context, _ *clusterv1.Cluster, _ *clusterv1.Machine) bool {
	return true
}
func (m *myInitLocker) Unlock(_ context.Context, _ *clusterv1.Cluster) bool { return true }
