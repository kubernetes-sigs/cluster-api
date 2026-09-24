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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// +kubebuilder:object:generate=true

// Conditions is a list of metav1.Condition.
type Conditions []metav1.Condition

// UnmarshalJSON unmarshals a Conditions list, keeping only the Ready condition.
func (in *Conditions) UnmarshalJSON(b []byte) error {
	parsedConditions := []metav1.Condition{}
	if err := yaml.Unmarshal(b, &parsedConditions); err != nil {
		return fmt.Errorf("failed to unmarshal conditions: %w", err)
	}
	for _, c := range parsedConditions {
		if c.Type != "Ready" {
			continue
		}

		*in = append(*in, c)
	}
	return nil
}

// +kubebuilder:object:generate=true

// V1Beta1Conditions is a list of clusterv1.Condition.
type V1Beta1Conditions []clusterv1.Condition

// UnmarshalJSON unmarshals a Conditions list, keeping only the Ready condition.
func (in *V1Beta1Conditions) UnmarshalJSON(b []byte) error {
	parsedConditions := []clusterv1.Condition{}
	if err := yaml.Unmarshal(b, &parsedConditions); err != nil {
		return fmt.Errorf("failed to unmarshal conditions: %w", err)
	}
	for _, c := range parsedConditions {
		if c.Type != "Ready" {
			continue
		}

		*in = append(*in, c)
	}
	return nil
}
