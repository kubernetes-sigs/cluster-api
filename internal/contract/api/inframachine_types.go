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
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

// InfraMachine defines the fields of an object implementing the InfraMachine contract.
// Note: This interface contains the fields defined by the current contract version, but it allows
// concrete implementations to handle the compatibility with older contract versions.
type InfraMachine interface {
	client.Object

	// GetProviderID returns the provider ID.
	GetProviderID() string

	// GetProvisioned returns whether the infrastructure is provisioned.
	GetProvisioned() bool

	// GetInterruptible returns whether the infrastructure is interruptible.
	GetInterruptible() bool

	// GetAddresses returns the machine addresses.
	GetAddresses() []clusterv1.MachineAddress

	// GetSpecFailureDomain returns the failure domain requested in spec.
	//
	// Deprecated: This method is deprecated and is going to be removed when support for v1beta1 will be dropped.
	GetSpecFailureDomain() string

	// GetFailureDomain returns the actual failure domain from status.
	GetFailureDomain() string

	// GetFailureReason returns the failure reason.
	//
	// Deprecated: This method is deprecated and is going to be removed when support for v1beta1 will be dropped.
	GetFailureReason() string

	// GetFailureMessage returns the failure message.
	//
	// Deprecated: This method is deprecated and is going to be removed when support for v1beta1 will be dropped.
	GetFailureMessage() string

	v1beta1conditions.Getter

	conditions.Getter
}
