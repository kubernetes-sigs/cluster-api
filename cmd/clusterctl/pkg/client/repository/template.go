/*
Copyright 2019 The Kubernetes Authors.

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

package repository

import (
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

// Template wraps a YAML file that defines the cluster objects (Cluster, Machines etc.).
// It is important to notice that clusterctl applies a set of processing steps to the “raw” cluster template YAML read
// from the provider repositories:
// 1. Checks for all the variables in the cluster template YAML file and replace with corresponding config values
// 2. Process go templates contained in the cluster template YAML file
// 3. Ensure all the cluster objects are deployed in the target namespace
type Template interface {
	// configuration of the provider the template belongs to.
	config.Provider

	// Version of the provider the template belongs to.
	Version() string

	// Flavor implemented by the template (empty means default flavor).
	// A flavor is a variant of cluster template supported by the provider, like e.g. Prod, Test.
	Flavor() string

	// Bootstrap provider used by the cluster template.
	Bootstrap() string

	// Variables required by the template.
	// This value is derived by the template YAML.
	Variables() []string

	// TargetNamespace where the template objects will be installed.
	// This value is inherited by the template options.
	TargetNamespace() string

	// Yaml file defining all the cluster objects.
	Yaml() []byte
}
