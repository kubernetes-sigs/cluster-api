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
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	//nolint:errcheck
	clusterv1.AddToScheme(scheme)
	//nolint:errcheck
	bootstrapv1.AddToScheme(scheme)
	//nolint:errcheck
	corev1.AddToScheme(scheme)
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

// Tests for cluster with infrastructure ready, but control pane not ready yet
func TestRequeueKubeadmConfigForJoinNodesIfControlPlaneIsNotReady(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

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
	if result.RequeueAfter != 30*time.Second {
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
	if result.RequeueAfter != 30*time.Second {
		t.Fatal("expected to requeue after 30s")
	}
}

func TestReconcileKubeadmConfigForInitNodesIfControlPlaneIsNotReady(t *testing.T) {
	cluster := newCluster("cluster")
	cluster.Status.InfrastructureReady = true

	controlPlaneMachine := newControlPlaneMachine(cluster)
	controlPlaneInitConfig := newControlPlaneInitKubeadmConfig(controlPlaneMachine, "control-plane-init-cfg")

	objects := []runtime.Object{
		cluster,
		controlPlaneMachine,
		controlPlaneInitConfig,
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

	certsSecret, err := getCertsSecret(myclient, cluster.GetName())
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to locate certs secret:\n %+v", err))
	}

	certsMap := certs.NewCertificatesFromMap(certsSecret.Data)
	err = certsMap.Validate()
	if err != nil {
		t.Fatal(fmt.Sprintf("Certificates not valid:\n %+v", err))
	}
}

// Tests for cluster with infrastructure ready, control pane ready

func TestFailIfNotJoinConfigurationAndControlPlaneIsReady(t *testing.T) {
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

func TestFailIfJoinConfigurationInconsistentWithMachineRole(t *testing.T) {
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

func TestRequeueIfMissingControlPaneEndpointAndControlPlaneIsReady(t *testing.T) {
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
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	// stage a secret for certs
	certificates, _ := certs.NewCertificates()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterCertificatesSecretName(cluster.GetName()),
			Namespace: controlPaneJoinConfig.GetNamespace(),
		},
		Data: certificates.ToMap(),
	}
	_ = myclient.Create(context.Background(), secret)

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

	certificates, _ := certs.NewCertificates()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterCertificatesSecretName(cluster.GetName()),
			Namespace: namespace,
		},
		Data: certificates.ToMap(),
	}

	myclient := fake.NewFakeClientWithScheme(setupScheme(), cluster, machine, workerMachine, config, workerConfig, secret)

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
					APIVersion: "v1alpha2",
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

// getCertsSecret returns a Secret object from the cluster containing certificates
func getCertsSecret(c client.Client, clusterName string) (*corev1.Secret, error) {
	ctx := context.Background()
	certSecretKey := client.ObjectKey{
		Namespace: "default",
		Name:      ClusterCertificatesSecretName(clusterName),
	}
	certSecret := &corev1.Secret{}
	err := c.Get(ctx, certSecretKey, certSecret)
	return certSecret, err
}

func stringPtr(s string) *string {
	return &s
}

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
