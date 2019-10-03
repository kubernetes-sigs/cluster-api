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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClusterReconciler_reconcileKubeconfig(t *testing.T) {
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
			c := fake.NewFakeClient(tt.cluster)
			if tt.secret != nil {
				c = fake.NewFakeClient(tt.cluster, tt.secret)
			}
			r := &ClusterReconciler{
				Client: c,
			}
			err := r.reconcileKubeconfig(context.Background(), tt.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileKubeconfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			_, hasRequeErr := errors.Cause(err).(capierrors.HasRequeueAfterError)
			if tt.wantRequeue != hasRequeErr {
				t.Errorf("expected RequeAfterError = %v, got %v", tt.wantRequeue, hasRequeErr)
			}
		})
	}
}
