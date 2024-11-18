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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

func TestSetRefVersionsUpToDateCondition(t *testing.T) {
	testCases := []struct {
		name                             string
		outdatedExternalReferences       []outdatedRef
		reconcileExternalReferencesError error
		expectCondition                  metav1.Condition
	}{
		{
			name:                             "error occurred",
			reconcileExternalReferencesError: errors.New("failed to set ClusterClass owner reference for KubeadmControlPlaneTemplate test-kcp"),
			expectCondition: metav1.Condition{
				Type:    clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterClassRefVersionsUpToDateInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name: "some refs are outdated",
			outdatedExternalReferences: []outdatedRef{
				{
					Outdated: &corev1.ObjectReference{
						APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
						Kind:       "KubeadmControlPlaneTemplate",
						Name:       "test-kcp",
						Namespace:  metav1.NamespaceDefault,
					},
					UpToDate: &corev1.ObjectReference{
						APIVersion: "controlplane.cluster.x-k8s.io/v1beta2",
						Kind:       "KubeadmControlPlaneTemplate",
						Name:       "test-kcp",
						Namespace:  metav1.NamespaceDefault,
					},
				},
				{
					Outdated: &corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "DockerMachineTemplate",
						Name:       "test-dmt",
						Namespace:  metav1.NamespaceDefault,
					},
					UpToDate: &corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
						Kind:       "DockerMachineTemplate",
						Name:       "test-dmt",
						Namespace:  metav1.NamespaceDefault,
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterClassRefVersionsNotUpToDateV1Beta2Reason,
				Message: "* Ref \"controlplane.cluster.x-k8s.io/v1beta1, Kind=KubeadmControlPlaneTemplate default/test-kcp\" should be " +
					"\"controlplane.cluster.x-k8s.io/v1beta2, Kind=KubeadmControlPlaneTemplate default/test-kcp\"\n" +
					"* Ref \"infrastructure.cluster.x-k8s.io/v1beta1, Kind=DockerMachineTemplate default/test-dmt\" should be " +
					"\"infrastructure.cluster.x-k8s.io/v1beta2, Kind=DockerMachineTemplate default/test-dmt\"",
			},
		},
		{
			name:                       "all refs are up-to-date",
			outdatedExternalReferences: []outdatedRef{},
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Reason,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			cc := &clusterv1.ClusterClass{}

			setRefVersionsUpToDateCondition(ctx, cc, tc.outdatedExternalReferences, tc.reconcileExternalReferencesError)

			condition := v1beta2conditions.Get(cc, clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetVariablesReconciledCondition(t *testing.T) {
	testCases := []struct {
		name                   string
		variableDiscoveryError error
		expectCondition        metav1.Condition
	}{
		{
			name: "error occurred",
			variableDiscoveryError: errors.New("VariableDiscovery failed: patch1.variables[httpProxy].schema.openAPIV3Schema.properties[noProxy].type: Unsupported value: \"invalidType\": " +
				"supported values: \"array\", \"boolean\", \"integer\", \"number\", \"object\", \"string\""),
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterClassVariablesReadyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.ClusterClassVariablesReadyVariableDiscoveryFailedV1Beta2Reason,
				Message: "VariableDiscovery failed: patch1.variables[httpProxy].schema.openAPIV3Schema.properties[noProxy].type: Unsupported value: \"invalidType\": " +
					"supported values: \"array\", \"boolean\", \"integer\", \"number\", \"object\", \"string\"",
			},
		},
		{
			name: "variable reconcile succeeded",
			expectCondition: metav1.Condition{
				Type:   clusterv1.ClusterClassVariablesReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterClassVariablesReadyV1Beta2Reason,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			cc := &clusterv1.ClusterClass{}

			setVariablesReconciledCondition(ctx, cc, tc.variableDiscoveryError)

			condition := v1beta2conditions.Get(cc, clusterv1.ClusterClassVariablesReadyV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}
