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
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
)

const (
	// BuiltinsName is the name of the builtin variable.
	// NOTE: User-defined variables are not allowed to start with "builtin".
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

// ControlPlaneBuiltins represents builtin ControlPlane variables.
// NOTE: These variables are only set for templates belonging to the ControlPlane object.
type ControlPlaneBuiltins struct {
	// Version is the Kubernetes version of the ControlPlane object.
	// NOTE: Please note that this version is the version we are currently reconciling towards.
	// It can differ from the current version of the ControlPlane while an upgrade process is
	// being orchestrated.
	Version string `json:"version,omitempty"`

	// Replicas is the value of the replicas field of the ControlPlane object.
	Replicas *int64 `json:"replicas,omitempty"`
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

	// Add builtin variables derived from the cluster object.
	if err := setVariable(variables, BuiltinsName, builtin); err != nil {
		return nil, err
	}

	return variables, nil
}

// ControlPlane returns variables that apply to templates belonging to the ControlPlane.
func ControlPlane(controlPlaneTopology *clusterv1.ControlPlaneTopology, controlPlane *unstructured.Unstructured) (VariableMap, error) {
	variables := VariableMap{}

	// Construct builtin variable.
	builtin := Builtins{
		ControlPlane: &ControlPlaneBuiltins{},
	}

	// If it is required to manage the number of replicas for the ControlPlane, set the corresponding variable.
	// NOTE: If the Cluster.spec.topology.controlPlane.replicas field is nil, the topology reconciler won't set
	// the replicas field on the ControlPlane. This happens either when the ControlPlane provider does
	// not implement support for this field or the default value of the ControlPlane is used.
	if controlPlaneTopology.Replicas != nil {
		replicas, err := contract.ControlPlane().Replicas().Get(controlPlane)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get spec.replicas from the ControlPlane")
		}
		builtin.ControlPlane.Replicas = replicas
	}

	version, err := contract.ControlPlane().Version().Get(controlPlane)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get spec.version from the ControlPlane")
	}
	builtin.ControlPlane.Version = *version

	if err := setVariable(variables, BuiltinsName, builtin); err != nil {
		return nil, err
	}

	return variables, nil
}

// MachineDeployment returns variables that apply to templates belonging to a MachineDeployment.
func MachineDeployment(mdTopology *clusterv1.MachineDeploymentTopology, md *clusterv1.MachineDeployment) (VariableMap, error) {
	variables := VariableMap{}

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
