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

package v1beta2

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util/annotations"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

type pausedConditionSetter interface {
	Setter
	metav1.Object
}

// SetPausedCondition sets the paused condition on the object and returns if it should be considered as paused.
func SetPausedCondition[T pausedConditionSetter](cluster *clusterv1.Cluster, ms T) (isPaused bool) {
	if annotations.IsPaused(cluster, ms) {
		msgs := []string{}
		if cluster.Spec.Paused {
			msgs = append(msgs, "Cluster: .spec.paused is set to true")
		}
		if annotations.HasPaused(ms) {
			msgs = append(msgs, fmt.Sprintf("Annotations: paused annotation is set"))
		}

		Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetPausedV1Beta2Condition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.ResourcePausedV1Beta2Reason,
			Message: strings.Join(msgs, "; "),
		})
		return true
	}

	Set(ms, metav1.Condition{
		Type:   clusterv1.MachineSetPausedV1Beta2Condition,
		Status: metav1.ConditionFalse,
		Reason: clusterv1.ResourceNotPausedV1Beta2Reason,
	})
	return false
}
