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

// Package upstream contains types to handle additional data during Marshal or UnMarshal of kubeadm's types.
//
// In this context "Additional data" refers to data existing in the kubeadm's types, but not included in the
// corresponding Cluster API types, or migrated from one struct to another in one of the older versions
// of the kubeadm's types.
package upstream

// AdditionalDataSetter defines capabilities of a type to set additional data.
type AdditionalDataSetter interface {
	SetAdditionalData(data AdditionalData)
}

// AdditionalDataGetter defines capabilities of a type to get additional data.
type AdditionalDataGetter interface {
	GetAdditionalData(*AdditionalData)
}

// AdditionalData is additional data that must go in kubeadm's ClusterConfiguration, but exists
// in different Cluster API objects, like e.g. the Cluster object.
type AdditionalData struct {
	// Data from Cluster API's Cluster object.
	KubernetesVersion *string
	ClusterName       *string
	DNSDomain         *string
	ServiceSubnet     *string
	PodSubnet         *string

	// Data migrated from ClusterConfiguration to InitConfiguration in kubeadm's v1beta4 API version.
	// Note: Corresponding Cluster API types are aligned with kubeadm's v1beta4 API version.
	ControlPlaneComponentHealthCheckSeconds *int32
}

// Clone returns a clone of AdditionalData.
func (a *AdditionalData) Clone() *AdditionalData {
	return &AdditionalData{
		KubernetesVersion:                       a.KubernetesVersion,
		ClusterName:                             a.ClusterName,
		DNSDomain:                               a.DNSDomain,
		ServiceSubnet:                           a.ServiceSubnet,
		PodSubnet:                               a.PodSubnet,
		ControlPlaneComponentHealthCheckSeconds: a.ControlPlaneComponentHealthCheckSeconds,
	}
}
