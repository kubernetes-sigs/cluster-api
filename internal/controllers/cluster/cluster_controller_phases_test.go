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

package cluster

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func TestClusterReconcilePhases(t *testing.T) {
	t.Run("reconcile infrastructure", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
			Status: clusterv1.ClusterStatus{
				InfrastructureReady: true,
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "1.2.3.4",
					Port: 8443,
				},
				InfrastructureRef: &corev1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericInfrastructureMachine",
					Name:       "test",
				},
			},
		}
		clusterNoEndpoint := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
			Status: clusterv1.ClusterStatus{
				InfrastructureReady: true,
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericInfrastructureMachine",
					Name:       "test",
				},
			},
		}

		tests := []struct {
			name         string
			cluster      *clusterv1.Cluster
			infraRef     map[string]interface{}
			expectErr    bool
			expectResult ctrl.Result
			check        func(g *GomegaWithT, in *clusterv1.Cluster)
		}{
			{
				name:      "returns no error if infrastructure ref is nil",
				cluster:   &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-namespace"}},
				expectErr: false,
			},
			{
				name:         "returns error if unable to reconcile infrastructure ref",
				cluster:      cluster,
				expectErr:    false,
				expectResult: ctrl.Result{RequeueAfter: 30 * time.Second},
			},
			{
				name:    "returns no error if infra config is marked for deletion",
				cluster: cluster,
				infraRef: map[string]interface{}{
					"kind":       "GenericInfrastructureMachine",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "test-namespace",
						"deletionTimestamp": "sometime",
					},
				},
				expectErr: false,
			},
			{
				name:    "returns no error if infrastructure is marked ready on cluster",
				cluster: cluster,
				infraRef: map[string]interface{}{
					"kind":       "GenericInfrastructureMachine",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "test-namespace",
						"deletionTimestamp": "sometime",
					},
				},
				expectErr: false,
			},
			{
				name:    "returns no error if infrastructure has the paused annotation",
				cluster: cluster,
				infraRef: map[string]interface{}{
					"kind":       "GenericInfrastructureMachine",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test-namespace",
						"annotations": map[string]interface{}{
							"cluster.x-k8s.io/paused": "true",
						},
					},
				},
				expectErr: false,
			},
			{
				name:    "returns no error if the control plane endpoint is not yet set",
				cluster: clusterNoEndpoint,
				infraRef: map[string]interface{}{
					"kind":       "GenericInfrastructureMachine",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "test-namespace",
						"deletionTimestamp": "sometime",
					},
					"status": map[string]interface{}{
						"ready": true,
					},
				},
				expectErr: false,
			},
			{
				name:    "should propagate the control plane endpoint once set",
				cluster: clusterNoEndpoint,
				infraRef: map[string]interface{}{
					"kind":       "GenericInfrastructureMachine",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "test-namespace",
						"deletionTimestamp": "sometime",
					},
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "example.com",
							"port": int64(6443),
						},
					},
					"status": map[string]interface{}{
						"ready": true,
					},
				},
				expectErr: false,
				check: func(g *GomegaWithT, in *clusterv1.Cluster) {
					g.Expect(in.Spec.ControlPlaneEndpoint.Host).To(Equal("example.com"))
					g.Expect(in.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(6443))
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				var c client.Client
				if tt.infraRef != nil {
					infraConfig := &unstructured.Unstructured{Object: tt.infraRef}
					c = fake.NewClientBuilder().
						WithObjects(builder.GenericInfrastructureMachineCRD.DeepCopy(), tt.cluster, infraConfig).
						Build()
				} else {
					c = fake.NewClientBuilder().
						WithObjects(builder.GenericInfrastructureMachineCRD.DeepCopy(), tt.cluster).
						Build()
				}
				r := &Reconciler{
					Client:   c,
					recorder: record.NewFakeRecorder(32),
				}

				res, err := r.reconcileInfrastructure(ctx, tt.cluster)
				g.Expect(res).To(BeComparableTo(tt.expectResult))
				if tt.expectErr {
					g.Expect(err).To(HaveOccurred())
				} else {
					g.Expect(err).ToNot(HaveOccurred())
				}

				if tt.check != nil {
					tt.check(g, tt.cluster)
				}
			})
		}
	})

	t.Run("reconcile control plane ref", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
			Status: clusterv1.ClusterStatus{
				InfrastructureReady: true,
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "1.2.3.4",
					Port: 8443,
				},
				ControlPlaneRef: &corev1.ObjectReference{
					APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericControlPlane",
					Name:       "test",
				},
			},
		}
		clusterNoEndpoint := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
			Status: clusterv1.ClusterStatus{
				InfrastructureReady: true,
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: &corev1.ObjectReference{
					APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericControlPlane",
					Name:       "test",
				},
			},
		}

		tests := []struct {
			name         string
			cluster      *clusterv1.Cluster
			cpRef        map[string]interface{}
			expectErr    bool
			expectResult ctrl.Result
			check        func(g *GomegaWithT, in *clusterv1.Cluster)
		}{
			{
				name:      "returns no error if control plane ref is nil",
				cluster:   &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-namespace"}},
				expectErr: false,
			},
			{
				name:         "requeues if unable to reconcile control plane ref",
				cluster:      cluster,
				expectErr:    false,
				expectResult: ctrl.Result{RequeueAfter: 30 * time.Second},
			},
			{
				name:    "returns no error if control plane ref is marked for deletion",
				cluster: cluster,
				cpRef: map[string]interface{}{
					"kind":       "GenericControlPlane",
					"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "test-namespace",
						"deletionTimestamp": "sometime",
					},
				},
				expectErr: false,
			},
			{
				name:    "returns no error if control plane has the paused annotation",
				cluster: cluster,
				cpRef: map[string]interface{}{
					"kind":       "GenericControlPlane",
					"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test-namespace",
						"annotations": map[string]interface{}{
							"cluster.x-k8s.io/paused": "true",
						},
					},
				},
				expectErr: false,
			},
			{
				name:    "returns no error if the control plane endpoint is not yet set",
				cluster: clusterNoEndpoint,
				cpRef: map[string]interface{}{
					"kind":       "GenericControlPlane",
					"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "test-namespace",
						"deletionTimestamp": "sometime",
					},
					"status": map[string]interface{}{
						"ready": true,
					},
				},
				expectErr: false,
			},
			{
				name:    "should propagate the control plane endpoint if set",
				cluster: clusterNoEndpoint,
				cpRef: map[string]interface{}{
					"kind":       "GenericControlPlane",
					"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "test-namespace",
						"deletionTimestamp": "sometime",
					},
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "example.com",
							"port": int64(6443),
						},
					},
					"status": map[string]interface{}{
						"ready": true,
					},
				},
				expectErr: false,
				check: func(g *GomegaWithT, in *clusterv1.Cluster) {
					g.Expect(in.Spec.ControlPlaneEndpoint.Host).To(Equal("example.com"))
					g.Expect(in.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(6443))
				},
			},
			{
				name:    "should propagate the initialized and ready conditions",
				cluster: clusterNoEndpoint,
				cpRef: map[string]interface{}{
					"kind":       "GenericControlPlane",
					"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "test-namespace",
						"deletionTimestamp": "sometime",
					},
					"spec": map[string]interface{}{},
					"status": map[string]interface{}{
						"ready":       true,
						"initialized": true,
					},
				},
				expectErr: false,
				check: func(g *GomegaWithT, in *clusterv1.Cluster) {
					g.Expect(conditions.IsTrue(in, clusterv1.ControlPlaneReadyCondition)).To(BeTrue())
					g.Expect(conditions.IsTrue(in, clusterv1.ControlPlaneInitializedCondition)).To(BeTrue())
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				var c client.Client
				if tt.cpRef != nil {
					cpConfig := &unstructured.Unstructured{Object: tt.cpRef}
					c = fake.NewClientBuilder().
						WithObjects(builder.GenericControlPlaneCRD.DeepCopy(), tt.cluster, cpConfig).
						Build()
				} else {
					c = fake.NewClientBuilder().
						WithObjects(builder.GenericControlPlaneCRD.DeepCopy(), tt.cluster).
						Build()
				}
				r := &Reconciler{
					Client:   c,
					recorder: record.NewFakeRecorder(32),
				}

				res, err := r.reconcileControlPlane(ctx, tt.cluster)
				g.Expect(res).To(BeComparableTo(tt.expectResult))
				if tt.expectErr {
					g.Expect(err).To(HaveOccurred())
				} else {
					g.Expect(err).ToNot(HaveOccurred())
				}

				if tt.check != nil {
					tt.check(g, tt.cluster)
				}
			})
		}
	})

	t.Run("reconcile kubeconfig", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "1.2.3.4",
					Port: 8443,
				},
			},
		}

		tests := []struct {
			name        string
			cluster     *clusterv1.Cluster
			secret      *corev1.Secret
			wantErr     bool
			wantRequeue bool
		}{
			{
				name:    "cluster not provisioned, apiEndpoint is not set",
				cluster: &clusterv1.Cluster{},
				wantErr: false,
			},
			{
				name:    "kubeconfig secret found",
				cluster: cluster,
				secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster-kubeconfig",
					},
				},
				wantErr: false,
			},
			{
				name:        "kubeconfig secret not found, should requeue",
				cluster:     cluster,
				wantErr:     false,
				wantRequeue: true,
			},
			{
				name:    "invalid ca secret, should return error",
				cluster: cluster,
				secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster-ca",
					},
				},
				wantErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				c := fake.NewClientBuilder().
					WithObjects(tt.cluster).
					Build()
				if tt.secret != nil {
					c = fake.NewClientBuilder().
						WithObjects(tt.cluster, tt.secret).
						Build()
				}
				r := &Reconciler{
					Client:   c,
					recorder: record.NewFakeRecorder(32),
				}
				res, err := r.reconcileKubeconfig(ctx, tt.cluster)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
				} else {
					g.Expect(err).ToNot(HaveOccurred())
				}

				if tt.wantRequeue {
					g.Expect(res.RequeueAfter).To(BeNumerically(">=", 0))
				}
			})
		}
	})
}

func TestClusterReconciler_reconcilePhase(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Status: clusterv1.ClusterStatus{},
		Spec:   clusterv1.ClusterSpec{},
	}
	createClusterError := capierrors.CreateClusterError
	failureMsg := "Create failed"

	tests := []struct {
		name      string
		cluster   *clusterv1.Cluster
		wantPhase clusterv1.ClusterPhase
	}{
		{
			name:      "cluster not provisioned",
			cluster:   cluster,
			wantPhase: clusterv1.ClusterPhasePending,
		},
		{
			name: "cluster has infrastructureRef",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: clusterv1.ClusterStatus{},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
				},
			},

			wantPhase: clusterv1.ClusterPhaseProvisioning,
		},
		{
			name: "cluster infrastructure is ready",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
				},
			},

			wantPhase: clusterv1.ClusterPhaseProvisioning,
		},
		{
			name: "cluster infrastructure is ready and ControlPlaneEndpoint is set",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
					ControlPlaneEndpoint: clusterv1.APIEndpoint{
						Host: "1.2.3.4",
						Port: 8443,
					},
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
				},
			},

			wantPhase: clusterv1.ClusterPhaseProvisioned,
		},
		{
			name: "cluster status has FailureReason",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
					FailureReason:       &createClusterError,
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
				},
			},

			wantPhase: clusterv1.ClusterPhaseFailed,
		},
		{
			name: "cluster status has FailureMessage",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
					FailureMessage:      &failureMsg,
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
				},
			},

			wantPhase: clusterv1.ClusterPhaseFailed,
		},
		{
			name: "cluster has deletion timestamp",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster",
					DeletionTimestamp: &metav1.Time{Time: time.Now().UTC()},
					Finalizers:        []string{clusterv1.ClusterFinalizer},
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
				},
			},

			wantPhase: clusterv1.ClusterPhaseDeleting,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().
				WithObjects(tt.cluster).
				Build()

			r := &Reconciler{
				Client:   c,
				recorder: record.NewFakeRecorder(32),
			}
			r.reconcilePhase(ctx, tt.cluster)
			g.Expect(tt.cluster.Status.GetTypedPhase()).To(Equal(tt.wantPhase))
		})
	}
}

func TestClusterReconcilePhases_reconcileFailureDomains(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Status: clusterv1.ClusterStatus{
			InfrastructureReady: true,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: "1.2.3.4",
				Port: 8443,
			},
			InfrastructureRef: &corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureCluster",
				Name:       "test",
			},
		},
	}

	newFailureDomain := clusterv1.FailureDomains{
		"newdomain": clusterv1.FailureDomainSpec{
			ControlPlane: false,
			Attributes: map[string]string{
				"attribute1": "value1",
			},
		},
	}

	newFailureDomainUpdated := clusterv1.FailureDomains{
		"newdomain": clusterv1.FailureDomainSpec{
			ControlPlane: false,
			Attributes: map[string]string{
				"attribute2": "value2",
			},
		},
	}

	clusterWithNewFailureDomainUpdated := cluster.DeepCopy()
	clusterWithNewFailureDomainUpdated.Status.FailureDomains = newFailureDomainUpdated

	oldFailureDomain := clusterv1.FailureDomains{
		"olddomain": clusterv1.FailureDomainSpec{
			ControlPlane: false,
			Attributes: map[string]string{
				"attribute1": "value1",
			},
		},
	}

	clusterWithOldFailureDomain := cluster.DeepCopy()
	clusterWithOldFailureDomain.Status.FailureDomains = oldFailureDomain

	tests := []struct {
		name                 string
		cluster              *clusterv1.Cluster
		infraRef             map[string]interface{}
		expectFailureDomains clusterv1.FailureDomains
	}{
		{
			name:    "expect no failure domain if infrastructure ref is nil",
			cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-namespace"}},
		},
		{
			name:                 "expect no failure domain if infra config does not have failure domain",
			cluster:              cluster.DeepCopy(),
			infraRef:             generateInfraRef(false),
			expectFailureDomains: clusterv1.FailureDomains{},
		},
		{
			name:                 "expect cluster failure domain to be reset to empty if infra config does not have failure domain",
			cluster:              clusterWithOldFailureDomain.DeepCopy(),
			infraRef:             generateInfraRef(false),
			expectFailureDomains: clusterv1.FailureDomains{},
		},
		{
			name:                 "expect failure domain to remain same if infra config have same failure domain",
			cluster:              cluster.DeepCopy(),
			infraRef:             generateInfraRef(true),
			expectFailureDomains: newFailureDomain,
		},
		{
			name:                 "expect failure domain to be updated if infra config has updates to failure domain",
			cluster:              clusterWithNewFailureDomainUpdated.DeepCopy(),
			infraRef:             generateInfraRef(true),
			expectFailureDomains: newFailureDomain,
		},
		{
			name:                 "expect failure domain to be reset if infra config have different failure domain",
			cluster:              clusterWithOldFailureDomain.DeepCopy(),
			infraRef:             generateInfraRef(true),
			expectFailureDomains: newFailureDomain,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{builder.GenericInfrastructureClusterCRD.DeepCopy(), tt.cluster}
			if tt.infraRef != nil {
				objs = append(objs, &unstructured.Unstructured{Object: tt.infraRef})
			}

			c := fake.NewClientBuilder().WithObjects(objs...).Build()
			r := &Reconciler{
				Client:   c,
				recorder: record.NewFakeRecorder(32),
			}

			_, err := r.reconcileInfrastructure(ctx, tt.cluster)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(tt.cluster.Status.FailureDomains).To(BeEquivalentTo(tt.expectFailureDomains))
		})
	}
}

func generateInfraRef(withFailureDomain bool) map[string]interface{} {
	infraRef := map[string]interface{}{
		"kind":       "GenericInfrastructureCluster",
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"metadata": map[string]interface{}{
			"name":              "test",
			"namespace":         "test-namespace",
			"deletionTimestamp": "sometime",
		},
		"status": map[string]interface{}{
			"ready": true,
		},
	}

	if withFailureDomain {
		infraRef["status"] = map[string]interface{}{
			"failureDomains": map[string]interface{}{
				"newdomain": map[string]interface{}{
					"controlPlane": false,
					"attributes": map[string]interface{}{
						"attribute1": "value1",
					},
				},
			},
			"ready": true,
		}
	}

	return infraRef
}
