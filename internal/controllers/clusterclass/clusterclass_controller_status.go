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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

func updateStatus(ctx context.Context, s *scope) {
	setRefVersionsUpToDateCondition(ctx, s.clusterClass, s.outdatedExternalReferences, s.reconcileExternalReferencesError)
	setVariablesReconciledCondition(ctx, s.clusterClass, s.variableDiscoveryError)
}

func setRefVersionsUpToDateCondition(_ context.Context, clusterClass *clusterv1.ClusterClass, outdatedRefs []outdatedRef, reconcileExternalReferencesError error) {
	if reconcileExternalReferencesError != nil {
		conditions.MarkUnknown(clusterClass,
			clusterv1.ClusterClassRefVersionsUpToDateCondition,
			clusterv1.ClusterClassRefVersionsUpToDateInternalErrorReason,
			"Please check controller logs for errors",
		)
		v1beta2conditions.Set(clusterClass, metav1.Condition{
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
		conditions.Set(clusterClass,
			conditions.FalseCondition(
				clusterv1.ClusterClassRefVersionsUpToDateCondition,
				clusterv1.ClusterClassOutdatedRefVersionsReason,
				clusterv1.ConditionSeverityWarning,
				strings.Join(msg, "\n"),
			),
		)
		v1beta2conditions.Set(clusterClass, metav1.Condition{
			Type:    clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterClassRefVersionsNotUpToDateV1Beta2Reason,
			Message: strings.Join(msg, "\n"),
		})
		return
	}

	conditions.Set(clusterClass,
		conditions.TrueCondition(clusterv1.ClusterClassRefVersionsUpToDateCondition),
	)
	v1beta2conditions.Set(clusterClass, metav1.Condition{
		Type:   clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Reason,
	})
}

func setVariablesReconciledCondition(_ context.Context, clusterClass *clusterv1.ClusterClass, variableDiscoveryError error) {
	if variableDiscoveryError != nil {
		conditions.MarkFalse(clusterClass,
			clusterv1.ClusterClassVariablesReconciledCondition,
			clusterv1.VariableDiscoveryFailedReason,
			clusterv1.ConditionSeverityError,
			variableDiscoveryError.Error(),
		)
		v1beta2conditions.Set(clusterClass, metav1.Condition{
			Type:    clusterv1.ClusterClassVariablesReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterClassVariablesReadyVariableDiscoveryFailedV1Beta2Reason,
			Message: variableDiscoveryError.Error(),
		})
		return
	}

	conditions.MarkTrue(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition)
	v1beta2conditions.Set(clusterClass, metav1.Condition{
		Type:   clusterv1.ClusterClassVariablesReadyV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterClassVariablesReadyV1Beta2Reason,
	})
}
