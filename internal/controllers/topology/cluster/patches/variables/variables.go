/*
Copyright 2021 The Kubernetes Authors.

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

// Package variables calculates variables for patching.
package variables

import (
	"encoding/json"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
)

const (
	// BuiltinsName is the name of the builtin variable.
	BuiltinsName = "builtin"
)

// Builtins represents builtin variables exposed through patches.
type Builtins struct {
	Cluster           *ClusterBuiltins           `json:"cluster,omitempty"`
	ControlPlane      *ControlPlaneBuiltins      `json:"controlPlane,omitempty"`
	MachineDeployment *MachineDeploymentBuiltins `json:"machineDeployment,omitempty"`
}

// ClusterBuiltins represents builtin cluster variables.
type ClusterBuiltins struct {
	// Name is the name of the cluster.
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the cluster.
	Namespace string `json:"namespace,omitempty"`

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
	Bootstrap *MachineDeploymentBootstrapBuiltins `json:"bootstrap,omitempty"`

	// InfrastructureRef is the value of the .spec.template.spec.bootstrap field of the MachineDeployment.
	InfrastructureRef *MachineDeploymentInfrastructureRefBuiltins `json:"infrastructureRef,omitempty"`
}

// MachineDeploymentBootstrapBuiltins is the value of the .spec.template.spec.bootstrap field
// of the MachineDeployment.
type MachineDeploymentBootstrapBuiltins struct {
	// ConfigRef is the value of the .spec.template.spec.bootstrap.configRef field of the MachineDeployment.
	ConfigRef *MachineDeploymentBootstrapConfigRefBuiltins `json:"configRef,omitempty"`
}

// MachineDeploymentBootstrapConfigRefBuiltins is the value of the .spec.template.spec.bootstrap.configRef
// field of the MachineDeployment.
type MachineDeploymentBootstrapConfigRefBuiltins struct {
	// Name of the bootstrap.configRef.
	Name string `json:"name,omitempty"`
}

// MachineDeploymentInfrastructureRefBuiltins is the value of the .spec.template.spec.infrastructureRef field
// of the MachineDeployment.
type MachineDeploymentInfrastructureRefBuiltins struct {
	// Name of the infrastructureRef.
	Name string `json:"name,omitempty"`
}

// VariableMap is a name/value map of variables.
// Values are marshalled as JSON.
type VariableMap map[string]apiextensionsv1.JSON

// Global returns variables that apply to all the templates, including user provided variables
// and builtin variables for the Cluster object.
func Global(clusterTopology *clusterv1.Topology, cluster *clusterv1.Cluster) (VariableMap, error) {
	variables := VariableMap{}

	// Add user defined variables from Cluster.spec.topology.variables.
	for _, variable := range clusterTopology.Variables {
		variables[variable.Name] = variable.Value
	}

	// Construct builtin variable.
	builtin := Builtins{
		Cluster: &ClusterBuiltins{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
			Topology: &ClusterTopologyBuiltins{
				Version: cluster.Spec.Topology.Version,
				Class:   cluster.Spec.Topology.Class,
			},
		},
	}
	if cluster.Spec.ClusterNetwork != nil {
		clusterNetworkIPFamily, err := cluster.GetIPFamily()
		if err != nil {
			return nil, err
		}
		builtin.Cluster.Network = &ClusterNetworkBuiltins{
			IPFamily: ipFamilyToString(clusterNetworkIPFamily),
		}
		if cluster.Spec.ClusterNetwork.ServiceDomain != "" {
			builtin.Cluster.Network.ServiceDomain = &cluster.Spec.ClusterNetwork.ServiceDomain
		}
		if cluster.Spec.ClusterNetwork.Services != nil && cluster.Spec.ClusterNetwork.Services.CIDRBlocks != nil {
			builtin.Cluster.Network.Services = cluster.Spec.ClusterNetwork.Services.CIDRBlocks
		}
		if cluster.Spec.ClusterNetwork.Pods != nil && cluster.Spec.ClusterNetwork.Pods.CIDRBlocks != nil {
			builtin.Cluster.Network.Pods = cluster.Spec.ClusterNetwork.Pods.CIDRBlocks
		}
	}

	// Add builtin variables derived from the cluster object.
	if err := setVariable(variables, BuiltinsName, builtin); err != nil {
		return nil, err
	}

	return variables, nil
}

// ControlPlane returns variables that apply to templates belonging to the ControlPlane.
func ControlPlane(cpTopology *clusterv1.ControlPlaneTopology, cp, cpInfrastructureMachineTemplate *unstructured.Unstructured) (VariableMap, error) {
	variables := VariableMap{}

	// Construct builtin variable.
	builtin := Builtins{
		ControlPlane: &ControlPlaneBuiltins{
			Name: cp.GetName(),
		},
	}

	// If it is required to manage the number of replicas for the ControlPlane, set the corresponding variable.
	// NOTE: If the Cluster.spec.topology.controlPlane.replicas field is nil, the topology reconciler won't set
	// the replicas field on the ControlPlane. This happens either when the ControlPlane provider does
	// not implement support for this field or the default value of the ControlPlane is used.
	if cpTopology.Replicas != nil {
		replicas, err := contract.ControlPlane().Replicas().Get(cp)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get spec.replicas from the ControlPlane")
		}
		builtin.ControlPlane.Replicas = replicas
	}

	version, err := contract.ControlPlane().Version().Get(cp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get spec.version from the ControlPlane")
	}
	builtin.ControlPlane.Version = *version

	if cpInfrastructureMachineTemplate != nil {
		builtin.ControlPlane.MachineTemplate = &ControlPlaneMachineTemplateBuiltins{
			InfrastructureRef: ControlPlaneMachineTemplateInfrastructureRefBuiltins{
				Name: cpInfrastructureMachineTemplate.GetName(),
			},
		}
	}

	if err := setVariable(variables, BuiltinsName, builtin); err != nil {
		return nil, err
	}

	return variables, nil
}

// MachineDeployment returns variables that apply to templates belonging to a MachineDeployment.
func MachineDeployment(mdTopology *clusterv1.MachineDeploymentTopology, md *clusterv1.MachineDeployment, mdBootstrapTemplate, mdInfrastructureMachineTemplate *unstructured.Unstructured) (VariableMap, error) {
	variables := VariableMap{}

	// Add variables overrides for the MachineDeployment.
	if mdTopology.Variables != nil {
		for _, variable := range mdTopology.Variables.Overrides {
			variables[variable.Name] = variable.Value
		}
	}

	// Construct builtin variable.
	builtin := Builtins{
		MachineDeployment: &MachineDeploymentBuiltins{
			Version:      *md.Spec.Template.Spec.Version,
			Class:        mdTopology.Class,
			Name:         md.Name,
			TopologyName: mdTopology.Name,
		},
	}
	if md.Spec.Replicas != nil {
		builtin.MachineDeployment.Replicas = pointer.Int64(int64(*md.Spec.Replicas))
	}

	if mdBootstrapTemplate != nil {
		builtin.MachineDeployment.Bootstrap = &MachineDeploymentBootstrapBuiltins{
			ConfigRef: &MachineDeploymentBootstrapConfigRefBuiltins{
				Name: mdBootstrapTemplate.GetName(),
			},
		}
	}

	if mdInfrastructureMachineTemplate != nil {
		builtin.MachineDeployment.InfrastructureRef = &MachineDeploymentInfrastructureRefBuiltins{
			Name: mdInfrastructureMachineTemplate.GetName(),
		}
	}

	if err := setVariable(variables, BuiltinsName, builtin); err != nil {
		return nil, err
	}

	return variables, nil
}

// setVariable converts value to JSON and adds the variable to the variables map.
func setVariable(variables VariableMap, name string, value interface{}) error {
	marshalledValue, err := json.Marshal(value)
	if err != nil {
		return errors.Wrapf(err, "failed to set variable %q: error marshalling", name)
	}

	variables[name] = apiextensionsv1.JSON{Raw: marshalledValue}
	return nil
}

func ipFamilyToString(ipFamily clusterv1.ClusterIPFamily) string {
	switch ipFamily {
	case clusterv1.DualStackIPFamily:
		return "DualStack"
	case clusterv1.IPv4IPFamily:
		return "IPv4"
	case clusterv1.IPv6IPFamily:
		return "IPv6"
	default:
		return "Invalid"
	}
}
