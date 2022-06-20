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

import (
	"sync"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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

// ControlPlaneEndpoint provides access to ControlPlaneEndpoint in an InfrastructureCluster object.
func (c *InfrastructureClusterContract) ControlPlaneEndpoint() *InfrastructureClusterControlPlaneEndpoint {
	return &InfrastructureClusterControlPlaneEndpoint{}
}

// InfrastructureClusterControlPlaneEndpoint provides a helper struct for working with ControlPlaneEndpoint
// in an InfrastructureCluster object.
type InfrastructureClusterControlPlaneEndpoint struct{}

// Host provides access to the host field in the ControlPlaneEndpoint in an InfrastructureCluster object.
func (c *InfrastructureClusterControlPlaneEndpoint) Host() *String {
	return &String{
		path: []string{"spec", "controlPlaneEndpoint", "host"},
	}
}

// Port provides access to the port field in the ControlPlaneEndpoint in an InfrastructureCluster object.
func (c *InfrastructureClusterControlPlaneEndpoint) Port() *Int64 {
	return &Int64{
		path: []string{"spec", "controlPlaneEndpoint", "port"},
	}
}

// IgnorePaths returns a list of paths to be ignored when reconciling an InfrastructureCluster.
// NOTE: The controlPlaneEndpoint struct currently contains two mandatory fields (host and port).
// As the host and port fields are not using omitempty, they are automatically set to their zero values
// if they are not set by the user. We don't want to reconcile the zero values as we would then overwrite
// changes applied by the infrastructure provider controller.
func (c *InfrastructureClusterContract) IgnorePaths(infrastructureCluster *unstructured.Unstructured) ([]Path, error) {
	var ignorePaths []Path

	host, ok, err := unstructured.NestedString(infrastructureCluster.UnstructuredContent(), InfrastructureCluster().ControlPlaneEndpoint().Host().Path()...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s", InfrastructureCluster().ControlPlaneEndpoint().Host().Path().String())
	}
	if ok && host == "" {
		ignorePaths = append(ignorePaths, InfrastructureCluster().ControlPlaneEndpoint().Host().Path())
	}

	port, ok, err := unstructured.NestedInt64(infrastructureCluster.UnstructuredContent(), InfrastructureCluster().ControlPlaneEndpoint().Port().Path()...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s", InfrastructureCluster().ControlPlaneEndpoint().Port().Path().String())
	}
	if ok && port == 0 {
		ignorePaths = append(ignorePaths, InfrastructureCluster().ControlPlaneEndpoint().Port().Path())
	}

	return ignorePaths, nil
}
