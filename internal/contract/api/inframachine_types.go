/*
Copyright The Kubernetes Authors.

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

package api

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// InfraMachine defines the contract for the InfraMachine.
type InfraMachine interface {
	client.Object

	// GetStatusWithReadyConditions returns the status of the BootstrapConfig with its Ready conditions.
	GetStatusWithReadyConditions() any

	// GetProviderID returns the provider ID.
	GetProviderID() string

	// GetProvisioned returns whether the infrastructure is provisioned.
	GetProvisioned() bool

	// GetInterruptible returns whether the infrastructure is interruptible.
	GetInterruptible() bool

	// GetAddresses returns the machine addresses.
	GetAddresses() []clusterv1.MachineAddress

	// GetSpecFailureDomain returns the failure domain requested in spec.
	GetSpecFailureDomain() string

	// GetFailureDomain returns the actual failure domain from status.
	GetFailureDomain() string

	// GetFailureReason returns the failure reason.
	GetFailureReason() string

	// GetFailureMessage returns the failure message.
	GetFailureMessage() string
}
