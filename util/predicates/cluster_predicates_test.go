/*
Copyright 2022 The Kubernetes Authors.

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

package predicates_test

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/predicates"
)

func TestClusterControlplaneInitializedPredicate(t *testing.T) {
	g := NewWithT(t)
	predicate := predicates.ClusterControlPlaneInitialized(runtime.NewScheme(), logr.New(log.NullLogSink{}))

	markedFalse := clusterv1.Cluster{}
	conditions.MarkFalse(&markedFalse, clusterv1.ControlPlaneInitializedCondition, clusterv1.MissingNodeRefReason, clusterv1.ConditionSeverityWarning, "")

	markedTrue := clusterv1.Cluster{}
	conditions.MarkTrue(&markedTrue, clusterv1.ControlPlaneInitializedCondition)

	notMarked := clusterv1.Cluster{}

	testcases := []struct {
		name       string
		oldCluster clusterv1.Cluster
		newCluster clusterv1.Cluster
		expected   bool
	}{
		{
			name:       "no conditions -> no conditions: should return false",
			oldCluster: notMarked,
			newCluster: notMarked,
			expected:   false,
		},
		{
			name:       "no conditions -> true: should return true",
			oldCluster: notMarked,
			newCluster: markedTrue,
			expected:   true,
		},
		{
			name:       "false -> true: should return true",
			oldCluster: markedFalse,
			newCluster: markedTrue,
			expected:   true,
		},
		{
			name:       "no conditions -> false: should return false",
			oldCluster: notMarked,
			newCluster: markedFalse,
			expected:   false,
		},
		{
			name:       "true -> false: should return false",
			oldCluster: markedTrue,
			newCluster: markedFalse,
			expected:   false,
		},
		{
			name:       "true -> true: should return false",
			oldCluster: markedTrue,
			newCluster: markedTrue,
			expected:   false,
		},
	}

	for i := range testcases {
		tc := testcases[i]
		t.Run(tc.name, func(*testing.T) {
			ev := event.UpdateEvent{
				ObjectOld: &tc.oldCluster,
				ObjectNew: &tc.newCluster,
			}

			g.Expect(predicate.Update(ev)).To(Equal(tc.expected))
		})
	}
}
