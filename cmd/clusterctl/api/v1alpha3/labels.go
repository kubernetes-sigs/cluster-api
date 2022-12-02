/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha3

import "fmt"

const (
	// ClusterctlLabel is applied to all components managed by clusterctl.
	ClusterctlLabel = "clusterctl.cluster.x-k8s.io"

	// ClusterctlCoreLabel is applied to all the core objects managed by clusterctl.
	ClusterctlCoreLabel = "clusterctl.cluster.x-k8s.io/core"

	// ClusterctlCoreLabelInventoryValue define the value for ClusterctlCoreLabel to be used for inventory objects.
	ClusterctlCoreLabelInventoryValue = "inventory"

	// ClusterctlCoreLabelCertManagerValue define the value for ClusterctlCoreLabel to be used for cert-manager objects.
	ClusterctlCoreLabelCertManagerValue = "cert-manager"

	// ClusterctlMoveLabel can be set on CRDs that providers wish to move but that are not part of a Cluster.
	ClusterctlMoveLabel = "clusterctl.cluster.x-k8s.io/move"

	// ClusterctlMoveHierarchyLabel can be set on CRDs that providers wish to move with their entire hierarchy, but that are not part of a Cluster.
	ClusterctlMoveHierarchyLabel = "clusterctl.cluster.x-k8s.io/move-hierarchy"
)

// ManifestLabel returns the cluster.x-k8s.io/provider label value for a provider/type.
//
// Note: the label uniquely describes the provider type and its kind (e.g. bootstrap-kubeadm);
// it's not meant to be used to describe each instance of a particular provider.
func ManifestLabel(name string, providerType ProviderType) string {
	switch providerType {
	case BootstrapProviderType:
		return fmt.Sprintf("bootstrap-%s", name)
	case ControlPlaneProviderType:
		return fmt.Sprintf("control-plane-%s", name)
	case InfrastructureProviderType:
		return fmt.Sprintf("infrastructure-%s", name)
	case IPAMProviderType:
		return fmt.Sprintf("ipam-%s", name)
	case RuntimeExtensionProviderType:
		return fmt.Sprintf("runtime-extension-%s", name)
	default:
		return name
	}
}
