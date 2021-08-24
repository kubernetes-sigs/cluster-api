/*
Copyright 2021 The Kubernetes Authors.

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

import "sync"

// InfrastructureClusterContract encodes information about the Cluster API contract for InfrastructureCluster objects
// like e.g the DockerCluster, AWS Cluster etc.
type InfrastructureClusterContract struct{}

var infrastructureCluster *InfrastructureClusterContract
var onceInfrastructureCluster sync.Once

// InfrastructureCluster provide access to the information about the Cluster API contract for InfrastructureCluster objects.
func InfrastructureCluster() *InfrastructureClusterContract {
	onceInfrastructureCluster.Do(func() {
		infrastructureCluster = &InfrastructureClusterContract{}
	})
	return infrastructureCluster
}

// IgnorePaths returns a list of paths to be ignored when reconciling a topology.
func (c *InfrastructureClusterContract) IgnorePaths() []Path {
	return []Path{
		// NOTE: the controlPlaneEndpoint struct currently contains two mandatory fields (host and port); without this
		// ignore path they are going to be always reconciled to the default value or to the value set into the template.
		{"spec", "controlPlaneEndpoint"},
	}
}
