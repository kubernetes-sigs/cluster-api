/*
Copyright 2024 The Kubernetes Authors.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/types"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// BuiltinsName is the name of the builtin variable.
const BuiltinsName = "builtin"

// Builtins represents builtin variables exposed through patches.
type Builtins struct {
	Cluster           *ClusterBuiltins           `json:"cluster,omitempty"`
	ControlPlane      *ControlPlaneBuiltins      `json:"controlPlane,omitempty"`
	MachineDeployment *MachineDeploymentBuiltins `json:"machineDeployment,omitempty"`
	MachinePool       *MachinePoolBuiltins       `json:"machinePool,omitempty"`
}

// ClusterBuiltins represents builtin cluster variables.
type ClusterBuiltins struct {
	// Name is the name of the cluster.
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the cluster.
	Namespace string `json:"namespace,omitempty"`

	// UID is the unqiue identifier of the cluster.
	UID types.UID `json:"uid,omitempty"`

	// Topology represents the cluster topology variables.
	Topology *ClusterTopologyBuiltins `json:"topology,omitempty"`

	// Network represents the cluster network variables.
	Network *ClusterNetworkBuiltins `json:"network,omitempty"`
}

// ClusterTopologyBuiltins represents builtin cluster topology variables.
type ClusterTopologyBuiltins struct {
	// Version is the Kubernetes version of the Cluster.
	// NOTE: Please note that this version might temporarily differ from the version
	// of the ControlPlane or workers while an upgrade process is being orchestrated.
	Version string `json:"version,omitempty"`

	// Class is the name of the ClusterClass of the Cluster.
	Class string `json:"class,omitempty"`
}

// ClusterNetworkBuiltins represents builtin cluster network variables.
type ClusterNetworkBuiltins struct {
	// ServiceDomain is the domain name for services.
	ServiceDomain *string `json:"serviceDomain,omitempty"`
	// Services is the network ranges from which service VIPs are allocated.
	Services []string `json:"services,omitempty"`
	// Pods is the network ranges from which Pod networks are allocated.
	Pods []string `json:"pods,omitempty"`
	// IPFamily is the IPFamily the Cluster is operating in. One of Invalid, IPv4, IPv6, DualStack.
	//
	// Deprecated: IPFamily is not a concept in Kubernetes. It was originally introduced in CAPI for CAPD.
	// IPFamily will be dropped in a future release. More details at https://github.com/kubernetes-sigs/cluster-api/issues/7521
	IPFamily string `json:"ipFamily,omitempty"`
}

// ControlPlaneBuiltins represents builtin ControlPlane variables.
// NOTE: These variables are only set for templates belonging to the ControlPlane object.
type ControlPlaneBuiltins struct {
	// Version is the Kubernetes version of the ControlPlane object.
	// NOTE: Please note that this version is the version we are currently reconciling towards.
	// It can differ from the current version of the ControlPlane while an upgrade process is
	// being orchestrated.
	Version string `json:"version,omitempty"`

	// Metadata is the metadata set on the ControlPlane object.
	Metadata *clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// Name is the name of the ControlPlane,
	// to which the current template belongs to.
	Name string `json:"name,omitempty"`

	// Replicas is the value of the replicas field of the ControlPlane object.
	Replicas *int64 `json:"replicas,omitempty"`

	// MachineTemplate is the value of the .spec.machineTemplate field of the ControlPlane object.
	MachineTemplate *ControlPlaneMachineTemplateBuiltins `json:"machineTemplate,omitempty"`
}

// ControlPlaneMachineTemplateBuiltins is the value of the .spec.machineTemplate field of the ControlPlane object.
type ControlPlaneMachineTemplateBuiltins struct {
	// InfrastructureRef is the value of the infrastructureRef field of ControlPlane.spec.machineTemplate.
	InfrastructureRef ControlPlaneMachineTemplateInfrastructureRefBuiltins `json:"infrastructureRef,omitempty"`
}

// ControlPlaneMachineTemplateInfrastructureRefBuiltins is the value of the infrastructureRef field of
// ControlPlane.spec.machineTemplate.
type ControlPlaneMachineTemplateInfrastructureRefBuiltins struct {
	// Name of the infrastructureRef.
	Name string `json:"name,omitempty"`
}

// MachineDeploymentBuiltins represents builtin MachineDeployment variables.
// NOTE: These variables are only set for templates belonging to a MachineDeployment.
type MachineDeploymentBuiltins struct {
	// Version is the Kubernetes version of the MachineDeployment,
	// to which the current template belongs to.
	// NOTE: Please note that this version is the version we are currently reconciling towards.
	// It can differ from the current version of the MachineDeployment machines while an upgrade process is
	// being orchestrated.
	Version string `json:"version,omitempty"`

	// Metadata is the metadata set on the MachineDeployment.
	Metadata *clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// Class is the class name of the MachineDeployment,
	// to which the current template belongs to.
	Class string `json:"class,omitempty"`

	// Name is the name of the MachineDeployment,
	// to which the current template belongs to.
	Name string `json:"name,omitempty"`

	// TopologyName is the topology name of the MachineDeployment,
	// to which the current template belongs to.
	TopologyName string `json:"topologyName,omitempty"`

	// Replicas is the value of the replicas field of the MachineDeployment,
	// to which the current template belongs to.
	Replicas *int64 `json:"replicas,omitempty"`

	// Bootstrap is the value of the .spec.template.spec.bootstrap field of the MachineDeployment.
	Bootstrap *MachineBootstrapBuiltins `json:"bootstrap,omitempty"`

	// InfrastructureRef is the value of the .spec.template.spec.infrastructureRef field of the MachineDeployment.
	InfrastructureRef *MachineInfrastructureRefBuiltins `json:"infrastructureRef,omitempty"`
}

// MachinePoolBuiltins represents builtin MachinePool variables.
// NOTE: These variables are only set for templates belonging to a MachinePool.
type MachinePoolBuiltins struct {
	// Version is the Kubernetes version of the MachinePool,
	// to which the current template belongs to.
	// NOTE: Please note that this version is the version we are currently reconciling towards.
	// It can differ from the current version of the MachinePool machines while an upgrade process is
	// being orchestrated.
	Version string `json:"version,omitempty"`

	// Metadata is the metadata set on the MachinePool.
	Metadata *clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// Class is the class name of the MachinePool,
	// to which the current template belongs to.
	Class string `json:"class,omitempty"`

	// Name is the name of the MachinePool,
	// to which the current template belongs to.
	Name string `json:"name,omitempty"`

	// TopologyName is the topology name of the MachinePool,
	// to which the current template belongs to.
	TopologyName string `json:"topologyName,omitempty"`

	// Replicas is the value of the replicas field of the MachinePool,
	// to which the current template belongs to.
	Replicas *int64 `json:"replicas,omitempty"`

	// Bootstrap is the value of the .spec.template.spec.bootstrap field of the MachinePool.
	Bootstrap *MachineBootstrapBuiltins `json:"bootstrap,omitempty"`

	// InfrastructureRef is the value of the .spec.template.spec.infrastructureRef field of the MachinePool.
	InfrastructureRef *MachineInfrastructureRefBuiltins `json:"infrastructureRef,omitempty"`
}

// MachineBootstrapBuiltins is the value of the .spec.template.spec.bootstrap field
// of the MachineDeployment or MachinePool.
type MachineBootstrapBuiltins struct {
	// ConfigRef is the value of the .spec.template.spec.bootstrap.configRef field of the MachineDeployment.
	ConfigRef *MachineBootstrapConfigRefBuiltins `json:"configRef,omitempty"`
}

// MachineBootstrapConfigRefBuiltins is the value of the .spec.template.spec.bootstrap.configRef
// field of the MachineDeployment or MachinePool.
type MachineBootstrapConfigRefBuiltins struct {
	// Name of the bootstrap.configRef.
	Name string `json:"name,omitempty"`
}

// MachineInfrastructureRefBuiltins is the value of the .spec.template.spec.infrastructureRef field
// of the MachineDeployment or MachinePool.
type MachineInfrastructureRefBuiltins struct {
	// Name of the infrastructureRef.
	Name string `json:"name,omitempty"`
}
