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
	"fmt"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
)

const (
	// Builtin is the prefix of all builtin variables.
	// NOTE: this prefix acts as a "namespace" preventing name collision between
	// user defined variable and builtin variables.
	Builtin = "builtin"
)

var (
	// Builtin Cluster variables.

	// BuiltinClusterName is the name of the cluster.
	BuiltinClusterName = builtinVariable("cluster.name")

	// BuiltinClusterNamespace is the namespace of the cluster.
	BuiltinClusterNamespace = builtinVariable("cluster.namespace")

	// BuiltinClusterTopologyVersion is the Kubernetes version of the Cluster.
	// NOTE: Please note that this version might temporarily differ from the version
	// of the ControlPlane or workers while an upgrade process is being orchestrated.
	BuiltinClusterTopologyVersion = builtinVariable("cluster.topology.version")

	// BuiltinClusterTopologyClass is the name of the ClusterClass of the Cluster.
	BuiltinClusterTopologyClass = builtinVariable("cluster.topology.class")

	// Builtin ControlPlane variables.
	// NOTE: These variables are only set for templates belonging to the ControlPlane object.

	// BuiltinControlPlaneReplicas is the value of the replicas field of the ControlPlane object.
	BuiltinControlPlaneReplicas = builtinVariable("controlPlane.replicas")

	// BuiltinControlPlaneVersion is the Kubernetes version of the ControlPlane object.
	// NOTE: Please note that this version is the version we are currently reconciling towards.
	// It can differ from the current version of the ControlPlane while an upgrade process is
	// being orchestrated.
	BuiltinControlPlaneVersion = builtinVariable("controlPlane.version")

	// Builtin MachineDeployment variables.
	// NOTE: These variables are only set for templates belonging to a MachineDeployment.

	// BuiltinMachineDeploymentReplicas is the value of the replicas field of the MachineDeployment,
	// to which the current template belongs to.
	BuiltinMachineDeploymentReplicas = builtinVariable("machineDeployment.replicas")

	// BuiltinMachineDeploymentVersion is the Kubernetes version of the MachineDeployment,
	// to which the current template belongs to.
	// NOTE: Please note that this version is the version we are currently reconciling towards.
	// It can differ from the current version of the MachineDeployment machines while an upgrade process is
	// being orchestrated.
	BuiltinMachineDeploymentVersion = builtinVariable("machineDeployment.version")

	// BuiltinMachineDeploymentClass is the class name of the MachineDeployment,
	// to which the current template belongs to.
	BuiltinMachineDeploymentClass = builtinVariable("machineDeployment.class")

	// BuiltinMachineDeploymentName is the name of the MachineDeployment,
	// to which the current template belongs to.
	BuiltinMachineDeploymentName = builtinVariable("machineDeployment.name")

	// BuiltinMachineDeploymentTopologyName is the topology name of the MachineDeployment,
	// to which the current template belongs to.
	BuiltinMachineDeploymentTopologyName = builtinVariable("machineDeployment.topologyName")
)

func builtinVariable(name string) string {
	return fmt.Sprintf("%s.%s", Builtin, name)
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

	// Add builtin variables derived from the cluster object.
	if err := setVariable(variables, BuiltinClusterName, cluster.Name); err != nil {
		return nil, err
	}
	if err := setVariable(variables, BuiltinClusterNamespace, cluster.Namespace); err != nil {
		return nil, err
	}
	if err := setVariable(variables, BuiltinClusterTopologyVersion, cluster.Spec.Topology.Version); err != nil {
		return nil, err
	}
	if err := setVariable(variables, BuiltinClusterTopologyClass, cluster.Spec.Topology.Class); err != nil {
		return nil, err
	}

	return variables, nil
}

// ControlPlane returns variables that apply to templates belonging to the ControlPlane.
func ControlPlane(controlPlaneTopology *clusterv1.ControlPlaneTopology, controlPlane *unstructured.Unstructured) (VariableMap, error) {
	variables := VariableMap{}

	// If it is required to manage the number of replicas for the ControlPlane, set the corresponding variable.
	// NOTE: If the Cluster.spec.topology.controlPlane.replicas field is nil, the topology reconciler won't set
	// the replicas field on the ControlPlane. This happens either when the ControlPlane provider does
	// not implement support for this field or the default value of the ControlPlane is used.
	if controlPlaneTopology.Replicas != nil {
		replicas, err := contract.ControlPlane().Replicas().Get(controlPlane)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get spec.replicas from the ControlPlane")
		}
		if err := setVariable(variables, BuiltinControlPlaneReplicas, *replicas); err != nil {
			return nil, err
		}
	}

	version, err := contract.ControlPlane().Version().Get(controlPlane)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get spec.version from the ControlPlane")
	}
	if err := setVariable(variables, BuiltinControlPlaneVersion, version); err != nil {
		return nil, err
	}

	return variables, nil
}

// MachineDeployment returns variables that apply to templates belonging to a MachineDeployment.
func MachineDeployment(mdTopology *clusterv1.MachineDeploymentTopology, md *clusterv1.MachineDeployment) (VariableMap, error) {
	variables := VariableMap{}

	if md.Spec.Replicas != nil {
		if err := setVariable(variables, BuiltinMachineDeploymentReplicas, *md.Spec.Replicas); err != nil {
			return nil, err
		}
	}
	if err := setVariable(variables, BuiltinMachineDeploymentVersion, *md.Spec.Template.Spec.Version); err != nil {
		return nil, err
	}
	if err := setVariable(variables, BuiltinMachineDeploymentClass, mdTopology.Class); err != nil {
		return nil, err
	}
	if err := setVariable(variables, BuiltinMachineDeploymentName, md.Name); err != nil {
		return nil, err
	}
	if err := setVariable(variables, BuiltinMachineDeploymentTopologyName, mdTopology.Name); err != nil {
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
