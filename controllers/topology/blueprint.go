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

package topology

import (
	"context"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	tlog "sigs.k8s.io/cluster-api/controllers/topology/internal/log"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getBlueprint gets a ClusterBlueprint with the ClusterClass and the referenced templates to be used for a managed Cluster topology.
// It also converts and patches all ObjectReferences in ClusterClass and ControlPlane to the latest apiVersion of the current contract.
// NOTE: This function assumes that cluster.Spec.Topology.Class is set.
func (r *ClusterReconciler) getBlueprint(ctx context.Context, cluster *clusterv1.Cluster) (_ *scope.ClusterBlueprint, reterr error) {
	blueprint := &scope.ClusterBlueprint{
		Topology:           cluster.Spec.Topology,
		ClusterClass:       &clusterv1.ClusterClass{},
		MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{},
	}

	// Get ClusterClass.
	key := client.ObjectKey{Name: cluster.Spec.Topology.Class, Namespace: cluster.Namespace}
	if err := r.Client.Get(ctx, key, blueprint.ClusterClass); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ClusterClass/%s", cluster.Spec.Topology.Class)
	}

	// We use the patchHelper to patch potential changes to the ObjectReferences in ClusterClass.
	patchHelper, err := patch.NewHelper(blueprint.ClusterClass, r.Client)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := patchHelper.Patch(ctx, blueprint.ClusterClass); err != nil {
			reterr = kerrors.NewAggregate([]error{
				reterr,
				errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: blueprint.ClusterClass})},
			)
		}
	}()

	// Get ClusterClass.spec.infrastructure.
	blueprint.InfrastructureClusterTemplate, err = r.getReference(ctx, blueprint.ClusterClass.Spec.Infrastructure.Ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get infrastructure cluster template for %s", tlog.KObj{Obj: blueprint.ClusterClass})
	}

	// Get ClusterClass.spec.controlPlane.
	blueprint.ControlPlane = &scope.ControlPlaneBlueprint{}
	blueprint.ControlPlane.Template, err = r.getReference(ctx, blueprint.ClusterClass.Spec.ControlPlane.Ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get control plane template for %s", tlog.KObj{Obj: blueprint.ClusterClass})
	}

	// If the clusterClass mandates the controlPlane has infrastructureMachines, read it.
	if blueprint.HasControlPlaneInfrastructureMachine() {
		blueprint.ControlPlane.InfrastructureMachineTemplate, err = r.getReference(ctx, blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get control plane's machine template for %s", tlog.KObj{Obj: blueprint.ClusterClass})
		}
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
			return nil, errors.Wrapf(err, "failed to get infrastructure machine template for %s, MachineDeployment class %q", tlog.KObj{Obj: blueprint.ClusterClass}, machineDeploymentClass.Class)
		}

		// Get the bootstrap machine template.
		machineDeploymentBlueprint.BootstrapTemplate, err = r.getReference(ctx, machineDeploymentClass.Template.Bootstrap.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get bootstrap machine template for %s, MachineDeployment class %q", tlog.KObj{Obj: blueprint.ClusterClass}, machineDeploymentClass.Class)
		}

		blueprint.MachineDeployments[machineDeploymentClass.Class] = machineDeploymentBlueprint
	}

	return blueprint, nil
}
