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

// Package upstream contains types to handle additional data that only exists in upstream types.
package upstream

// DataSetter defines capabilities of a type to set additional data that only
// exists in the upstream types.
type DataSetter interface {
	SetUpstreamData(data Data)
}

// DataGetter defines capabilities of a type to get additional data that only
// exists in the upstream types.
type DataGetter interface {
	GetUpstreamData() Data
}

// Data is additional data that only exists in the upstream types.
type Data struct {
	KubernetesVersion    *string
	ClusterName          *string
	ControlPlaneEndpoint *string
	DNSDomain            *string
	ServiceSubnet        *string
	PodSubnet            *string
}
