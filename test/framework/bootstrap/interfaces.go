/*
Copyright 2020 The Kubernetes Authors.

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

// Package bootstrap implements bootstrap functionality for e2e testing.
package bootstrap

import "context"

// ClusterProvider defines the behavior of a type that is responsible for provisioning and managing a Kubernetes cluster.
type ClusterProvider interface {
	// Create a Kubernetes cluster.
	// Generally to be used in the BeforeSuite function to create a Kubernetes cluster to be shared between tests.
	Create(context.Context)

	// GetKubeconfigPath returns the path to the kubeconfig file to be used to access the Kubernetes cluster.
	GetKubeconfigPath() string

	// Dispose will completely clean up the provisioned cluster.
	// This should be implemented as a synchronous function.
	// Generally to be used in the AfterSuite function if a Kubernetes cluster is shared between tests.
	// Should try to clean everything up and report any dangling artifacts that needs manual intervention.
	Dispose(context.Context)
}
