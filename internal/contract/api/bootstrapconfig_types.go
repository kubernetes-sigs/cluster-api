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

	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

// BootstrapConfig defines the fields of an object implementing the BootstrapConfig contract.
// Note: This interface contains the fields defined by the current contract version, but it allows
// concrete implementations to handle the compatibility with older contract versions.
type BootstrapConfig interface {
	client.Object

	// GetDataSecretCreated returns whether the bootstrap data secret has been created.
	GetDataSecretCreated() bool

	// GetDataSecretName returns the name of the bootstrap data secret.
	GetDataSecretName() string

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
