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

package clusterclass

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

func updateStatus(ctx context.Context, s *scope) {
	setRefVersionsUpToDateCondition(ctx, s.clusterClass, s.outdatedExternalReferences, s.reconcileExternalReferencesError)
	setVariablesReconciledCondition(ctx, s.clusterClass, s.variableDiscoveryError)
}

func setRefVersionsUpToDateCondition(_ context.Context, clusterClass *clusterv1.ClusterClass, outdatedRefs []outdatedRef, reconcileExternalReferencesError error) {
	if reconcileExternalReferencesError != nil {
		v1beta1conditions.MarkUnknown(clusterClass,
			clusterv1.ClusterClassRefVersionsUpToDateCondition,
			clusterv1.ClusterClassRefVersionsUpToDateInternalErrorReason,
			"Please check controller logs for errors",
		)
		conditions.Set(clusterClass, metav1.Condition{
			Type:    clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterClassRefVersionsUpToDateInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(outdatedRefs) > 0 {
		var msg []string
		for _, outdatedRef := range outdatedRefs {
			msg = append(msg, fmt.Sprintf("* Ref %q should be %q", refString(outdatedRef.Outdated), refString(outdatedRef.UpToDate)))
		}
		v1beta1conditions.Set(clusterClass,
			v1beta1conditions.FalseCondition(
				clusterv1.ClusterClassRefVersionsUpToDateCondition,
				clusterv1.ClusterClassOutdatedRefVersionsReason,
				clusterv1.ConditionSeverityWarning,
				strings.Join(msg, "\n"),
			),
		)
		conditions.Set(clusterClass, metav1.Condition{
			Type:    clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterClassRefVersionsNotUpToDateV1Beta2Reason,
			Message: strings.Join(msg, "\n"),
		})
		return
	}

	v1beta1conditions.Set(clusterClass,
		v1beta1conditions.TrueCondition(clusterv1.ClusterClassRefVersionsUpToDateCondition),
	)
	conditions.Set(clusterClass, metav1.Condition{
		Type:   clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Reason,
	})
}

func setVariablesReconciledCondition(_ context.Context, clusterClass *clusterv1.ClusterClass, variableDiscoveryError error) {
	if variableDiscoveryError != nil {
		v1beta1conditions.MarkFalse(clusterClass,
			clusterv1.ClusterClassVariablesReconciledCondition,
			clusterv1.VariableDiscoveryFailedReason,
			clusterv1.ConditionSeverityError,
			variableDiscoveryError.Error(),
		)
		conditions.Set(clusterClass, metav1.Condition{
			Type:    clusterv1.ClusterClassVariablesReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterClassVariablesReadyVariableDiscoveryFailedV1Beta2Reason,
			Message: variableDiscoveryError.Error(),
		})
		return
	}

	v1beta1conditions.MarkTrue(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition)
	conditions.Set(clusterClass, metav1.Condition{
		Type:   clusterv1.ClusterClassVariablesReadyV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterClassVariablesReadyV1Beta2Reason,
	})
}
