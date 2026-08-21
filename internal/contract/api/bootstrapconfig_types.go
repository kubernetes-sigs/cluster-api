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

// Package api contains the API that is agnostic to the specific contract version.
package api

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BootstrapConfig defines the contract for the BootstrapConfig.
type BootstrapConfig interface {
	client.Object

	// GetStatusWithReadyConditions returns the status of the BootstrapConfig with its Ready conditions.
	GetStatusWithReadyConditions() any

	// GetDataSecretCreated returns whether the bootstrap data secret has been created.
	GetDataSecretCreated() bool

	// GetDataSecretName returns the name of the bootstrap data secret.
	GetDataSecretName() string

	// GetFailureReason returns the failure reason.
	GetFailureReason() string

	// GetFailureMessage returns the failure message.
	GetFailureMessage() string
}
