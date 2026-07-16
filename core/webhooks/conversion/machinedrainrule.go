/*
Copyright 2026 The Kubernetes Authors.

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

package conversion

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// MachineDrainRule is a HubSpokeConverter for the MachineDrainRule API type.
var MachineDrainRule = conversion.NewHubSpokeConverter(&clusterv1.MachineDrainRule{},
	conversion.NewSpokeConverter(&clusterv1beta1.MachineDrainRule{}, ConvertMachineDrainRuleHubToV1Beta1, ConvertMachineDrainRuleV1Beta1ToHub),
)

// ConvertMachineDrainRuleV1Beta1ToHub converts a v1beta1 MachineDrainRule to a hub MachineDrainRule.
func ConvertMachineDrainRuleV1Beta1ToHub(_ context.Context, src *clusterv1beta1.MachineDrainRule, dst *clusterv1.MachineDrainRule) error {
	return clusterv1beta1.Convert_v1beta1_MachineDrainRule_To_v1beta2_MachineDrainRule(src, dst, nil)
}

// ConvertMachineDrainRuleHubToV1Beta1 converts a hub MachineDrainRule to a v1beta1 MachineDrainRule.
func ConvertMachineDrainRuleHubToV1Beta1(_ context.Context, src *clusterv1.MachineDrainRule, dst *clusterv1beta1.MachineDrainRule) error {
	return clusterv1beta1.Convert_v1beta2_MachineDrainRule_To_v1beta1_MachineDrainRule(src, dst, nil)
}
