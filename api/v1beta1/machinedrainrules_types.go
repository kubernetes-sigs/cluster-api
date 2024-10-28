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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// PodDrainLabel is the label that can be set on Pods in workload clusters to ensure a Pod is not drained.
	// The only valid value is "skip".
	// This label takes precedence over MachineDrainRules defined in the management cluster.
	PodDrainLabel = "cluster.x-k8s.io/drain"
)

// MachineDrainRuleDrainBehavior defines the drain behavior. Can be either "Drain" or "Skip".
// +kubebuilder:validation:Enum=Drain;Skip
type MachineDrainRuleDrainBehavior string

const (
	// MachineDrainRuleDrainBehaviorDrain means a Pod should be drained.
	MachineDrainRuleDrainBehaviorDrain MachineDrainRuleDrainBehavior = "Drain"

	// MachineDrainRuleDrainBehaviorSkip means the drain for a Pod should be skipped.
	MachineDrainRuleDrainBehaviorSkip MachineDrainRuleDrainBehavior = "Skip"
)

// MachineDrainRuleSpec defines the spec of a MachineDrainRule.
type MachineDrainRuleSpec struct {
	// drain configures if and how Pods are drained.
	// +required
	Drain MachineDrainRuleDrainConfig `json:"drain"`

	// machines defines to which Machines this MachineDrainRule should be applied.
	//
	// If machines is empty, the MachineDrainRule applies to all Machines.
	// If machines contains multiple selectors, the results are ORed.
	// Within a single selector the results of selector and clusterSelector are ANDed.
	// This field is NOT optional.
	//
	// Example: Selects control plane Machines in all Clusters and
	//          Machines with label "os" == "linux" in Clusters with label
	//          "stage" == "production".
	//
	//  - selector:
	//      matchExpressions:
	//      - key: cluster.x-k8s.io/control-plane
	//        operator: Exists
	//  - selector:
	//      matchLabels:
	//        os: linux
	//    clusterSelector:
	//      matchExpressions:
	//      - key: stage
	//        operator: In
	//        values:
	//        - production
	//
	// +required
	// +kubebuilder:validation:MaxItems=32
	Machines []MachineDrainRuleMachineSelector `json:"machines"`

	// pods defines to which Pods this MachineDrainRule should be applied.
	//
	// If pods is empty, the MachineDrainRule applies to all Pods.
	// If pods contains multiple selectors, the results are ORed.
	// Within a single selector the results of selector and namespaceSelector are ANDed.
	// This field is NOT optional.
	//
	// Example: Selects Pods with label "app" == "logging" in all Namespaces and
	//          Pods with label "app" == "prometheus" in the "monitoring"
	//          Namespace.
	//
	//  - selector:
	//      matchExpressions:
	//      - key: app
	//        operator: In
	//        values:
	//        - logging
	//  - selector:
	//      matchLabels:
	//        app: prometheus
	//    namespaceSelector:
	//      matchLabels:
	//        kubernetes.io/metadata.name: monitoring
	//
	// +required
	// +kubebuilder:validation:MaxItems=32
	Pods []MachineDrainRulePodSelector `json:"pods"`
}

// MachineDrainRuleDrainConfig configures if and how Pods are drained.
type MachineDrainRuleDrainConfig struct {
	// behavior defines the drain behavior.
	// Can be either "Drain" or "Skip".
	// +required
	Behavior MachineDrainRuleDrainBehavior `json:"behavior"`

	// order defines the order in which Pods are drained.
	// Pods with higher order are drained after Pods with lower order.
	// order can only be set if behavior is set to "Drain".
	// If order is not set, 0 will be used.
	// +optional
	Order *int32 `json:"order,omitempty"`
}

type MachineDrainRuleMachineSelector struct { //nolint:revive // Intentionally not adding a godoc comment as it would show up additionally to the field comment in the CRD
	// selector is a label selector which selects Machines by their labels.
	// This field follows standard label selector semantics; if not present or
	// empty, it selects all Machines.
	//
	// If clusterSelector is also set, then the selector as a whole selects
	// Machines matching selector belonging to Clusters selected by clusterSelector.
	// If clusterSelector is not set, it selects all Machines matching selector in
	// all Clusters.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// clusterSelector is a label selector which selects Machines by the labels of
	// their Clusters.
	// This field follows standard label selector semantics; if not present or
	// empty, it selects Machines of all Clusters.
	//
	// If selector is also set, then the selector as a whole selects
	// Machines matching selector belonging to Clusters selected by clusterSelector.
	// If selector is not set, it selects all Machines belonging to Clusters
	// selected by clusterSelector.
	// +optional
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
}

type MachineDrainRulePodSelector struct { //nolint:revive // Intentionally not adding a godoc comment as it would show up additionally to the field comment in the CRD
	// selector is a label selector which selects Pods by their labels.
	// This field follows standard label selector semantics; if not present or
	// empty, it selects all Pods.
	//
	// If namespaceSelector is also set, then the selector as a whole selects
	// Pods matching selector in Namespaces selected by namespaceSelector.
	// If namespaceSelector is not set, it selects all Pods matching selector in
	// all Namespaces.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// namespaceSelector is a label selector which selects Pods by the labels of
	// their Namespaces.
	// This field follows standard label selector semantics; if not present or
	// empty, it selects Pods of all Namespaces.
	//
	// If selector is also set, then the selector as a whole selects
	// Pods matching selector in Namespaces selected by namespaceSelector.
	// If selector is not set, it selects all Pods in Namespaces selected by
	// namespaceSelector.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machinedrainrules,shortName=mdr,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Behavior",type="string",JSONPath=".spec.drain.behavior",description="Drain behavior"
// +kubebuilder:printcolumn:name="Order",type="string",JSONPath=".spec.drain.order",description="Drain order"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of the MachineDrainRule"

// MachineDrainRule is the Schema for the MachineDrainRule API.
type MachineDrainRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MachineDrainRuleSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// MachineDrainRuleList contains a list of MachineDrainRules.
type MachineDrainRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineDrainRule `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &MachineDrainRule{}, &MachineDrainRuleList{})
}
