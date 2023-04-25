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

package scope

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
)

// ClusterState holds all the objects representing the state of a managed Cluster topology.
// NOTE: please note that we are going to deal with two different type state, the current state as read from the API server,
// and the desired state resulting from processing the ClusterBlueprint.
type ClusterState struct {
	// Cluster holds the Cluster object.
	Cluster *clusterv1.Cluster

	// InfrastructureCluster holds the infrastructure cluster object referenced by the Cluster.
	InfrastructureCluster *unstructured.Unstructured

	// ControlPlane holds the controlplane object referenced by the Cluster.
	ControlPlane *ControlPlaneState

	// MachineDeployments holds the machine deployments in the Cluster.
	MachineDeployments MachineDeploymentsStateMap
}

// ControlPlaneState holds all the objects representing the state of a managed control plane.
type ControlPlaneState struct {
	// Object holds the ControlPlane object.
	Object *unstructured.Unstructured

	// InfrastructureMachineTemplate holds the infrastructure template referenced by the ControlPlane object.
	InfrastructureMachineTemplate *unstructured.Unstructured

	// MachineHealthCheckClass holds the MachineHealthCheck for this ControlPlane.
	// +optional
	MachineHealthCheck *clusterv1.MachineHealthCheck
}

// MachineDeploymentsStateMap holds a collection of MachineDeployment states.
type MachineDeploymentsStateMap map[string]*MachineDeploymentState

// RollingOut returns the list of the machine deployments
// that are rolling out.
func (mds MachineDeploymentsStateMap) RollingOut() []string {
	names := []string{}
	for _, md := range mds {
		if md.IsRollingOut() {
			names = append(names, md.Object.Name)
		}
	}
	return names
}

// IsAnyRollingOut returns true if at least one of the
// machine deployments is rolling out. False, otherwise.
func (mds MachineDeploymentsStateMap) IsAnyRollingOut() bool {
	return len(mds.RollingOut()) != 0
}

// Upgrading returns the list of the machine deployments
// that are upgrading.
func (mds MachineDeploymentsStateMap) Upgrading(ctx context.Context, c client.Client) ([]string, error) {
	names := []string{}
	for _, md := range mds {
		upgrading, err := md.IsUpgrading(ctx, c)
		if err != nil {
			return nil, errors.Wrap(err, "failed to list upgrading MachineDeployments")
		}
		if upgrading {
			names = append(names, md.Object.Name)
		}
	}
	return names, nil
}

// MachineDeploymentState holds all the objects representing the state of a managed deployment.
type MachineDeploymentState struct {
	// Object holds the MachineDeployment object.
	Object *clusterv1.MachineDeployment

	// BootstrapTemplate holds the bootstrap template referenced by the MachineDeployment object.
	BootstrapTemplate *unstructured.Unstructured

	// InfrastructureMachineTemplate holds the infrastructure machine template referenced by the MachineDeployment object.
	InfrastructureMachineTemplate *unstructured.Unstructured

	// MachineHealthCheck holds a MachineHealthCheck linked to the MachineDeployment object.
	// +optional
	MachineHealthCheck *clusterv1.MachineHealthCheck
}

// IsRollingOut determines if the machine deployment is rolling out.
// A machine deployment is considered upgrading if:
// - if any of the replicas of the machine deployment is not ready.
func (md *MachineDeploymentState) IsRollingOut() bool {
	return !mdutil.DeploymentComplete(md.Object, &md.Object.Status) ||
		*md.Object.Spec.Replicas != md.Object.Status.ReadyReplicas ||
		md.Object.Status.UnavailableReplicas > 0
}

// IsUpgrading determines if the MachineDeployment is upgrading.
// A machine deployment is considered upgrading if at least one of the Machines of this
// MachineDeployment has a different version.
func (md *MachineDeploymentState) IsUpgrading(ctx context.Context, c client.Client) (bool, error) {
	// If the MachineDeployment has no version there is no definitive way to check if it is upgrading. Therefore, return false.
	// Note: This case should not happen.
	if md.Object.Spec.Template.Spec.Version == nil {
		return false, nil
	}
	selectorMap, err := metav1.LabelSelectorAsMap(&md.Object.Spec.Selector)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if MachineDeployment %s is upgrading: failed to convert label selector to map", md.Object.Name)
	}
	machines := &clusterv1.MachineList{}
	if err := c.List(ctx, machines, client.InNamespace(md.Object.Namespace), client.MatchingLabels(selectorMap)); err != nil {
		return false, errors.Wrapf(err, "failed to check if MachineDeployment %s is upgrading: failed to list Machines", md.Object.Name)
	}
	mdVersion := *md.Object.Spec.Template.Spec.Version
	// Check if the versions of the all the Machines match the MachineDeployment version.
	for i := range machines.Items {
		machine := machines.Items[i]
		if machine.Spec.Version == nil {
			return false, fmt.Errorf("failed to check if MachineDeployment %s is upgrading: Machine %s has no version", md.Object.Name, machine.Name)
		}
		if *machine.Spec.Version != mdVersion {
			return true, nil
		}
	}
	return false, nil
}
