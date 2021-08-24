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
	"strings"
	"sync"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ControlPlaneContract encodes information about the Cluster API contract for ControlPlane objects
// like e.g the KubeadmControlPlane etc.
type ControlPlaneContract struct{}

var controlPlane *ControlPlaneContract
var onceControlPlane sync.Once

// ControlPlane provide access to the information about the Cluster API contract for ControlPlane objects.
func ControlPlane() *ControlPlaneContract {
	onceControlPlane.Do(func() {
		controlPlane = &ControlPlaneContract{}
	})
	return controlPlane
}

// InfrastructureMachineTemplate provide access to InfrastructureMachineTemplate reference in a ControlPlane object, if any.
// NOTE: When working with unstructured there is no way to understand if the ControlPlane provider
// do support a field in the type definition from the fact that a field is not set in a given instance.
// This is why in we are deriving if InfrastructureMachineTemplate is required from the ClusterClass in the topology reconciler code.
func (c *ControlPlaneContract) InfrastructureMachineTemplate() *Ref {
	return &Ref{
		path: []string{"spec", "machineTemplate", "infrastructureRef"},
	}
}

// Version provide access to version field  in a ControlPlane object, if any.
// NOTE: When working with unstructured there is no way to understand if the ControlPlane provider
// do support a field in the type definition from the fact that a field is not set in a given instance.
// This is why in we are deriving if version is required from the ClusterClass in the topology reconciler code.
func (c *ControlPlaneContract) Version() *ControlPlaneVersion {
	return &ControlPlaneVersion{}
}

// Replicas provide access to replicas field  in a ControlPlane object, if any.
// NOTE: When working with unstructured there is no way to understand if the ControlPlane provider
// do support a field in the type definition from the fact that a field is not set in a given instance.
// This is why in we are deriving if replicas is required from the ClusterClass in the topology reconciler code.
func (c *ControlPlaneContract) Replicas() *ControlPlaneReplicas {
	return &ControlPlaneReplicas{}
}

// ControlPlaneVersion provide a helper struct for working with version in ClusterClass.
type ControlPlaneVersion struct{}

// Path returns the path of the reference.
func (v *ControlPlaneVersion) Path() Path {
	return Path{"spec", "version"}
}

// Get gets the version value from the ControlPlane object.
func (v *ControlPlaneVersion) Get(obj *unstructured.Unstructured) (*string, error) {
	value, ok, err := unstructured.NestedString(obj.UnstructuredContent(), v.Path()...)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.Errorf("%s not found", "."+strings.Join(v.Path(), "."))
	}
	return &value, nil
}

// Set sets the version value in the ControlPlane object.
func (v *ControlPlaneVersion) Set(obj *unstructured.Unstructured, value string) error {
	return unstructured.SetNestedField(obj.UnstructuredContent(), value, v.Path()...)
}

// ControlPlaneReplicas provide a helper struct for working with version in ClusterClass.
type ControlPlaneReplicas struct{}

// Path returns the path of the reference.
func (r *ControlPlaneReplicas) Path() Path {
	return Path{"spec", "replicas"}
}

// Get gets the replicas value from the ControlPlane object.
func (r *ControlPlaneReplicas) Get(obj *unstructured.Unstructured) (*int64, error) {
	value, ok, err := unstructured.NestedInt64(obj.UnstructuredContent(), r.Path()...)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.Errorf("%s not found", "."+strings.Join(r.Path(), "."))
	}
	return &value, nil
}

// Set sets the replica value in the ControlPlane object.
func (r *ControlPlaneReplicas) Set(obj *unstructured.Unstructured, value int64) error {
	return unstructured.SetNestedField(obj.UnstructuredContent(), value, r.Path()...)
}
