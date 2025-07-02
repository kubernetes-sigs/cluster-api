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
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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
func (c *InfrastructureClusterContract) ControlPlaneEndpoint() *ControlPlaneEndpoint {
	return &ControlPlaneEndpoint{
		path: []string{"spec", "controlPlaneEndpoint"},
	}
}

// Provisioned returns if the infrastructure cluster has been provisioned.
func (c *InfrastructureClusterContract) Provisioned(contractVersion string) *Bool {
	if contractVersion == "v1beta1" {
		return &Bool{
			path: []string{"status", "ready"},
		}
	}

	return &Bool{
		path: []string{"status", "initialization", "provisioned"},
	}
}

// ReadyConditionType returns the type of the ready condition.
func (c *InfrastructureClusterContract) ReadyConditionType() string {
	return "Ready"
}

// FailureReason provides access to the status.failureReason field in an InfrastructureCluster object. Note that this field is optional.
//
// Deprecated: This function is deprecated and is going to be removed. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
func (c *InfrastructureClusterContract) FailureReason() *String {
	return &String{
		path: []string{"status", "failureReason"},
	}
}

// FailureMessage provides access to the status.failureMessage field in an InfrastructureCluster object. Note that this field is optional.
//
// Deprecated: This function is deprecated and is going to be removed. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
func (c *InfrastructureClusterContract) FailureMessage() *String {
	return &String{
		path: []string{"status", "failureMessage"},
	}
}

// FailureDomains provides access to the status.failureDomains field in an InfrastructureCluster object. Note that this field is optional.
func (c *InfrastructureClusterContract) FailureDomains(contractVersion string) *FailureDomains {
	return &FailureDomains{
		contractVersion: contractVersion,
		path:            []string{"status", "failureDomains"},
	}
}

// IgnorePaths returns a list of paths to be ignored when reconciling an InfrastructureCluster.
// NOTE: The controlPlaneEndpoint struct currently contains two mandatory fields (host and port).
// As the host and port fields are not using omitempty, they are automatically set to their zero values
// if they are not set by the user. We don't want to reconcile the zero values as we would then overwrite
// changes applied by the infrastructure provider controller.
func (c *InfrastructureClusterContract) IgnorePaths(infrastructureCluster *unstructured.Unstructured) ([]Path, error) {
	var ignorePaths []Path

	host, ok, err := unstructured.NestedString(infrastructureCluster.UnstructuredContent(), InfrastructureCluster().ControlPlaneEndpoint().host().Path()...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s", InfrastructureCluster().ControlPlaneEndpoint().host().Path().String())
	}
	if ok && host == "" {
		ignorePaths = append(ignorePaths, InfrastructureCluster().ControlPlaneEndpoint().host().Path())
	}

	port, ok, err := unstructured.NestedInt64(infrastructureCluster.UnstructuredContent(), InfrastructureCluster().ControlPlaneEndpoint().port().Path()...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s", InfrastructureCluster().ControlPlaneEndpoint().port().Path().String())
	}
	if ok && port == 0 {
		ignorePaths = append(ignorePaths, InfrastructureCluster().ControlPlaneEndpoint().port().Path())
	}

	return ignorePaths, nil
}

// FailureDomains represents an accessor to a clusterv1.FailureDomains path value.
type FailureDomains struct {
	contractVersion string
	path            Path
}

// Path returns the path to the clusterv1.FailureDomains value.
func (d *FailureDomains) Path() Path {
	return d.path
}

// Get gets the metav1.MachineAddressList value.
func (d *FailureDomains) Get(obj *unstructured.Unstructured) ([]clusterv1.FailureDomain, error) {
	if d.contractVersion == "v1beta1" {
		domainMap, ok, err := unstructured.NestedMap(obj.UnstructuredContent(), d.path...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(d.path, "."))
		}
		if !ok {
			return nil, errors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(d.path, "."))
		}

		domains := make(clusterv1beta1.FailureDomains, len(domainMap))
		s, err := json.Marshal(domainMap)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal field at %s to json", "."+strings.Join(d.path, "."))
		}
		if err := json.Unmarshal(s, &domains); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal field at %s to json", "."+strings.Join(d.path, "."))
		}

		// Sort the failureDomains to ensure deterministic order.
		// Without this we would end up with infinite reconciles when writing `Cluster.status.failureDomains`
		// after retrieving the failureDomains from an InfraCluster.
		domainNames := slices.Collect(maps.Keys(domains))
		sort.Strings(domainNames)
		var domainsArray []clusterv1.FailureDomain
		for _, name := range domainNames {
			domain := domains[name]
			domainsArray = append(domainsArray, clusterv1.FailureDomain{
				Name:         name,
				ControlPlane: ptr.To(domain.ControlPlane),
				Attributes:   domain.Attributes,
			})
		}

		return domainsArray, nil
	}

	domainArray, ok, err := unstructured.NestedSlice(obj.UnstructuredContent(), d.path...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(d.path, "."))
	}
	if !ok {
		return nil, errors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(d.path, "."))
	}

	domains := make([]clusterv1.FailureDomain, len(domainArray))
	s, err := json.Marshal(domainArray)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal field at %s to json", "."+strings.Join(d.path, "."))
	}
	if err := json.Unmarshal(s, &domains); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal field at %s to json", "."+strings.Join(d.path, "."))
	}

	for i, domain := range domains {
		if domain.ControlPlane == nil {
			domain.ControlPlane = ptr.To(false)
			domains[i] = domain
		}
	}

	// Sort the failureDomains to ensure deterministic order even if the failureDomains field
	// on the InfraCluster is not sorted.
	slices.SortFunc(domains, func(a, b clusterv1.FailureDomain) int {
		if a.Name < b.Name {
			return -1
		}
		return 1
	})

	return domains, nil
}

// Set sets the []clusterv1.FailureDomain value in the path.
func (d *FailureDomains) Set(obj *unstructured.Unstructured, values []clusterv1.FailureDomain) error {
	domains := make([]interface{}, len(values))
	s, err := json.Marshal(values)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal supplied values to json for path %s", "."+strings.Join(d.path, "."))
	}
	if err := json.Unmarshal(s, &domains); err != nil {
		return errors.Wrapf(err, "failed to unmarshal supplied values to json for path %s", "."+strings.Join(d.path, "."))
	}

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), domains, d.path...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(d.path, "."), obj.GroupVersionKind())
	}
	return nil
}

// ControlPlaneEndpoint provides a helper struct for working with ControlPlaneEndpoint
// in an InfrastructureCluster object.
type ControlPlaneEndpoint struct {
	path Path
}

// Path returns the path to the ControlPlaneEndpoint in an InfrastructureCluster object.
func (c *ControlPlaneEndpoint) Path() Path {
	return c.path
}

// Get gets the ControlPlaneEndpoint value.
func (c *ControlPlaneEndpoint) Get(obj *unstructured.Unstructured) (*clusterv1.APIEndpoint, error) {
	controlPlaneEndpointMap, ok, err := unstructured.NestedMap(obj.UnstructuredContent(), c.path...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(c.path, "."))
	}
	if !ok {
		return nil, errors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(c.path, "."))
	}

	endpoint := &clusterv1.APIEndpoint{}
	s, err := json.Marshal(controlPlaneEndpointMap)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal field at %s to json", "."+strings.Join(c.path, "."))
	}
	if err := json.Unmarshal(s, &endpoint); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal field at %s to json", "."+strings.Join(c.path, "."))
	}

	return endpoint, nil
}

// Set sets the ControlPlaneEndpoint value.
func (c *ControlPlaneEndpoint) Set(obj *unstructured.Unstructured, value clusterv1.APIEndpoint) error {
	controlPlaneEndpointMap := make(map[string]interface{})
	s, err := json.Marshal(value)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal supplied values to json for path %s", "."+strings.Join(c.path, "."))
	}
	if err := json.Unmarshal(s, &controlPlaneEndpointMap); err != nil {
		return errors.Wrapf(err, "failed to unmarshal supplied values to json for path %s", "."+strings.Join(c.path, "."))
	}

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), controlPlaneEndpointMap, c.path...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(c.path, "."), obj.GroupVersionKind())
	}
	return nil
}

// host provides access to the host field in the ControlPlaneEndpoint in an InfrastructureCluster object.
func (c *ControlPlaneEndpoint) host() *String {
	return &String{
		path: c.path.Append("host"),
	}
}

// port provides access to the port field in the ControlPlaneEndpoint in an InfrastructureCluster object.
func (c *ControlPlaneEndpoint) port() *Int64 {
	return &Int64{
		path: c.path.Append("port"),
	}
}
