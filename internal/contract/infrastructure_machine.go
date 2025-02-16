/*
Copyright 2022 The Kubernetes Authors.

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

// InfrastructureMachineContract encodes information about the Cluster API contract for InfrastructureMachine objects
// like DockerMachines, AWS Machines, etc.
type InfrastructureMachineContract struct{}

var infrastructureMachine *InfrastructureMachineContract
var onceInfrastructureMachine sync.Once

// InfrastructureMachine provide access to the information about the Cluster API contract for InfrastructureMachine objects.
func InfrastructureMachine() *InfrastructureMachineContract {
	onceInfrastructureMachine.Do(func() {
		infrastructureMachine = &InfrastructureMachineContract{}
	})
	return infrastructureMachine
}

// Ready provides access to status.ready field in an InfrastructureMachine object.
func (m *InfrastructureMachineContract) Ready() *Bool {
	return &Bool{
		path: []string{"status", "ready"},
	}
}

// ReadyConditionType returns the type of the ready condition.
func (m *InfrastructureMachineContract) ReadyConditionType() string {
	return "Ready"
}

// FailureReason provides access to the status.failureReason field in an InfrastructureMachine object. Note that this field is optional.
//
// Deprecated: This function is deprecated and is going to be removed. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
func (m *InfrastructureMachineContract) FailureReason() *String {
	return &String{
		path: []string{"status", "failureReason"},
	}
}

// FailureMessage provides access to the status.failureMessage field in an InfrastructureMachine object. Note that this field is optional.
//
// Deprecated: This function is deprecated and is going to be removed. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
func (m *InfrastructureMachineContract) FailureMessage() *String {
	return &String{
		path: []string{"status", "failureMessage"},
	}
}

// Addresses provides access to the status.addresses field in an InfrastructureMachine object. Note that this field is optional.
func (m *InfrastructureMachineContract) Addresses() *MachineAddresses {
	return &MachineAddresses{
		path: []string{"status", "addresses"},
	}
}

// ProviderID provides access to the spec.providerID field in an InfrastructureMachine object.
func (m *InfrastructureMachineContract) ProviderID() *String {
	return &String{
		path: []string{"spec", "providerID"},
	}
}

// FailureDomain provides access to the spec.failureDomain field in an InfrastructureMachine object. Note that this field is optional.
func (m *InfrastructureMachineContract) FailureDomain() *String {
	return &String{
		path: []string{"spec", "failureDomain"},
	}
}

// MachineAddresses represents an accessor to a []clusterv1.MachineAddress path value.
type MachineAddresses struct {
	path Path
}

// Path returns the path to the []clusterv1.MachineAddress value.
func (m *MachineAddresses) Path() Path {
	return m.path
}

// Get gets the metav1.MachineAddressList value.
func (m *MachineAddresses) Get(obj *unstructured.Unstructured) (*[]clusterv1.MachineAddress, error) {
	slice, ok, err := unstructured.NestedSlice(obj.UnstructuredContent(), m.path...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(m.path, "."))
	}
	if !ok {
		return nil, errors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(m.path, "."))
	}

	addresses := make([]clusterv1.MachineAddress, len(slice))
	s, err := json.Marshal(slice)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshall field at %s to json", "."+strings.Join(m.path, "."))
	}
	err = json.Unmarshal(s, &addresses)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshall field at %s to json", "."+strings.Join(m.path, "."))
	}

	return &addresses, nil
}

// Set sets the []clusterv1.MachineAddress value in the path.
func (m *MachineAddresses) Set(obj *unstructured.Unstructured, values []clusterv1.MachineAddress) error {
	slice := make([]interface{}, len(values))
	s, err := json.Marshal(values)
	if err != nil {
		return errors.Wrapf(err, "failed to marshall supplied values to json for path %s", "."+strings.Join(m.path, "."))
	}
	err = json.Unmarshal(s, &slice)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshall supplied values to json for path %s", "."+strings.Join(m.path, "."))
	}

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), slice, m.path...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(m.path, "."), obj.GroupVersionKind())
	}
	return nil
}
