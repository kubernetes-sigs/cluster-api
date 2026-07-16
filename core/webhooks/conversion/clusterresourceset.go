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

	addonsv1beta1 "sigs.k8s.io/cluster-api/api/addons/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
)

// ClusterResourceSet is a HubSpokeConverter for the ClusterResourceSet API type.
var ClusterResourceSet = conversion.NewHubSpokeConverter(&addonsv1.ClusterResourceSet{},
	conversion.NewSpokeConverter(&addonsv1beta1.ClusterResourceSet{}, ConvertClusterResourceSetHubToV1Beta1, ConvertClusterResourceSetV1Beta1ToHub),
)

// ConvertClusterResourceSetV1Beta1ToHub converts a v1beta1 ClusterResourceSet to a hub ClusterResourceSet.
func ConvertClusterResourceSetV1Beta1ToHub(_ context.Context, src *addonsv1beta1.ClusterResourceSet, dst *addonsv1.ClusterResourceSet) error {
	return addonsv1beta1.Convert_v1beta1_ClusterResourceSet_To_v1beta2_ClusterResourceSet(src, dst, nil)
}

// ConvertClusterResourceSetHubToV1Beta1 converts a hub ClusterResourceSet to a v1beta1 ClusterResourceSet.
func ConvertClusterResourceSetHubToV1Beta1(_ context.Context, src *addonsv1.ClusterResourceSet, dst *addonsv1beta1.ClusterResourceSet) error {
	return addonsv1beta1.Convert_v1beta2_ClusterResourceSet_To_v1beta1_ClusterResourceSet(src, dst, nil)
}
