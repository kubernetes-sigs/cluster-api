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

package api

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// InfraMachine defines the contract for the InfraMachine.
type InfraMachine interface {
	client.Object

	GetStatusWithConditions() (any, error)

	GetProviderID() string

	GetProvisioned() bool

	GetInterruptible() bool

	GetAddresses() []clusterv1.MachineAddress

	// Note: Will be removed when v1beta1 is removed
	GetSpecFailureDomain() string

	GetFailureDomain() string

	GetFailureReason() string

	GetFailureMessage() string
}
