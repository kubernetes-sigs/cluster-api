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

package test

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func Test_validateMachineHealthCheck(t *testing.T) {
	tests := []struct {
		name               string
		machineHealthCheck *clusterv1.MachineHealthCheck
		wantErr            string
	}{
		{
			name: "Return no error if MachineHealthCheck is valid",
			machineHealthCheck: &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mhc",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: "test-cluster",
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"cluster.x-k8s.io/cluster-name": "test-cluster",
						},
					},
					Checks: clusterv1.MachineHealthCheckChecks{
						UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
							{
								Type:           "InfrastructureReady",
								Status:         metav1.ConditionFalse,
								TimeoutSeconds: ptr.To[int32](300),
							},
						},
					},
				},
			},
		},
		{
			name: "Return error if UnhealthyMachineCondition type is 'Ready'",
			machineHealthCheck: &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mhc",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: "test-cluster",
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"cluster.x-k8s.io/cluster-name": "test-cluster",
						},
					},
					Checks: clusterv1.MachineHealthCheckChecks{
						UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
							{
								Type:           "Ready",
								Status:         metav1.ConditionFalse,
								TimeoutSeconds: ptr.To[int32](300),
							},
						},
					},
				},
			},
			wantErr: "MachineHealthCheck.cluster.x-k8s.io \"mhc\" is invalid: " +
				"spec.checks.unhealthyMachineConditions[0].type: Invalid value: \"string\": type must not be one of: Ready, Available, HealthCheckSucceeded, OwnerRemediated, ExternallyRemediated",
		},
		{
			name: "Return error if UnhealthyMachineCondition type is 'Available'",
			machineHealthCheck: &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mhc",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: "test-cluster",
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"cluster.x-k8s.io/cluster-name": "test-cluster",
						},
					},
					Checks: clusterv1.MachineHealthCheckChecks{
						UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
							{
								Type:           "Available",
								Status:         metav1.ConditionFalse,
								TimeoutSeconds: ptr.To[int32](300),
							},
						},
					},
				},
			},
			wantErr: "MachineHealthCheck.cluster.x-k8s.io \"mhc\" is invalid: " +
				"spec.checks.unhealthyMachineConditions[0].type: Invalid value: \"string\": type must not be one of: Ready, Available, HealthCheckSucceeded, OwnerRemediated, ExternallyRemediated",
		},
		{
			name: "Return error if UnhealthyMachineCondition type is 'HealthCheckSucceeded'",
			machineHealthCheck: &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mhc",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: "test-cluster",
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"cluster.x-k8s.io/cluster-name": "test-cluster",
						},
					},
					Checks: clusterv1.MachineHealthCheckChecks{
						UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
							{
								Type:           "HealthCheckSucceeded",
								Status:         metav1.ConditionFalse,
								TimeoutSeconds: ptr.To[int32](300),
							},
						},
					},
				},
			},
			wantErr: "MachineHealthCheck.cluster.x-k8s.io \"mhc\" is invalid: " +
				"spec.checks.unhealthyMachineConditions[0].type: Invalid value: \"string\": type must not be one of: Ready, Available, HealthCheckSucceeded, OwnerRemediated, ExternallyRemediated",
		},
		{
			name: "Return error if UnhealthyMachineCondition type is 'OwnerRemediated'",
			machineHealthCheck: &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mhc",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: "test-cluster",
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"cluster.x-k8s.io/cluster-name": "test-cluster",
						},
					},
					Checks: clusterv1.MachineHealthCheckChecks{
						UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
							{
								Type:           "OwnerRemediated",
								Status:         metav1.ConditionFalse,
								TimeoutSeconds: ptr.To[int32](300),
							},
						},
					},
				},
			},
			wantErr: "MachineHealthCheck.cluster.x-k8s.io \"mhc\" is invalid: " +
				"spec.checks.unhealthyMachineConditions[0].type: Invalid value: \"string\": type must not be one of: Ready, Available, HealthCheckSucceeded, OwnerRemediated, ExternallyRemediated",
		},
		{
			name: "Return error if UnhealthyMachineCondition type is 'ExternallyRemediated'",
			machineHealthCheck: &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mhc",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: "test-cluster",
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"cluster.x-k8s.io/cluster-name": "test-cluster",
						},
					},
					Checks: clusterv1.MachineHealthCheckChecks{
						UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
							{
								Type:           "ExternallyRemediated",
								Status:         metav1.ConditionFalse,
								TimeoutSeconds: ptr.To[int32](300),
							},
						},
					},
				},
			},
			wantErr: "MachineHealthCheck.cluster.x-k8s.io \"mhc\" is invalid: " +
				"spec.checks.unhealthyMachineConditions[0].type: Invalid value: \"string\": type must not be one of: Ready, Available, HealthCheckSucceeded, OwnerRemediated, ExternallyRemediated",
		},
		{
			name: "Return no error if UnhealthyMachineCondition type is allowed custom type",
			machineHealthCheck: &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mhc",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: "test-cluster",
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"cluster.x-k8s.io/cluster-name": "test-cluster",
						},
					},
					Checks: clusterv1.MachineHealthCheckChecks{
						UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
							{
								Type:           "CustomHealthCheck",
								Status:         metav1.ConditionFalse,
								TimeoutSeconds: ptr.To[int32](300),
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := env.CreateAndWait(ctx, tt.machineHealthCheck)

			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(BeComparableTo(tt.wantErr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(env.CleanupAndWait(ctx, tt.machineHealthCheck)).To(Succeed())
			}
		})
	}
}
