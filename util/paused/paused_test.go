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

// Package paused implements paused helper functions.
package paused

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

func TestEnsurePausedCondition(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	normalCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-cluster",
			Namespace: "default",
		},
	}
	normalClusterWithCondition := normalCluster.DeepCopy()
	v1beta2conditions.Set(normalClusterWithCondition, pausedCondition(normalClusterWithCondition, normalClusterWithCondition, clusterv1.PausedV1Beta2Condition))

	pausedSpecCluster := normalCluster.DeepCopy()
	pausedSpecCluster.Spec.Paused = true
	pausedSpecClusterWithCondition := pausedSpecCluster.DeepCopy()
	v1beta2conditions.Set(pausedSpecClusterWithCondition, pausedCondition(pausedSpecClusterWithCondition, pausedSpecClusterWithCondition, clusterv1.PausedV1Beta2Condition))

	pausedAnnotationCluster := normalCluster.DeepCopy()
	pausedAnnotationCluster.SetAnnotations(map[string]string{clusterv1.PausedAnnotation: "true"})
	pausedAnnotationClusterWithCondition := pausedAnnotationCluster.DeepCopy()
	v1beta2conditions.Set(pausedAnnotationClusterWithCondition, pausedCondition(pausedAnnotationClusterWithCondition, pausedAnnotationClusterWithCondition, clusterv1.PausedV1Beta2Condition))

	pausedSpecAnnotationCluster := pausedSpecCluster.DeepCopy()
	pausedSpecAnnotationCluster.SetAnnotations(map[string]string{clusterv1.PausedAnnotation: "true"})
	pausedSpecAnnotationClusterWithCondition := pausedSpecAnnotationCluster.DeepCopy()
	v1beta2conditions.Set(pausedSpecAnnotationClusterWithCondition, pausedCondition(pausedSpecAnnotationClusterWithCondition, pausedSpecAnnotationClusterWithCondition, clusterv1.PausedV1Beta2Condition))

	tests := []struct {
		name                 string
		cluster              *clusterv1.Cluster
		object               pausedConditionSetter
		wantIsPaused         bool
		wantConditionChanged bool
		wantErr              bool
	}{
		{
			name:                 "not paused cluster without condition",
			cluster:              normalCluster.DeepCopy(),
			object:               normalCluster.DeepCopy(),
			wantIsPaused:         false,
			wantConditionChanged: true,
			wantErr:              false,
		},
		{
			name:                 "not paused cluster with condition",
			cluster:              normalClusterWithCondition.DeepCopy(),
			object:               normalClusterWithCondition.DeepCopy(),
			wantIsPaused:         false,
			wantConditionChanged: false,
			wantErr:              false,
		},
		{
			name:                 "paused cluster via spec without condition",
			cluster:              pausedSpecCluster.DeepCopy(),
			object:               pausedSpecCluster.DeepCopy(),
			wantIsPaused:         true,
			wantConditionChanged: true,
			wantErr:              false,
		},
		{
			name:                 "paused cluster via spec with condition",
			cluster:              pausedSpecClusterWithCondition.DeepCopy(),
			object:               pausedSpecClusterWithCondition.DeepCopy(),
			wantIsPaused:         true,
			wantConditionChanged: false,
			wantErr:              false,
		},
		{
			name:                 "paused cluster via annotation without condition",
			cluster:              pausedAnnotationCluster.DeepCopy(),
			object:               pausedAnnotationCluster.DeepCopy(),
			wantIsPaused:         true,
			wantConditionChanged: true,
			wantErr:              false,
		},
		{
			name:                 "paused cluster via annotation with condition",
			cluster:              pausedAnnotationClusterWithCondition.DeepCopy(),
			object:               pausedAnnotationClusterWithCondition.DeepCopy(),
			wantIsPaused:         true,
			wantConditionChanged: false,
			wantErr:              false,
		},
		{
			name:                 "paused cluster via spec and annotation without condition",
			cluster:              pausedSpecAnnotationCluster.DeepCopy(),
			object:               pausedSpecAnnotationCluster.DeepCopy(),
			wantIsPaused:         true,
			wantConditionChanged: true,
			wantErr:              false,
		},
		{
			name:                 "paused cluster via spec and annotation with condition",
			cluster:              pausedSpecAnnotationClusterWithCondition.DeepCopy(),
			object:               pausedSpecAnnotationClusterWithCondition.DeepCopy(),
			wantIsPaused:         true,
			wantConditionChanged: false,
			wantErr:              false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.Background()

			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&clusterv1.Cluster{}).
				WithObjects(tt.object).Build()

			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(tt.object), tt.object)).To(Succeed())

			gotIsPaused, gotConditionChanged, err := EnsurePausedCondition(ctx, c, tt.cluster, tt.object)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsurePausedCondition() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotIsPaused != tt.wantIsPaused {
				t.Errorf("EnsurePausedCondition() gotIsPaused = %v, want %v", gotIsPaused, tt.wantIsPaused)
			}
			if gotConditionChanged != tt.wantConditionChanged {
				t.Errorf("EnsurePausedCondition() gotConditionChanged = %v, want %v", gotConditionChanged, tt.wantConditionChanged)
			}
		})
	}
}
