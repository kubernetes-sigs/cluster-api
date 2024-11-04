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

package test

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func Test_validate(t *testing.T) {
	invalidSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Operator: "Invalid-Operator",
			},
		},
	}

	tests := []struct {
		name             string
		machineDrainRule *clusterv1.MachineDrainRule
		wantErr          string
	}{
		{
			name: "Return no error if MachineDrainRule is valid",
			machineDrainRule: &clusterv1.MachineDrainRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mdr",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineDrainRuleSpec{
					Drain: clusterv1.MachineDrainRuleDrainConfig{
						Behavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
						Order:    ptr.To[int32](5),
					},
					Pods: []clusterv1.MachineDrainRulePodSelector{
						{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "prometheus",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "monitoring",
								},
							},
						},
					},
					Machines: []clusterv1.MachineDrainRuleMachineSelector{
						{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"os": "linux",
								},
							},
							ClusterSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"stage": "production",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Return error if order is set with drain behavior Skip",
			machineDrainRule: &clusterv1.MachineDrainRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mdr",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineDrainRuleSpec{
					Drain: clusterv1.MachineDrainRuleDrainConfig{
						Behavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
						Order:    ptr.To[int32](5),
					},
				},
			},
			wantErr: "admission webhook \"validation.machinedrainrule.cluster.x-k8s.io\" denied the request: " +
				"MachineDrainRule.cluster.x-k8s.io \"mdr\" is invalid: " +
				"spec.drain.order: Invalid value: 5: order must not be set if drain behavior is \"Skip\"",
		},
		{
			name: "Return error for MachineDrainRules with invalid selector",
			machineDrainRule: &clusterv1.MachineDrainRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mdr",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineDrainRuleSpec{
					Drain: clusterv1.MachineDrainRuleDrainConfig{
						Behavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
					},
					Machines: []clusterv1.MachineDrainRuleMachineSelector{
						{
							Selector:        invalidSelector,
							ClusterSelector: invalidSelector,
						},
					},
					Pods: []clusterv1.MachineDrainRulePodSelector{
						{
							Selector:          invalidSelector,
							NamespaceSelector: invalidSelector,
						},
					},
				},
			},
			wantErr: "admission webhook \"validation.machinedrainrule.cluster.x-k8s.io\" denied the request: " +
				"MachineDrainRule.cluster.x-k8s.io \"mdr\" is invalid: [" +
				"spec.machines[0].selector: Invalid value: v1.LabelSelector{MatchLabels:map[string]string(nil), MatchExpressions:[]v1.LabelSelectorRequirement{v1.LabelSelectorRequirement{Key:\"\", Operator:\"Invalid-Operator\", Values:[]string(nil)}}}: \"Invalid-Operator\" is not a valid label selector operator, " +
				"spec.machines[0].clusterSelector: Invalid value: v1.LabelSelector{MatchLabels:map[string]string(nil), MatchExpressions:[]v1.LabelSelectorRequirement{v1.LabelSelectorRequirement{Key:\"\", Operator:\"Invalid-Operator\", Values:[]string(nil)}}}: \"Invalid-Operator\" is not a valid label selector operator, " +
				"spec.pods[0].selector: Invalid value: v1.LabelSelector{MatchLabels:map[string]string(nil), MatchExpressions:[]v1.LabelSelectorRequirement{v1.LabelSelectorRequirement{Key:\"\", Operator:\"Invalid-Operator\", Values:[]string(nil)}}}: \"Invalid-Operator\" is not a valid label selector operator, " +
				"spec.pods[0].namespaceSelector: Invalid value: v1.LabelSelector{MatchLabels:map[string]string(nil), MatchExpressions:[]v1.LabelSelectorRequirement{v1.LabelSelectorRequirement{Key:\"\", Operator:\"Invalid-Operator\", Values:[]string(nil)}}}: \"Invalid-Operator\" is not a valid label selector operator]",
		},
		{
			name: "Return error if selectors are not unique",
			machineDrainRule: &clusterv1.MachineDrainRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mdr",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineDrainRuleSpec{
					Drain: clusterv1.MachineDrainRuleDrainConfig{
						Behavior: clusterv1.MachineDrainRuleDrainBehaviorSkip,
					},
					Machines: []clusterv1.MachineDrainRuleMachineSelector{
						{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"os": "linux",
								},
							},
							ClusterSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"stage": "production",
								},
							},
						},
						{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"os": "linux",
								},
							},
							ClusterSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"stage": "production",
								},
							},
						},
					},
					Pods: []clusterv1.MachineDrainRulePodSelector{
						{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "does-not-match",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "monitoring",
								},
							},
						},
						{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "does-not-match",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "monitoring",
								},
							},
						},
					},
				},
			},
			wantErr: "admission webhook \"validation.machinedrainrule.cluster.x-k8s.io\" denied the request: " +
				"MachineDrainRule.cluster.x-k8s.io \"mdr\" is invalid: [" +
				"spec.machines: Forbidden: Entries in machines must be unique, " +
				"spec.pods: Forbidden: Entries in pods must be unique]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := env.CreateAndWait(ctx, tt.machineDrainRule)

			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(BeComparableTo(tt.wantErr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(env.CleanupAndWait(ctx, tt.machineDrainRule)).To(Succeed())
			}
		})
	}
}
