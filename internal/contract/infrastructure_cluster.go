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
	"encoding/json"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// InfrastructureClusterContract encodes information about the Cluster API contract for InfrastructureCluster objects
// like DockerClusters, AWS Clusters, etc.
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

// Ready provides access to the status.ready field in an InfrastructureCluster object.
func (c *InfrastructureClusterContract) Ready() *Bool {
	return &Bool{
		path: []string{"status", "ready"},
	}
}

// FailureReason provides access to the status.failureReason field in an InfrastructureCluster object. Note that this field is optional.
func (c *InfrastructureClusterContract) FailureReason() *String {
	return &String{
		path: []string{"status", "failureReason"},
	}
}

// FailureMessage provides access to the status.failureMessage field in an InfrastructureCluster object. Note that this field is optional.
func (c *InfrastructureClusterContract) FailureMessage() *String {
	return &String{
		path: []string{"status", "failureMessage"},
	}
}

// FailureDomains provides access to the status.failureDomains field in an InfrastructureCluster object. Note that this field is optional.
func (c *InfrastructureClusterContract) FailureDomains() *FailureDomains {
	return &FailureDomains{
		path: []string{"status", "failureDomains"},
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

// FailureDomains represents an accessor to a clusterv1.FailureDomains path value.
type FailureDomains struct {
	path Path
}

// Path returns the path to the clusterv1.FailureDomains value.
func (d *FailureDomains) Path() Path {
	return d.path
}

// Get gets the metav1.MachineAddressList value.
func (d *FailureDomains) Get(obj *unstructured.Unstructured) (*clusterv1.FailureDomains, error) {
	domainMap, ok, err := unstructured.NestedMap(obj.UnstructuredContent(), d.path...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(d.path, "."))
	}
	if !ok {
		return nil, errors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(d.path, "."))
	}

	domains := make(clusterv1.FailureDomains, len(domainMap))
	s, err := json.Marshal(domainMap)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshall field at %s to json", "."+strings.Join(d.path, "."))
	}
	err = json.Unmarshal(s, &domains)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshall field at %s to json", "."+strings.Join(d.path, "."))
	}

	return &domains, nil
}

// Set sets the clusterv1.FailureDomains value in the path.
func (d *FailureDomains) Set(obj *unstructured.Unstructured, values clusterv1.FailureDomains) error {
	domains := make(map[string]interface{}, len(values))
	s, err := json.Marshal(values)
	if err != nil {
		return errors.Wrapf(err, "failed to marshall supplied values to json for path %s", "."+strings.Join(d.path, "."))
	}
	err = json.Unmarshal(s, &domains)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshall supplied values to json for path %s", "."+strings.Join(d.path, "."))
	}

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), domains, d.path...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(d.path, "."), obj.GroupVersionKind())
	}
	return nil
}
