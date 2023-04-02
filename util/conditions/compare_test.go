/*
Copyright 2020 The Kubernetes Authors.

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

package conditions

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unstructv1 "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestCompareConditions(t *testing.T) {
	testCases := []struct {
		name        string
		actual      interface{}
		expected    interface{}
		expectMatch bool
	}{
		{
			name:        "with similar strings",
			actual:      "testing compare function",
			expected:    "testing compare function",
			expectMatch: true,
		},
		{
			name:        "with non similar strings",
			actual:      "testing compare function",
			expected:    "testing compare functions",
			expectMatch: false,
		},
		{
			name: "with similar clusterv1 cluster object",
			actual: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test",
					Namespace:    "test-namespace",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: false,
					Conditions: clusterv1.Conditions{
						*TrueCondition(clusterv1.ControlPlaneInitializedCondition),
					},
				},
			},
			expected: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test",
					Namespace:    "test-namespace",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: false,
					Conditions: clusterv1.Conditions{
						*TrueCondition(clusterv1.ControlPlaneInitializedCondition),
					},
				},
			},
			expectMatch: true,
		},
		{
			name: "with unsimilar clusterv1 cluster name object",
			actual: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test1",
					Namespace:    "test-namespace",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: false,
					Conditions: clusterv1.Conditions{
						*TrueCondition(clusterv1.ControlPlaneInitializedCondition),
					},
				},
			},
			expected: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test2",
					Namespace:    "test-namespace",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: false,
					Conditions: clusterv1.Conditions{
						*TrueCondition(clusterv1.ControlPlaneInitializedCondition),
					},
				},
			},
			expectMatch: false,
		},
		{
			name: "with unsimilar clusterv1 cluster conditions object",
			actual: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test1",
					Namespace:    "test-namespace",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: false,
					Conditions: clusterv1.Conditions{
						*TrueCondition(clusterv1.ControlPlaneInitializedCondition),
					},
				},
			},
			expected: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test1",
					Namespace:    "test-namespace",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: false,
					Conditions: clusterv1.Conditions{
						*FalseCondition(clusterv1.ControlPlaneInitializedCondition, "", clusterv1.ConditionSeverityInfo, ""),
					},
				},
			},
			expectMatch: false,
		},
		{
			name: "with similar unstructed object",
			actual: &unstructv1.Unstructured{
				Object: map[string]interface{}{
					"kind":       "GenericBootstrapConfig",
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"generateName": "test-bootstrap-",
						"namespace":    "test-namespace",
					},
					"status": map[string]interface{}{
						"ready":          true,
						"dataSecretName": "data",
					},
				},
			},
			expected: &unstructv1.Unstructured{
				Object: map[string]interface{}{
					"kind":       "GenericBootstrapConfig",
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"generateName": "test-bootstrap-",
						"namespace":    "test-namespace",
					},
					"status": map[string]interface{}{
						"ready":          true,
						"dataSecretName": "data",
					},
				},
			},
			expectMatch: true,
		},
		{
			name: "with unsimilar dataSecretName type object",
			actual: &unstructv1.Unstructured{
				Object: map[string]interface{}{
					"kind":       "GenericBootstrapConfig",
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"generateName": "test-bootstrap-",
						"namespace":    "test-namespace",
					},
					"status": map[string]interface{}{
						"ready":          true,
						"dataSecretName": "data1",
					},
				},
			},
			expected: &unstructv1.Unstructured{
				Object: map[string]interface{}{
					"kind":       "GenericBootstrapConfig",
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"generateName": "test-bootstrap-",
						"namespace":    "test-namespace",
					},
					"status": map[string]interface{}{
						"ready":          true,
						"dataSecretName": "data2",
					},
				},
			},
			expectMatch: false,
		},
		{
			name: "with unsimilar types",
			actual: &unstructv1.Unstructured{
				Object: map[string]interface{}{
					"kind":       "GenericBootstrapConfig",
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"generateName": "test-bootstrap-",
						"namespace":    "test-namespace",
					},
					"status": map[string]interface{}{
						"ready":          true,
						"dataSecretName": "data",
					},
				},
			},
			expected: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test",
					Namespace:    "test-namespace",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: false,
					Conditions: clusterv1.Conditions{
						*TrueCondition(clusterv1.ControlPlaneInitializedCondition),
					},
				},
			},
			expectMatch: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			result, _, _ := Check(tc.actual, tc.expected)
			g.Expect(result).To(Equal(tc.expectMatch))
		})
	}
}
