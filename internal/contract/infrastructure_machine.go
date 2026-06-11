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
	"sync"
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

// Provisioned returns if the InfrastructureMachine is provisioned.
func (m *InfrastructureMachineContract) Provisioned(contractVersion string) *Bool {
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
func (m *InfrastructureMachineContract) ReadyConditionType() string {
	return "Ready"
}

// ProviderID provides access to the spec.providerID field in an InfrastructureMachine object.
func (m *InfrastructureMachineContract) ProviderID() *String {
	return &String{
		path: []string{"spec", "providerID"},
	}
}
