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
	cabpkV1alpha2 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/certs"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
	capiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	capiv1alpha2.AddToScheme(scheme)
	cabpkV1alpha2.AddToScheme(scheme)
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

	controlPlaneMachine := newControlPlaneMachine(cluster, "control-plane-machine")
	controlPlaneInitConfig := newControlPlaneInitKubeadmConfig(controlPlaneMachine, "control-plane-init-cfg")

	objects := []runtime.Object{
		cluster,
		controlPlaneMachine,
		controlPlaneInitConfig,
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
	cluster.Status.APIEndpoints = []capiv1alpha2.APIEndpoint{{Host: "100.105.150.1", Port: 6443}}

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
		Log:                  log.Log,
		Client:               myclient,
		SecretsClientFactory: newFakeSecretFactory(),
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
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(10*time.Second) {
		t.Fatal("expected to requeue after 10s")
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
	}

	dummyCAHash := []string{"...."}
	goodcluster := &capiv1alpha2.Cluster{
		Status: capiv1alpha2.ClusterStatus{
			APIEndpoints: []capiv1alpha2.APIEndpoint{
				{
					Host: "foo.com",
					Port: 6443,
				},
			},
		},
	}
	var useCases = []struct {
		name              string
		cluster           *capiv1alpha2.Cluster
		config            *cabpkV1alpha2.KubeadmConfig
		validateDiscovery func(*cabpkV1alpha2.KubeadmConfig) error
	}{
		{
			name:    "Automatically generate token if discovery not specified",
			cluster: goodcluster,
			config: &cabpkV1alpha2.KubeadmConfig{
				Spec: cabpkV1alpha2.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
				},
			},
			validateDiscovery: func(c *cabpkV1alpha2.KubeadmConfig) error {
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
			config: &cabpkV1alpha2.KubeadmConfig{
				Spec: cabpkV1alpha2.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: kubeadmv1beta1.Discovery{
							File: &kubeadmv1beta1.FileDiscovery{},
						},
					},
				},
			},
			validateDiscovery: func(c *cabpkV1alpha2.KubeadmConfig) error {
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
			config: &cabpkV1alpha2.KubeadmConfig{
				Spec: cabpkV1alpha2.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: kubeadmv1beta1.Discovery{
							BootstrapToken: &kubeadmv1beta1.BootstrapTokenDiscovery{
								APIServerEndpoint: "bar.com:6443",
							},
						},
					},
				},
			},
			validateDiscovery: func(c *cabpkV1alpha2.KubeadmConfig) error {
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
			config: &cabpkV1alpha2.KubeadmConfig{
				Spec: cabpkV1alpha2.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: kubeadmv1beta1.Discovery{
							BootstrapToken: &kubeadmv1beta1.BootstrapTokenDiscovery{
								Token: "abcdef.0123456789abcdef",
							},
						},
					},
				},
			},
			validateDiscovery: func(c *cabpkV1alpha2.KubeadmConfig) error {
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
			config: &cabpkV1alpha2.KubeadmConfig{
				Spec: cabpkV1alpha2.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						Discovery: kubeadmv1beta1.Discovery{
							BootstrapToken: &kubeadmv1beta1.BootstrapTokenDiscovery{
								CACertHashes: dummyCAHash,
							},
						},
					},
				},
			},
			validateDiscovery: func(c *cabpkV1alpha2.KubeadmConfig) error {
				d := c.Spec.JoinConfiguration.Discovery
				if !reflect.DeepEqual(d.BootstrapToken.CACertHashes, dummyCAHash) {
					return errors.Errorf("BootstrapToken.CACertHashes=%s expected, got %s", dummyCAHash, d.BootstrapToken.Token)
				}
				return nil
			},
		},
	}

	for _, rt := range useCases {
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
		cluster *capiv1alpha2.Cluster
		config  *cabpkV1alpha2.KubeadmConfig
	}{
		{
			name:    "Fail if cluster has not APIEndpoints",
			cluster: &capiv1alpha2.Cluster{}, // cluster without endpoints
			config: &cabpkV1alpha2.KubeadmConfig{
				Spec: cabpkV1alpha2.KubeadmConfigSpec{
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
				},
			},
		},
	}

	for _, rt := range useCases {
		t.Run(rt.name, func(t *testing.T) {
			err := k.reconcileDiscovery(rt.cluster, rt.config)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
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
				ConfigRef: &corev1.ObjectReference{
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
	m := newMachine(cluster, name)
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

func (f FakeSecretFactory) NewSecretsClient(client client.Client, cluster *capiv1alpha2.Cluster) (typedcorev1.SecretInterface, error) {
	return f.client, nil
}
