/*
Copyright The Kubernetes Authors.

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

package api

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// +kubebuilder:object:generate=true

// Conditions is a list of Condition.
type Conditions []Condition

// +kubebuilder:object:generate=true

// Condition defines a condition and contains all fields from metav1.Condition and the CAPI Condition type.
// This is needed because our controllers are handling both condition types on v1beta1 & v1beta2.
type Condition struct {
	Type               string      `json:"type,omitempty"`
	Status             string      `json:"status,omitempty"`
	ObservedGeneration int64       `json:"observedGeneration,omitempty"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	Reason             string      `json:"reason,omitempty"`
	Message            string      `json:"message,omitempty"`
	Severity           string      `json:"severity,omitempty"`
}

// UnmarshalJSON unmarshals a Conditions list, keeping only the Ready condition.
func (in *Conditions) UnmarshalJSON(b []byte) error {
	conditions := []Condition{}
	if err := yaml.Unmarshal(b, &conditions); err != nil {
		return fmt.Errorf("failed to unmarshal conditions: %w", err)
	}
	for _, c := range conditions {
		if c.Type != "Ready" {
			continue
		}

		*in = append(*in, c)
	}
	return nil
}

// ReadyConditionAsAnyArray converts the Conditions list to a []any suitable for setting on an unstructured object.
func (in *Conditions) ReadyConditionAsAnyArray() []any {
	if len(*in) == 0 {
		return nil
	}

	var conditions []any
	for _, c := range *in {
		if c.Type != "Ready" {
			continue
		}

		condition := map[string]any{}
		if c.Type != "" {
			condition["type"] = c.Type
		}
		if c.Status != "" {
			condition["status"] = c.Status
		}
		if c.ObservedGeneration != 0 {
			condition["observedGeneration"] = c.ObservedGeneration
		}
		if !c.LastTransitionTime.IsZero() {
			condition["lastTransitionTime"] = c.LastTransitionTime.Format(time.RFC3339)
		}
		if c.Reason != "" {
			condition["reason"] = c.Reason
		}
		if c.Message != "" {
			condition["message"] = c.Message
		}
		if c.Severity != "" {
			condition["severity"] = c.Severity
		}
		conditions = append(conditions, condition)
	}
	return conditions
}
