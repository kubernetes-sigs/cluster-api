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
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
)

// Alias creates local aliases for types defined in the low-level libraries.
// By using a local alias, we ensure that users import and use clusterctl's high-level library.

// Provider defines a provider configuration.
type Provider config.Provider

// Components wraps a YAML file that defines the provider's components (CRDs, controller, RBAC rules etc.).
type Components repository.Components

// ComponentsOptions wraps inputs to get provider's components.
type ComponentsOptions repository.ComponentsOptions

// Template wraps a YAML file that defines the cluster objects (Cluster, Machines etc.).
type Template repository.Template

// UpgradePlan defines a list of possible upgrade targets for a management cluster.
type UpgradePlan cluster.UpgradePlan

// CertManagerUpgradePlan defines the upgrade plan if cert-manager needs to be
// upgraded to a different version.
type CertManagerUpgradePlan cluster.CertManagerUpgradePlan

// Kubeconfig is a type that specifies inputs related to the actual kubeconfig.
type Kubeconfig cluster.Kubeconfig

// Processor defines the methods necessary for creating a specific yaml
// processor.
type Processor yaml.Processor
