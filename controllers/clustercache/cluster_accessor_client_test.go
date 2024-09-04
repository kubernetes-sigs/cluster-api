/*
Copyright 2024 The Kubernetes Authors.

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

package clustercache

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRunningOnWorkloadCluster(t *testing.T) {
	tests := []struct {
		name                      string
		currentControllerMetadata *metav1.ObjectMeta
		clusterObjects            []client.Object
		expected                  bool
	}{
		{
			name: "should return true if the controller is running on the workload cluster",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod",
						Namespace: "controller-pod-namespace",
						UID:       types.UID("controller-pod-uid"),
					},
				},
			},
			expected: true,
		},
		{
			name: "should return false if the controller is not running on the workload cluster: name mismatch",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod-mismatch",
						Namespace: "controller-pod-namespace",
						UID:       types.UID("controller-pod-uid"),
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false if the controller is not running on the workload cluster: namespace mismatch",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod",
						Namespace: "controller-pod-namespace-mismatch",
						UID:       types.UID("controller-pod-uid"),
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false if the controller is not running on the workload cluster: uid mismatch",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod",
						Namespace: "controller-pod-namespace",
						UID:       types.UID("controller-pod-uid-mismatch"),
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false if the controller is not running on the workload cluster: no pod in cluster",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{},
			expected:       false,
		},
		{
			name:                      "should return false if the controller is not running on the workload cluster: no controller metadata",
			currentControllerMetadata: nil,
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod",
						Namespace: "controller-pod-namespace",
						UID:       types.UID("controller-pod-uid"),
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithObjects(tt.clusterObjects...).Build()

			found, err := runningOnWorkloadCluster(ctx, tt.currentControllerMetadata, c)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(found).To(Equal(tt.expected))
		})
	}
}
