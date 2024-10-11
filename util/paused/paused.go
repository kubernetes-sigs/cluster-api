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
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
)

type pausedConditionSetter interface {
	v1beta2conditions.Setter
	client.Object
}

// EnsurePausedOption is some configuration that modifies options for a EnsurePausedCondition request.
type EnsurePausedOption interface {
	// ApplyToSet applies this configuration to the given EnsurePausedCondition options.
	ApplyToSet(option *EnsurePausedOptions)
}

// EnsurePausedOptions allows to define options for the EnsurePausedCondition operation.
type EnsurePausedOptions struct {
	targetConditionType string
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *EnsurePausedOptions) ApplyOptions(opts []EnsurePausedOption) *EnsurePausedOptions {
	for _, opt := range opts {
		opt.ApplyToSet(o)
	}
	return o
}

// EnsurePausedCondition sets the paused condition on the object and returns if it should be considered as paused.
func EnsurePausedCondition[T pausedConditionSetter](ctx context.Context, c client.Client, cluster *clusterv1.Cluster, o T, opts ...EnsurePausedOption) (isPaused bool, conditionChanged bool, err error) {
	op := &EnsurePausedOptions{
		targetConditionType: clusterv1.PausedV1Beta2Condition,
	}
	for _, opt := range opts {
		opt.ApplyToSet(op)
	}

	oldCondition := v1beta2conditions.Get(o, op.targetConditionType)
	newCondition := pausedCondition(cluster, o, op.targetConditionType)

	isPaused = newCondition.Status == metav1.ConditionTrue

	// Return early if the paused condition did not change.
	if oldCondition != nil &&
		oldCondition.Status == newCondition.Status &&
		oldCondition.Reason == newCondition.Reason &&
		oldCondition.Message == newCondition.Message {
		return isPaused, false, nil
	}

	patchHelper, err := patch.NewHelper(o, c)
	if err != nil {
		return isPaused, false, err
	}

	v1beta2conditions.Set(o, newCondition)

	if isPaused {
		log := ctrl.LoggerFrom(ctx)
		log.Info("Reconciliation is paused for this object")
	}

	return newCondition.Status == metav1.ConditionTrue, true,
		patchHelper.Patch(ctx, o, patch.WithOwnedV1Beta2Conditions{Conditions: []string{
			op.targetConditionType,
		}})
}

// pausedCondition sets the paused condition on the object and returns if it should be considered as paused.
func pausedCondition[T pausedConditionSetter](cluster *clusterv1.Cluster, o T, targetConditionType string) metav1.Condition {
	if (cluster == nil && annotations.HasPaused(o)) || (cluster != nil && annotations.IsPaused(cluster, o)) {
		var messages []string
		if cluster != nil && cluster.Spec.Paused {
			messages = append(messages, "Cluster spec.paused is set to true")
		}
		if annotations.HasPaused(o) {
			messages = append(messages, fmt.Sprintf("%s has the cluster.x-k8s.io/paused annotation", o.GetObjectKind().GroupVersionKind().Kind))
		}

		return metav1.Condition{
			Type:    targetConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.PausedV1Beta2Reason,
			Message: strings.Join(messages, ", "),
		}
	}

	return metav1.Condition{
		Type:   targetConditionType,
		Status: metav1.ConditionFalse,
		Reason: clusterv1.NotPausedV1Beta2Reason,
	}
}
