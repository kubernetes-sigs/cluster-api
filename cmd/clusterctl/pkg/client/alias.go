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

package client

import (
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
)

// Alias creates local aliases for types defined in the low-level libraries.
// By using a local alias, we ensure that users import and use clusterctl's high-level library.

// Provider defines a provider configuration.
type Provider config.Provider

// Components wraps a YAML file that defines the provider's components (CRD, controller, RBAC rules etc.)
type Components repository.Components
