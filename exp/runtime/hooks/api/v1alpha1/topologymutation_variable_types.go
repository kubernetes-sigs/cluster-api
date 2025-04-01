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
	// cluster represents builtin cluster variables.
	// +optional
	Cluster *ClusterBuiltins `json:"cluster,omitempty"`
	// controlPlane represents builtin ControlPlane variables.
	// +optional
	ControlPlane *ControlPlaneBuiltins `json:"controlPlane,omitempty"`
	// machineDeployment represents builtin MachineDeployment variables.
	// +optional
	MachineDeployment *MachineDeploymentBuiltins `json:"machineDeployment,omitempty"`
	// machinePool represents builtin MachinePool variables.
	// +optional
	MachinePool *MachinePoolBuiltins `json:"machinePool,omitempty"`
}

// ClusterBuiltins represents builtin cluster variables.
type ClusterBuiltins struct {
	// name is the name of the cluster.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace is the namespace of the cluster.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// uid is the unqiue identifier of the cluster.
	// +optional
	UID types.UID `json:"uid,omitempty"`

	// metadata is the metadata set on the Cluster object.
	// +optional
	Metadata *clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// topology represents the cluster topology variables.
	// +optional
	Topology *ClusterTopologyBuiltins `json:"topology,omitempty"`

	// network represents the cluster network variables.
	// +optional
	Network *ClusterNetworkBuiltins `json:"network,omitempty"`
}

// ClusterTopologyBuiltins represents builtin cluster topology variables.
type ClusterTopologyBuiltins struct {
	// version is the Kubernetes version of the Cluster.
	// NOTE: Please note that this version might temporarily differ from the version
	// of the ControlPlane or workers while an upgrade process is being orchestrated.
	// +optional
	Version string `json:"version,omitempty"`

	// class is the name of the ClusterClass of the Cluster.
	// +optional
	Class string `json:"class,omitempty"`

	// classNamespace is the namespace of the ClusterClass of the Cluster.
	// +optional
	ClassNamespace string `json:"classNamespace,omitempty"`
}

// ClusterNetworkBuiltins represents builtin cluster network variables.
type ClusterNetworkBuiltins struct {
	// serviceDomain is the domain name for services.
	// +optional
	ServiceDomain *string `json:"serviceDomain,omitempty"`
	// services is the network ranges from which service VIPs are allocated.
	// +optional
	Services []string `json:"services,omitempty"`
	// pods is the network ranges from which Pod networks are allocated.
	// +optional
	Pods []string `json:"pods,omitempty"`
	// ipFamily is the IPFamily the Cluster is operating in. One of Invalid, IPv4, IPv6, DualStack.
	//
	// Deprecated: IPFamily is not a concept in Kubernetes. It was originally introduced in CAPI for CAPD.
	// IPFamily will be dropped in a future release. More details at https://github.com/kubernetes-sigs/cluster-api/issues/7521
	// +optional
	IPFamily string `json:"ipFamily,omitempty"`
}

// ControlPlaneBuiltins represents builtin ControlPlane variables.
// NOTE: These variables are only set for templates belonging to the ControlPlane object.
type ControlPlaneBuiltins struct {
	// version is the Kubernetes version of the ControlPlane object.
	// NOTE: Please note that this version is the version we are currently reconciling towards.
	// It can differ from the current version of the ControlPlane while an upgrade process is
	// being orchestrated.
	// +optional
	Version string `json:"version,omitempty"`

	// metadata is the metadata set on the ControlPlane object.
	// +optional
	Metadata *clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// name is the name of the ControlPlane,
	// to which the current template belongs to.
	// +optional
	Name string `json:"name,omitempty"`

	// replicas is the value of the replicas field of the ControlPlane object.
	// +optional
	Replicas *int64 `json:"replicas,omitempty"`

	// machineTemplate is the value of the .spec.machineTemplate field of the ControlPlane object.
	// +optional
	MachineTemplate *ControlPlaneMachineTemplateBuiltins `json:"machineTemplate,omitempty"`
}

// ControlPlaneMachineTemplateBuiltins is the value of the .spec.machineTemplate field of the ControlPlane object.
type ControlPlaneMachineTemplateBuiltins struct {
	// infrastructureRef is the value of the infrastructureRef field of ControlPlane.spec.machineTemplate.
	// +optional
	InfrastructureRef ControlPlaneMachineTemplateInfrastructureRefBuiltins `json:"infrastructureRef,omitempty"`
}

// ControlPlaneMachineTemplateInfrastructureRefBuiltins is the value of the infrastructureRef field of
// ControlPlane.spec.machineTemplate.
type ControlPlaneMachineTemplateInfrastructureRefBuiltins struct {
	// name of the infrastructureRef.
	// +optional
	Name string `json:"name,omitempty"`
}

// MachineDeploymentBuiltins represents builtin MachineDeployment variables.
// NOTE: These variables are only set for templates belonging to a MachineDeployment.
type MachineDeploymentBuiltins struct {
	// version is the Kubernetes version of the MachineDeployment,
	// to which the current template belongs to.
	// NOTE: Please note that this version is the version we are currently reconciling towards.
	// It can differ from the current version of the MachineDeployment machines while an upgrade process is
	// being orchestrated.
	// +optional
	Version string `json:"version,omitempty"`

	// metadata is the metadata set on the MachineDeployment.
	// +optional
	Metadata *clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// class is the class name of the MachineDeployment,
	// to which the current template belongs to.
	// +optional
	Class string `json:"class,omitempty"`

	// name is the name of the MachineDeployment,
	// to which the current template belongs to.
	// +optional
	Name string `json:"name,omitempty"`

	// topologyName is the topology name of the MachineDeployment,
	// to which the current template belongs to.
	// +optional
	TopologyName string `json:"topologyName,omitempty"`

	// replicas is the value of the replicas field of the MachineDeployment,
	// to which the current template belongs to.
	// +optional
	Replicas *int64 `json:"replicas,omitempty"`

	// bootstrap is the value of the .spec.template.spec.bootstrap field of the MachineDeployment.
	// +optional
	Bootstrap *MachineBootstrapBuiltins `json:"bootstrap,omitempty"`

	// infrastructureRef is the value of the .spec.template.spec.infrastructureRef field of the MachineDeployment.
	// +optional
	InfrastructureRef *MachineInfrastructureRefBuiltins `json:"infrastructureRef,omitempty"`
}

// MachinePoolBuiltins represents builtin MachinePool variables.
// NOTE: These variables are only set for templates belonging to a MachinePool.
type MachinePoolBuiltins struct {
	// version is the Kubernetes version of the MachinePool,
	// to which the current template belongs to.
	// NOTE: Please note that this version is the version we are currently reconciling towards.
	// It can differ from the current version of the MachinePool machines while an upgrade process is
	// being orchestrated.
	// +optional
	Version string `json:"version,omitempty"`

	// metadata is the metadata set on the MachinePool.
	// +optional
	Metadata *clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// class is the class name of the MachinePool,
	// to which the current template belongs to.
	// +optional
	Class string `json:"class,omitempty"`

	// name is the name of the MachinePool,
	// to which the current template belongs to.
	// +optional
	Name string `json:"name,omitempty"`

	// topologyName is the topology name of the MachinePool,
	// to which the current template belongs to.
	// +optional
	TopologyName string `json:"topologyName,omitempty"`

	// replicas is the value of the replicas field of the MachinePool,
	// to which the current template belongs to.
	// +optional
	Replicas *int64 `json:"replicas,omitempty"`

	// bootstrap is the value of the .spec.template.spec.bootstrap field of the MachinePool.
	// +optional
	Bootstrap *MachineBootstrapBuiltins `json:"bootstrap,omitempty"`

	// infrastructureRef is the value of the .spec.template.spec.infrastructureRef field of the MachinePool.
	// +optional
	InfrastructureRef *MachineInfrastructureRefBuiltins `json:"infrastructureRef,omitempty"`
}

// MachineBootstrapBuiltins is the value of the .spec.template.spec.bootstrap field
// of the MachineDeployment or MachinePool.
type MachineBootstrapBuiltins struct {
	// configRef is the value of the .spec.template.spec.bootstrap.configRef field of the MachineDeployment.
	// +optional
	ConfigRef *MachineBootstrapConfigRefBuiltins `json:"configRef,omitempty"`
}

// MachineBootstrapConfigRefBuiltins is the value of the .spec.template.spec.bootstrap.configRef
// field of the MachineDeployment or MachinePool.
type MachineBootstrapConfigRefBuiltins struct {
	// name of the bootstrap.configRef.
	// +optional
	Name string `json:"name,omitempty"`
}

// MachineInfrastructureRefBuiltins is the value of the .spec.template.spec.infrastructureRef field
// of the MachineDeployment or MachinePool.
type MachineInfrastructureRefBuiltins struct {
	// name of the infrastructureRef.
	// +optional
	Name string `json:"name,omitempty"`
}
