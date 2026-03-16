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

package machine

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	externalfake "sigs.k8s.io/cluster-api/controllers/external/fake"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func init() {
	externalReadyWait = 1 * time.Second
}

func TestReconcileBootstrap(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: clusterv1.GroupVersionBootstrap.Group,
					Kind:     "GenericBootstrapConfig",
					Name:     "bootstrap-config1",
				},
			},
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	testCases := []struct {
		name                    string
		contract                string
		machine                 *clusterv1.Machine
		bootstrapConfig         map[string]interface{}
		bootstrapConfigGetError error
		expectResult            ctrl.Result
		expectError             bool
		expected                func(g *WithT, m *clusterv1.Machine)
	}{
		{
			name:                    "no op if bootstrap config ref is not set",
			contract:                "v1beta1",
			machine:                 &clusterv1.Machine{},
			bootstrapConfig:         nil,
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
		},
		{
			name:                    "err reading bootstrap config (something different than not found), it should return error",
			contract:                "v1beta1",
			machine:                 defaultMachine.DeepCopy(),
			bootstrapConfig:         nil,
			bootstrapConfigGetError: errors.New("some error"),
			expectResult:            ctrl.Result{},
			expectError:             true,
		},
		{
			name:                    "bootstrap config is not found, it should requeue",
			contract:                "v1beta1",
			machine:                 defaultMachine.DeepCopy(),
			bootstrapConfig:         nil,
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{RequeueAfter: externalReadyWait},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.BootstrapDataSecretCreated, false)).To(BeFalse())
			},
		},
		{
			name:     "bootstrap config not ready, it should reconcile but no data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          false,
					"dataSecretName": "secret-data",
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.BootstrapDataSecretCreated, false)).To(BeFalse())
				g.Expect(m.Spec.Bootstrap.DataSecretName).To(BeNil())
			},
		},
		{
			name:     "bootstrap config ready with data, it should reconcile and data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.BootstrapDataSecretCreated, false)).To(BeTrue())
				g.Expect(m.Spec.Bootstrap.DataSecretName).NotTo(BeNil())
				g.Expect(*m.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))
			},
		},
		{
			name:     "bootstrap config ready and paused, it should reconcile and data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/paused": "true",
					},
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.BootstrapDataSecretCreated, false)).To(BeTrue())
				g.Expect(m.Spec.Bootstrap.DataSecretName).NotTo(BeNil())
				g.Expect(*m.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))
			},
		},
		{
			name:     "bootstrap config ready and paused, it should reconcile and data should surface on the machine (v1beta2)",
			contract: "v1beta2",
			machine:  defaultMachine.DeepCopy(),
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/paused": "true",
					},
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"initialization": map[string]interface{}{
						"dataSecretCreated": true,
					},
					"dataSecretName": "secret-data",
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.BootstrapDataSecretCreated, false)).To(BeTrue())
				g.Expect(m.Spec.Bootstrap.DataSecretName).NotTo(BeNil())
				g.Expect(*m.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))
			},
		},
		{
			name:     "bootstrap config ready with no bootstrap secret",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.BootstrapDataSecretCreated, false)).To(BeFalse())
				g.Expect(m.Spec.Bootstrap.DataSecretName).To(BeNil())
			},
		},
		{
			name:     "bootstrap data secret and bootstrap ready should not change after bootstrap config is set",
			contract: "v1beta1",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "GenericBootstrapConfig",
							Name:     "bootstrap-config1",
						},
						DataSecretName: ptr.To("secret-data"),
					},
				},
				Status: clusterv1.MachineStatus{
					Initialization: clusterv1.MachineInitializationStatus{
						BootstrapDataSecretCreated: ptr.To(true),
					},
				},
			},
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          false,
					"dataSecretName": "secret-data-changed",
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.BootstrapDataSecretCreated, false)).To(BeTrue())
				g.Expect(*m.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))
			},
		},
		{
			name:     "bootstrap config not found is tolerated when machine is deleting",
			contract: "v1beta1",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "deleting-machine",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: ptr.To(metav1.Now()),
					Finalizers:        []string{"foo"},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "GenericBootstrapConfig",
							Name:     "bootstrap-config1",
						},
						DataSecretName: ptr.To("secret-data"),
					},
				},
				Status: clusterv1.MachineStatus{
					Initialization: clusterv1.MachineInitializationStatus{
						BootstrapDataSecretCreated: ptr.To(true),
					},
				},
			},
			bootstrapConfig:         nil,
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected:                func(_ *WithT, _ *clusterv1.Machine) {},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			if tc.machine == nil {
				tc.machine = defaultMachine.DeepCopy()
			}

			var bootstrapConfig *unstructured.Unstructured
			if tc.bootstrapConfig != nil {
				bootstrapConfig = &unstructured.Unstructured{Object: tc.bootstrapConfig}
			}

			c := fake.NewClientBuilder().
				WithObjects(tc.machine).Build()

			if tc.bootstrapConfigGetError == nil {
				crd := builder.GenericBootstrapConfigCRD.DeepCopy()
				crd.Labels = map[string]string{
					// Set contract label for tc.contract.
					fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, tc.contract): clusterv1.GroupVersionBootstrap.Version,
				}
				g.Expect(c.Create(ctx, crd)).To(Succeed())
			}

			if bootstrapConfig != nil {
				g.Expect(c.Create(ctx, bootstrapConfig)).To(Succeed())
			}

			r := &Reconciler{
				Client: c,
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          runtime.NewScheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}
			s := &scope{cluster: defaultCluster, machine: tc.machine}
			res, err := r.reconcileBootstrap(ctx, s)
			g.Expect(res).To(BeComparableTo(tc.expectResult))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}
		})
	}
}

func TestReconcileInfrastructure(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "GenericInfrastructureMachine",
				Name:     "infra-config1",
			},
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	testCases := []struct {
		name                     string
		contract                 string
		machine                  *clusterv1.Machine
		infraMachine             map[string]interface{}
		infraMachineGetError     error
		expectResult             ctrl.Result
		expectError              bool
		expected                 func(g *WithT, m *clusterv1.Machine)
		expectDeferNextReconcile time.Duration
	}{
		{
			name:                 "err reading infra machine (something different than not found), it should return error",
			contract:             "v1beta1",
			machine:              defaultMachine.DeepCopy(),
			infraMachine:         nil,
			infraMachineGetError: errors.New("some error"),
			expectResult:         ctrl.Result{},
			expectError:          true,
		},
		{
			name:                 "infra machine not found and infrastructure not yet ready, it should requeue",
			contract:             "v1beta1",
			machine:              defaultMachine.DeepCopy(),
			infraMachine:         nil,
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{RequeueAfter: externalReadyWait},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeFalse())
				g.Expect(m.Status.Deprecated).To(BeNil())
			},
		},
		{
			name:     "infra machine not ready, it should reconcile but no data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID": "test://id-1",
				},
				"status": map[string]interface{}{
					"ready": false,
					"addresses": []interface{}{
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.1",
						},
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeFalse())
				g.Expect(m.Spec.ProviderID).To(BeEmpty())
				g.Expect(m.Spec.FailureDomain).To(BeEmpty())
				g.Expect(m.Status.Addresses).To(BeNil())
			},
		},
		{
			name:     "infra machine ready and without optional fields, it should reconcile and data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID": "test://id-1",
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Spec.ProviderID).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(BeEmpty())
				g.Expect(m.Status.Addresses).To(BeNil())
			},
		},
		{
			name:     "infra machine ready and without optional fields, it should reconcile and data should surface on the machine (v1beta2)",
			contract: "v1beta2",
			machine:  defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID": "test://id-1",
				},
				"status": map[string]interface{}{
					"initialization": map[string]interface{}{
						"provisioned": true,
					},
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Spec.ProviderID).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(BeEmpty())
				g.Expect(m.Status.Addresses).To(BeNil())
			},
		},
		{
			name:     "infra machine ready and with optional failure domain (deprecated), it should reconcile and data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Spec.ProviderID).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(BeNil())
			},
		},
		{
			name:     "infra machine ready and with optional failure domain, it should reconcile and data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID": "test://id-1",
				},
				"status": map[string]interface{}{
					"ready":         true,
					"failureDomain": "foo",
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Spec.ProviderID).To(Equal("test://id-1"))
				g.Expect(m.Status.FailureDomain).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(BeNil())
			},
		},
		{
			name:     "infra machine ready and with optional addresses, it should reconcile and data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID": "test://id-1",
				},
				"status": map[string]interface{}{
					"ready": true,
					"addresses": []interface{}{
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.1",
						},
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Spec.ProviderID).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(BeEmpty())
				g.Expect(m.Status.Addresses).To(HaveLen(2))
			},
		},
		{
			name:     "infra machine ready and with all the optional fields, it should reconcile and data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
				},
				"status": map[string]interface{}{
					"ready": true,
					"addresses": []interface{}{
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.1",
						},
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
					"failureDomain": "bar",
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Spec.ProviderID).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(HaveLen(2))
				g.Expect(m.Status.FailureDomain).To(Equal("bar"))
			},
		},
		{
			name:     "infra machine ready and paused, it should reconcile and data should surface on the machine",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/paused": "true",
					},
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
				},
				"status": map[string]interface{}{
					"ready": true,
					"addresses": []interface{}{
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.1",
						},
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Spec.ProviderID).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(HaveLen(2))
			},
		},
		{
			name:     "should do nothing if infra machine ready and no provider ID",
			contract: "v1beta1",
			machine:  defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			infraMachineGetError:     nil,
			expectResult:             ctrl.Result{},
			expectError:              false,
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name:     "should never revert back to infrastructure not ready",
			contract: "v1beta1",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "GenericInfrastructureMachine",
						Name:     "infra-config1",
					},
					ProviderID:    "test://something",
					FailureDomain: "something",
				},
				Status: clusterv1.MachineStatus{
					Initialization: clusterv1.MachineInitializationStatus{
						InfrastructureProvisioned: ptr.To(true),
					},
					Addresses: []clusterv1.MachineAddress{
						{
							Type:    clusterv1.MachineExternalIP,
							Address: "1.2.3.4",
						},
					},
				},
			},
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
				},
				"status": map[string]interface{}{
					"ready": false,
					"addresses": []interface{}{
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.1",
						},
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Spec.ProviderID).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(HaveLen(2))
			},
		},
		{
			name:     "should change data also after infrastructure ready is set",
			contract: "v1beta1",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "GenericInfrastructureMachine",
						Name:     "infra-config1",
					},
					ProviderID:    "test://something",
					FailureDomain: "something",
				},
				Status: clusterv1.MachineStatus{
					Initialization: clusterv1.MachineInitializationStatus{
						InfrastructureProvisioned: ptr.To(true),
					},
					Addresses: []clusterv1.MachineAddress{
						{
							Type:    clusterv1.MachineExternalIP,
							Address: "1.2.3.4",
						},
					},
				},
			},
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
				},
				"status": map[string]interface{}{
					"ready": true,
					"addresses": []interface{}{
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.1",
						},
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Spec.ProviderID).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(HaveLen(2))
			},
		},
		{
			name:     "err reading infra machine when infrastructure have been ready (something different than not found), it should return error",
			contract: "v1beta1",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "GenericInfrastructureMachine",
						Name:     "infra-config1",
					},
				},
				Status: clusterv1.MachineStatus{
					Initialization: clusterv1.MachineInitializationStatus{
						InfrastructureProvisioned: ptr.To(true),
					},
				},
			},
			infraMachine:         nil,
			infraMachineGetError: errors.New("some error"),
			expectResult:         ctrl.Result{},
			expectError:          true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Status.Deprecated).To(BeNil())
			},
		},
		{
			name:     "infra machine not found when infrastructure have been ready, should be treated as terminal error",
			contract: "v1beta1",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "GenericInfrastructureMachine",
						Name:     "infra-config1",
					},
				},
				Status: clusterv1.MachineStatus{
					Initialization: clusterv1.MachineInitializationStatus{
						InfrastructureProvisioned: ptr.To(true),
					},
				},
			},
			infraMachine:         nil,
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(m.Status.Deprecated).ToNot(BeNil())
				g.Expect(m.Status.Deprecated.V1Beta1).ToNot(BeNil())
				g.Expect(m.Status.Deprecated.V1Beta1.FailureMessage).ToNot(BeNil())
				g.Expect(m.Status.Deprecated.V1Beta1.FailureReason).ToNot(BeNil())
			},
		},
		{
			name:     "infra machine is not found is tolerated when infrastructure not yet ready and machine is deleting",
			contract: "v1beta1",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "deleting-machine",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: ptr.To(metav1.Now()),
					Finalizers:        []string{"foo"},
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "GenericInfrastructureMachine",
						Name:     "infra-config1",
					},
				},
				Status: clusterv1.MachineStatus{
					Initialization: clusterv1.MachineInitializationStatus{
						InfrastructureProvisioned: ptr.To(false),
					},
				},
			},
			infraMachine:         nil,
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected:             func(_ *WithT, _ *clusterv1.Machine) {},
		},
		{
			name:     "infra machine is not found is tolerated when infrastructure ready and machine is deleting",
			contract: "v1beta1",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "deleting-machine",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: ptr.To(metav1.Now()),
					Finalizers:        []string{"foo"},
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "GenericInfrastructureMachine",
						Name:     "infra-config1",
					},
				},
				Status: clusterv1.MachineStatus{
					Initialization: clusterv1.MachineInitializationStatus{
						InfrastructureProvisioned: ptr.To(true),
					},
				},
			},
			infraMachine:         nil,
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected:             func(_ *WithT, _ *clusterv1.Machine) {},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			var infraMachine *unstructured.Unstructured
			if tc.infraMachine != nil {
				infraMachine = &unstructured.Unstructured{Object: tc.infraMachine}
			}
			c := fake.NewClientBuilder().
				WithObjects(tc.machine).Build()

			if tc.infraMachineGetError == nil {
				crd := builder.GenericInfrastructureMachineCRD.DeepCopy()
				crd.Labels = map[string]string{
					// Set contract label for tc.contract.
					fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, tc.contract): clusterv1.GroupVersionInfrastructure.Version,
				}
				g.Expect(c.Create(ctx, crd)).To(Succeed())
			}

			if infraMachine != nil {
				g.Expect(c.Create(ctx, infraMachine)).To(Succeed())
			}

			fc := capicontrollerutil.NewFakeController()

			r := &Reconciler{
				Client:     c,
				controller: fc,
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          c.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}
			s := &scope{cluster: defaultCluster, machine: tc.machine}
			result, err := r.reconcileInfrastructure(ctx, s)
			g.Expect(result).To(BeComparableTo(tc.expectResult))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}

			if tc.expectDeferNextReconcile == 0 {
				g.Expect(fc.Deferrals).To(BeEmpty())
			} else {
				g.Expect(fc.Deferrals).To(HaveKeyWithValue(
					reconcile.Request{NamespacedName: client.ObjectKeyFromObject(s.machine)},
					BeTemporally("~", time.Now().Add(tc.expectDeferNextReconcile), 1*time.Second)),
				)
			}
		})
	}
}

func TestReconcileCertificateExpiry(t *testing.T) {
	fakeTimeString := "2020-01-01T00:00:00Z"
	fakeTime, _ := time.Parse(time.RFC3339, fakeTimeString)
	fakeMetaTime := metav1.Time{Time: fakeTime}

	fakeTimeString2 := "2020-02-02T00:00:00Z"
	fakeTime2, _ := time.Parse(time.RFC3339, fakeTimeString2)
	fakeMetaTime2 := metav1.Time{Time: fakeTime2}

	bootstrapConfigWithExpiry := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": clusterv1.GroupVersionBootstrap.String(),
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-with-expiry",
				"namespace": metav1.NamespaceDefault,
				"annotations": map[string]interface{}{
					clusterv1.MachineCertificatesExpiryDateAnnotation: fakeTimeString,
				},
			},
			"spec": map[string]interface{}{},
			"status": map[string]interface{}{
				"ready":          true,
				"dataSecretName": "secret-data",
			},
		},
	}

	bootstrapConfigWithoutExpiry := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": clusterv1.GroupVersionBootstrap.String(),
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-without-expiry",
				"namespace": metav1.NamespaceDefault,
			},
			"spec": map[string]interface{}{},
			"status": map[string]interface{}{
				"ready":          true,
				"dataSecretName": "secret-data",
			},
		},
	}

	tests := []struct {
		name            string
		machine         *clusterv1.Machine
		bootstrapConfig *unstructured.Unstructured
		expected        func(g *WithT, m *clusterv1.Machine)
	}{
		{
			name: "worker machine with certificate expiry annotation should not update expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						clusterv1.MachineCertificatesExpiryDateAnnotation: fakeTimeString,
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{},
				},
			},
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate.IsZero()).To(BeTrue())
			},
		},
		{
			name: "control plane machine with no bootstrap config and no certificate expiry annotation should not set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{},
				},
			},
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate.IsZero()).To(BeTrue())
			},
		},
		{
			name: "control plane machine with bootstrap config and no certificate expiry annotation should not set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "GenericBootstrapConfig",
							Name:     "bootstrap-config-without-expiry",
						},
					},
				},
			},
			bootstrapConfig: bootstrapConfigWithoutExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate.IsZero()).To(BeTrue())
			},
		},
		{
			name: "control plane machine with certificate expiry annotation in bootstrap config should set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "GenericBootstrapConfig",
							Name:     "bootstrap-config-with-expiry",
						},
					},
				},
			},
			bootstrapConfig: bootstrapConfigWithExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(Equal(fakeMetaTime))
			},
		},
		{
			name: "control plane machine with certificate expiry annotation and no certificate expiry annotation on bootstrap config should set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
					Annotations: map[string]string{
						clusterv1.MachineCertificatesExpiryDateAnnotation: fakeTimeString,
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "GenericBootstrapConfig",
							Name:     "bootstrap-config-without-expiry",
						},
					},
				},
			},
			bootstrapConfig: bootstrapConfigWithoutExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(Equal(fakeMetaTime))
			},
		},
		{
			name: "control plane machine with certificate expiry annotation in machine should take precedence over bootstrap config and should set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
					Annotations: map[string]string{
						clusterv1.MachineCertificatesExpiryDateAnnotation: fakeTimeString2,
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "GenericBootstrapConfig",
							Name:     "bootstrap-config-with-expiry",
						},
					},
				},
			},
			bootstrapConfig: bootstrapConfigWithExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(Equal(fakeMetaTime2))
			},
		},
		{
			name: "reset certificates expiry information in machine status if the information is not available on the machine and the bootstrap config",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "GenericBootstrapConfig",
							Name:     "bootstrap-config-without-expiry",
						},
					},
				},
				Status: clusterv1.MachineStatus{
					CertificatesExpiryDate: fakeMetaTime,
				},
			},
			bootstrapConfig: bootstrapConfigWithoutExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate.IsZero()).To(BeTrue())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			r := &Reconciler{}
			s := &scope{machine: tc.machine, bootstrapConfig: tc.bootstrapConfig}
			_, _ = r.reconcileCertificateExpiry(ctx, s)
			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}
		})
	}
}
