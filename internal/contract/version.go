/*
Copyright 2025 The Kubernetes Authors.

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

package contract

import (
	"k8s.io/apimachinery/pkg/util/sets"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

var (
	// Version is the contract version supported by this Cluster API version.
	// Note: Each Cluster API version supports one contract version, and by convention the contract version matches the current API version.
	Version = clusterv1.GroupVersion.Version
)

// GetCompatibleVersions return the list of contract version compatible with a given contract version.
// NOTE: A contract version might be temporarily compatible with older contract versions e.g. to allow users time to transition to the new API.
// NOTE: The return value must include also the contract version received in input.
func GetCompatibleVersions(contract string) sets.Set[string] {
	compatibleContracts := sets.New(contract)
	// v1beta2 contract is temporarily be compatible with v1beta1 (until v1beta1 is EOL).
	if contract == "v1beta2" {
		compatibleContracts.Insert("v1beta1")
	}
	return compatibleContracts
}
