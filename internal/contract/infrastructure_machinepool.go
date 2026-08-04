/*
Copyright 2026 The Kubernetes Authors.

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

	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// InfrastructureMachinePoolContract encodes information about the Cluster API contract for InfrastructureMachinePool objects
// like DockerMachinePools, AWSMachinePools, etc.
type InfrastructureMachinePoolContract struct{}

var infrastructureMachinePool *InfrastructureMachinePoolContract
var onceInfrastructureMachinePool sync.Once

// InfrastructureMachinePool provide access to the information about the Cluster API contract for InfrastructureMachinePool objects.
func InfrastructureMachinePool() *InfrastructureMachinePoolContract {
	onceInfrastructureMachinePool.Do(func() {
		infrastructureMachinePool = &InfrastructureMachinePoolContract{}
	})
	return infrastructureMachinePool
}

// Provisioned returns if the InfrastructureMachinePool is provisioned.
// Note: When using the v1beta2 version of the contract, reading status.initialization.provisioned falls back to
// status.ready if the field is not set, thus preserving temporary compatibility with providers that have
// not yet adopted the new field.
func (m *InfrastructureMachinePoolContract) Provisioned(contractVersion string) *ProvisionedBool {
	if contractVersion == "v1beta1" {
		return &ProvisionedBool{
			path: []string{"status", "ready"},
		}
	}

	return &ProvisionedBool{
		path:         []string{"status", "initialization", "provisioned"},
		fallbackPath: []string{"status", "ready"},
	}
}

// ProvisionedBool represents an accessor to the provisioned status of an InfrastructureMachinePool, with an
// optional fallback path that is read when the field at the primary path is not set. This is needed as the
// MachinePool contracts are lagging behind core CAPI. So a provider will declare they are compliant with
// CAPI v1beta2 but may not have fully adopted the v1beta2 contract for MachinePools. So this allows us to
// fallback to the previous contract value if the new fields doesn't exist.
type ProvisionedBool struct {
	path         Path
	fallbackPath Path
}

// Path returns the primary path to the bool value.
func (b *ProvisionedBool) Path() Path {
	return b.path
}

// Get gets the bool value from the primary path; if the field is not set and a fallback path is defined,
// the value is read from the fallback path instead.
func (b *ProvisionedBool) Get(obj *unstructured.Unstructured) (*bool, error) {
	value, ok, err := unstructured.NestedBool(obj.UnstructuredContent(), b.path...)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to get %s from object", "."+strings.Join(b.path, "."))
	}
	if !ok && len(b.fallbackPath) > 0 {
		value, ok, err = unstructured.NestedBool(obj.UnstructuredContent(), b.fallbackPath...)
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to get %s from object", "."+strings.Join(b.fallbackPath, "."))
		}
	}
	if !ok {
		return nil, pkgerrors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(b.path, "."))
	}
	return &value, nil
}

// Set sets the bool value in the primary path.
func (b *ProvisionedBool) Set(obj *unstructured.Unstructured, value bool) error {
	if err := unstructured.SetNestedField(obj.UnstructuredContent(), value, b.path...); err != nil {
		return pkgerrors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(b.path, "."), obj.GroupVersionKind())
	}
	return nil
}

// ProviderIDList provides access to the spec.providerIDList field in an InfrastructureMachinePool object.
func (m *InfrastructureMachinePoolContract) ProviderIDList() *StringSlice {
	return &StringSlice{
		path: []string{"spec", "providerIDList"},
	}
}

// Replicas provides access to the status.replicas field in an InfrastructureMachinePool object.
func (m *InfrastructureMachinePoolContract) Replicas() *Int32 {
	return &Int32{
		path: []string{"status", "replicas"},
	}
}

// InfrastructureMachineKind provides access to the status.infrastructureMachineKind field in an InfrastructureMachinePool object.
// Note that this field is optional and it is set only if the InfrastructureMachinePool supports MachinePool Machines.
func (m *InfrastructureMachinePoolContract) InfrastructureMachineKind() *String {
	return &String{
		path: []string{"status", "infrastructureMachineKind"},
	}
}
