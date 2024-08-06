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

package cluster

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
)

// getBlueprint gets a ClusterBlueprint with the ClusterClass and the referenced templates to be used for a managed Cluster topology.
// It also converts and patches all ObjectReferences in ClusterClass and ControlPlane to the latest apiVersion of the current contract.
// NOTE: This function assumes that cluster.Spec.Topology.Class is set.
func (r *Reconciler) getBlueprint(ctx context.Context, cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) (_ *scope.ClusterBlueprint, reterr error) {
	blueprint := &scope.ClusterBlueprint{
		Topology:           cluster.Spec.Topology,
		ClusterClass:       clusterClass,
		MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{},
		MachinePools:       map[string]*scope.MachinePoolBlueprint{},
	}

	var err error
	// Get ClusterClass.spec.infrastructure.
	blueprint.InfrastructureClusterTemplate, err = r.getReference(ctx, blueprint.ClusterClass.Spec.Infrastructure.Ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get infrastructure cluster template for ClusterClass %s", klog.KObj(blueprint.ClusterClass))
	}

	// Get ClusterClass.spec.controlPlane.
	blueprint.ControlPlane = &scope.ControlPlaneBlueprint{}
	blueprint.ControlPlane.Template, err = r.getReference(ctx, blueprint.ClusterClass.Spec.ControlPlane.Ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get control plane template for ClusterClass %s", klog.KObj(blueprint.ClusterClass))
	}

	// If the clusterClass mandates the controlPlane has infrastructureMachines, read it.
	if blueprint.HasControlPlaneInfrastructureMachine() {
		blueprint.ControlPlane.InfrastructureMachineTemplate, err = r.getReference(ctx, blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get control plane's machine template for ClusterClass %s", klog.KObj(blueprint.ClusterClass))
		}
	}

	// If the clusterClass defines a valid MachineHealthCheck (including a defined MachineInfrastructure) set the blueprint MachineHealthCheck.
	if blueprint.HasControlPlaneMachineHealthCheck() {
		blueprint.ControlPlane.MachineHealthCheck = blueprint.ClusterClass.Spec.ControlPlane.MachineHealthCheck
	}

	// Loop over the machine deployments classes in ClusterClass
	// and fetch the related templates.
	for _, machineDeploymentClass := range blueprint.ClusterClass.Spec.Workers.MachineDeployments {
		machineDeploymentBlueprint := &scope.MachineDeploymentBlueprint{}

		// Make sure to copy the metadata from the blueprint, which is later layered
		// with the additional metadata defined in the Cluster's topology section
		// for the MachineDeployment that is created or updated.
		machineDeploymentClass.Template.Metadata.DeepCopyInto(&machineDeploymentBlueprint.Metadata)

		// Get the infrastructure machine template.
		machineDeploymentBlueprint.InfrastructureMachineTemplate, err = r.getReference(ctx, machineDeploymentClass.Template.Infrastructure.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get infrastructure machine template for ClusterClass %s, MachineDeployment class %q", klog.KObj(blueprint.ClusterClass), machineDeploymentClass.Class)
		}

		// Get the bootstrap config template.
		machineDeploymentBlueprint.BootstrapTemplate, err = r.getReference(ctx, machineDeploymentClass.Template.Bootstrap.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get bootstrap config template for ClusterClass %s, MachineDeployment class %q", klog.KObj(blueprint.ClusterClass), machineDeploymentClass.Class)
		}

		// If the machineDeploymentClass defines a MachineHealthCheck add it to the blueprint.
		if machineDeploymentClass.MachineHealthCheck != nil {
			machineDeploymentBlueprint.MachineHealthCheck = machineDeploymentClass.MachineHealthCheck
		}
		blueprint.MachineDeployments[machineDeploymentClass.Class] = machineDeploymentBlueprint
	}

	// Loop over the machine pool classes in ClusterClass
	// and fetch the related templates.
	for _, machinePoolClass := range blueprint.ClusterClass.Spec.Workers.MachinePools {
		machinePoolBlueprint := &scope.MachinePoolBlueprint{}

		// Make sure to copy the metadata from the blueprint, which is later layered
		// with the additional metadata defined in the Cluster's topology section
		// for the MachinePool that is created or updated.
		machinePoolClass.Template.Metadata.DeepCopyInto(&machinePoolBlueprint.Metadata)

		// Get the InfrastructureMachinePoolTemplate.
		machinePoolBlueprint.InfrastructureMachinePoolTemplate, err = r.getReference(ctx, machinePoolClass.Template.Infrastructure.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get InfrastructureMachinePoolTemplate for ClusterClass %s, MachinePool class %q", klog.KObj(blueprint.ClusterClass), machinePoolClass.Class)
		}

		// Get the bootstrap config template.
		machinePoolBlueprint.BootstrapTemplate, err = r.getReference(ctx, machinePoolClass.Template.Bootstrap.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get bootstrap config for ClusterClass %s, MachinePool class %q", klog.KObj(blueprint.ClusterClass), machinePoolClass.Class)
		}

		blueprint.MachinePools[machinePoolClass.Class] = machinePoolBlueprint
	}

	return blueprint, nil
}
