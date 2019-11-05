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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestClusterReconcilePhases(t *testing.T) {
	t.Run("reconcile infrastructure", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
			Status: clusterv1.ClusterStatus{
				APIEndpoints: []clusterv1.APIEndpoint{{
					Host: "1.2.3.4",
					Port: 0,
				}},
				InfrastructureReady: true,
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
					Kind:       "InfrastructureConfig",
					Name:       "test",
				},
			},
		}

		tests := []struct {
			name      string
			cluster   *clusterv1.Cluster
			infraRef  map[string]interface{}
			expectErr bool
		}{
			{
				name:      "returns no error if infrastructure ref is nil",
				cluster:   &clusterv1.Cluster{ObjectMeta: v1.ObjectMeta{Name: "test-cluster", Namespace: "test-namespace"}},
				expectErr: false,
			},
			{
				name:      "returns error if unable to reconcile infrastructure ref",
				cluster:   cluster,
				expectErr: true,
			},
			{
				name:    "returns no error if infra config is marked for deletion",
				cluster: cluster,
				infraRef: map[string]interface{}{
					"kind":       "InfrastructureConfig",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
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
					"kind":       "InfrastructureConfig",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "test-namespace",
						"deletionTimestamp": "sometime",
					},
				},
				expectErr: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				RegisterTestingT(t)
				err := clusterv1.AddToScheme(scheme.Scheme)
				if err != nil {
					t.Fatal(err)
				}

				var c client.Client
				if tt.infraRef != nil {
					infraConfig := &unstructured.Unstructured{Object: tt.infraRef}
					c = fake.NewFakeClient(tt.cluster, infraConfig)
				} else {
					c = fake.NewFakeClient(tt.cluster)
				}
				r := &ClusterReconciler{
					Client: c,
					Log:    log.Log,
				}

				err = r.reconcileInfrastructure(context.Background(), tt.cluster)
				if tt.expectErr {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
			})
		}

	})

	t.Run("reconcile kubeconfig", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-cluster",
			},
			Status: clusterv1.ClusterStatus{
				APIEndpoints: []clusterv1.APIEndpoint{{
					Host: "1.2.3.4",
					Port: 0,
				}},
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
					ObjectMeta: v1.ObjectMeta{
						Name: "test-cluster-kubeconfig",
					},
				},
				wantErr: false,
			},
			{
				name:        "kubeconfig secret not found, should return RequeueAfterError",
				cluster:     cluster,
				wantErr:     true,
				wantRequeue: true,
			},
			{
				name:    "invalid ca secret, should return error",
				cluster: cluster,
				secret: &corev1.Secret{
					ObjectMeta: v1.ObjectMeta{
						Name: "test-cluster-ca",
					},
				},
				wantErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				RegisterTestingT(t)
				err := clusterv1.AddToScheme(scheme.Scheme)
				if err != nil {
					t.Fatal(err)
				}

				c := fake.NewFakeClient(tt.cluster)
				if tt.secret != nil {
					c = fake.NewFakeClient(tt.cluster, tt.secret)
				}
				r := &ClusterReconciler{
					Client: c,
				}
				err = r.reconcileKubeconfig(context.Background(), tt.cluster)
				if (err != nil) != tt.wantErr {
					t.Errorf("reconcileKubeconfig() error = %v, wantErr %v", err, tt.wantErr)
				}

				_, hasRequeErr := errors.Cause(err).(capierrors.HasRequeueAfterError)
				if tt.wantRequeue != hasRequeErr {
					t.Errorf("expected RequeAfterError = %v, got %v", tt.wantRequeue, hasRequeErr)
				}
			})
		}
	})
}

func TestClusterReconciler_reconcilePhase(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-cluster",
		},
		Status: clusterv1.ClusterStatus{},
		Spec:   clusterv1.ClusterSpec{},
	}
	createClusterError := capierrors.CreateClusterError
	errorMsg := "Create failed"

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
				ObjectMeta: v1.ObjectMeta{
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
				ObjectMeta: v1.ObjectMeta{
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
			name: "cluster infrastructure is ready and APIEndpoints is set",
			cluster: &clusterv1.Cluster{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
					APIEndpoints: []clusterv1.APIEndpoint{{
						Host: "1.2.3.4",
						Port: 0,
					}},
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
				},
			},

			wantPhase: clusterv1.ClusterPhaseProvisioned,
		},
		{
			name: "cluster status has ErrorReason",
			cluster: &clusterv1.Cluster{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
					ErrorReason:         &createClusterError,
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{},
				},
			},

			wantPhase: clusterv1.ClusterPhaseFailed,
		},
		{
			name: "cluster status has ErrorMessage",
			cluster: &clusterv1.Cluster{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
					ErrorMessage:        &errorMsg,
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
				ObjectMeta: v1.ObjectMeta{
					Name:              "test-cluster",
					DeletionTimestamp: &v1.Time{Time: time.Now().UTC()},
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

			c := fake.NewFakeClient(tt.cluster)

			r := &ClusterReconciler{
				Client: c,
			}
			r.reconcilePhase(context.TODO(), tt.cluster)
			if tt.wantPhase != tt.cluster.Status.GetTypedPhase() {
				t.Errorf("expected cluster phase  = %s, got %s", tt.wantPhase, tt.cluster.Status.GetTypedPhase())
			}
		})
	}
}
