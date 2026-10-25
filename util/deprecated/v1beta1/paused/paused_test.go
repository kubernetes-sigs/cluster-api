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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/deprecated/v1beta1/conditions/v1beta2"
	"sigs.k8s.io/cluster-api/util/deprecated/v1beta1/test/builder"
)

func TestEnsurePausedCondition(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(builder.AddTransitionV1Beta2ToScheme(scheme)).To(Succeed())
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	// Cluster Case 1: unpaused
	normalCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-cluster",
			Namespace: "default",
		},
	}

	// Cluster Case 2: paused
	pausedCluster := normalCluster.DeepCopy()
	pausedCluster.Spec.Paused = ptr.To(true)

	// Object case 1: unpaused
	obj := &builder.Phase1Obj{ObjectMeta: metav1.ObjectMeta{
		Name:       "some-object",
		Namespace:  "default",
		Generation: 1,
	}}

	// Object case 2: paused
	pausedObj := obj.DeepCopy()
	pausedObj.SetAnnotations(map[string]string{clusterv1beta1.PausedAnnotation: ""})

	tests := []struct {
		name                             string
		cluster                          *clusterv1.Cluster
		object                           ConditionSetter
		wantIsPaused                     bool
		wantRequeueAfterGenerationChange bool
	}{
		{
			name:                             "unpaused cluster and unpaused object",
			cluster:                          normalCluster.DeepCopy(),
			object:                           obj.DeepCopy(),
			wantIsPaused:                     false,
			wantRequeueAfterGenerationChange: false, // We don't want a requeue in this case to avoid additional reconciles.
		},
		{
			name:                             "paused cluster and unpaused object",
			cluster:                          pausedCluster.DeepCopy(),
			object:                           obj.DeepCopy(),
			wantIsPaused:                     true,
			wantRequeueAfterGenerationChange: true,
		},
		{
			name:                             "unpaused cluster and paused object",
			cluster:                          normalCluster.DeepCopy(),
			object:                           pausedObj.DeepCopy(),
			wantIsPaused:                     true,
			wantRequeueAfterGenerationChange: true,
		},
		{
			name:                             "paused cluster and paused object",
			cluster:                          pausedCluster.DeepCopy(),
			object:                           pausedObj.DeepCopy(),
			wantIsPaused:                     true,
			wantRequeueAfterGenerationChange: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.Background()

			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&clusterv1.Cluster{}, &builder.Phase1Obj{}).
				WithObjects(tt.object, tt.cluster).Build()

			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(tt.object), tt.object)).To(Succeed())

			// The first run should set the condition.
			gotIsPaused, gotRequeue, err := EnsurePausedCondition(ctx, c, tt.cluster, tt.object)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(gotRequeue).To(BeTrue(), "The first reconcile should return requeue=true because it set the Paused condition")
			g.Expect(gotIsPaused).To(Equal(tt.wantIsPaused))
			assertCondition(g, tt.object, tt.wantIsPaused)

			// The second reconcile should be a no-op.
			gotIsPaused, gotRequeue, err = EnsurePausedCondition(ctx, c, tt.cluster, tt.object)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(gotRequeue).To(BeFalse(), "The second reconcile should return requeue=false as the Paused condition was not changed")
			g.Expect(gotIsPaused).To(Equal(tt.wantIsPaused))
			assertCondition(g, tt.object, tt.wantIsPaused)

			// The third reconcile reconciles a generation change, condition should be updated and requeue=true
			// should only be returned if the object is paused.
			tt.object.SetGeneration(tt.object.GetGeneration() + 1)
			gotIsPaused, gotRequeue, err = EnsurePausedCondition(ctx, c, tt.cluster, tt.object)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(gotRequeue).To(Equal(tt.wantRequeueAfterGenerationChange))
			g.Expect(gotIsPaused).To(Equal(tt.wantIsPaused))
			assertCondition(g, tt.object, tt.wantIsPaused)
		})
	}
}

func assertCondition(g Gomega, object ConditionSetter, wantIsPaused bool) {
	condition := v1beta2conditions.Get(object, clusterv1beta1.PausedV1Beta2Condition)
	g.Expect(condition.ObservedGeneration).To(Equal(object.GetGeneration()))
	if wantIsPaused {
		g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		g.Expect(condition.Reason).To(Equal(clusterv1beta1.PausedV1Beta2Reason))
		g.Expect(condition.Message).ToNot(BeEmpty())
	} else {
		g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(condition.Reason).To(Equal(clusterv1beta1.NotPausedV1Beta2Reason))
		g.Expect(condition.Message).To(BeEmpty())
	}
}
