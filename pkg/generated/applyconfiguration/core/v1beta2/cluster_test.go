/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import (
	"encoding/json"
	"testing"

	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"
)

func TestClusterApplyConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		builder  func() *ClusterApplyConfiguration
		wantJSON string
	}{
		{
			name: "basic cluster with name and namespace",
			builder: func() *ClusterApplyConfiguration {
				return Cluster("test-cluster", "default")
			},
			wantJSON: `{"kind":"Cluster","apiVersion":"cluster.x-k8s.io/v1beta2","metadata":{"name":"test-cluster","namespace":"default"}}`,
		},
		{
			name: "cluster with control plane endpoint",
			builder: func() *ClusterApplyConfiguration {
				return Cluster("test-cluster", "default").
					WithSpec(ClusterSpec().
						WithControlPlaneEndpoint(APIEndpoint().
							WithHost("10.0.0.1").
							WithPort(6443)))
			},
			wantJSON: `{"kind":"Cluster","apiVersion":"cluster.x-k8s.io/v1beta2","metadata":{"name":"test-cluster","namespace":"default"},"spec":{"controlPlaneEndpoint":{"host":"10.0.0.1","port":6443}}}`,
		},
		{
			name: "cluster with paused field",
			builder: func() *ClusterApplyConfiguration {
				return Cluster("test-cluster", "default").
					WithSpec(ClusterSpec().
						WithPaused(true))
			},
			wantJSON: `{"kind":"Cluster","apiVersion":"cluster.x-k8s.io/v1beta2","metadata":{"name":"test-cluster","namespace":"default"},"spec":{"paused":true}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyConfig := tt.builder()

			// Serialize to JSON
			gotJSON, err := json.Marshal(applyConfig)
			if err != nil {
				t.Fatalf("failed to marshal ApplyConfiguration: %v", err)
			}

			// Compare JSON
			if string(gotJSON) != tt.wantJSON {
				t.Errorf("JSON mismatch:\ngot:  %s\nwant: %s", string(gotJSON), tt.wantJSON)
			}
		})
	}
}

func TestClusterStatusApplyConfiguration(t *testing.T) {
	cluster := Cluster("test-cluster", "default").
		WithStatus(ClusterStatus().
			WithPhase("Provisioned").
			WithConditions(metav1apply.Condition().
				WithType("Ready").
				WithStatus("True").
				WithReason("ClusterReady")))

	// Verify the cluster has status
	if cluster.Status == nil {
		t.Error("expected Status to be set")
	}

	if cluster.Status.Phase == nil || *cluster.Status.Phase != "Provisioned" {
		t.Error("expected Phase to be 'Provisioned'")
	}

	if cluster.Status.Conditions == nil || len(cluster.Status.Conditions) == 0 {
		t.Error("expected Conditions to be set")
	}
}

func TestMachineApplyConfiguration(t *testing.T) {
	machine := Machine("test-machine", "default").
		WithSpec(MachineSpec().
			WithVersion("v1.28.0").
			WithClusterName("test-cluster"))

	// Serialize to JSON
	gotJSON, err := json.Marshal(machine)
	if err != nil {
		t.Fatalf("failed to marshal Machine ApplyConfiguration: %v", err)
	}

	// Verify it contains expected fields
	var result map[string]interface{}
	if err := json.Unmarshal(gotJSON, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["kind"] != "Machine" {
		t.Errorf("expected kind=Machine, got %v", result["kind"])
	}

	if result["apiVersion"] != "cluster.x-k8s.io/v1beta2" {
		t.Errorf("expected apiVersion=cluster.x-k8s.io/v1beta2, got %v", result["apiVersion"])
	}
}
