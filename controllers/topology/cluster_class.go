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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getClass gets the ClusterClass and the referenced templates to be used for a managed Cluster topology. It also converts
// and patches all ObjectReferences in ClusterClass and ControlPlane to the latest apiVersion of the current contract.
// NOTE: This function assumes that cluster.Spec.Topology.Class is set.
func (r *ClusterReconciler) getClass(ctx context.Context, cluster *clusterv1.Cluster) (_ *clusterTopologyClass, reterr error) {
	class := &clusterTopologyClass{
		clusterClass:       &clusterv1.ClusterClass{},
		machineDeployments: map[string]*machineDeploymentTopologyClass{},
	}

	// Get ClusterClass.
	key := client.ObjectKey{Name: cluster.Spec.Topology.Class, Namespace: cluster.Namespace}
	if err := r.Client.Get(ctx, key, class.clusterClass); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ClusterClass %q", cluster.Spec.Topology.Class)
	}

	// We use the patchHelper to patch potential changes to the ObjectReferences in ClusterClass.
	patchHelper, err := patch.NewHelper(class.clusterClass, r.Client)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := patchHelper.Patch(ctx, class.clusterClass); err != nil {
			reterr = kerrors.NewAggregate([]error{
				reterr,
				errors.Wrapf(err, "failed to patch ClusterClass %q", class.clusterClass.Name)},
			)
		}
	}()

	// Get ClusterClass.spec.infrastructure.
	class.infrastructureClusterTemplate, err = r.getReference(ctx, class.clusterClass.Spec.Infrastructure.Ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get infrastructure cluster template")
	}

	// Get ClusterClass.spec.controlPlane.
	class.controlPlane = &controlPlaneTopologyClass{}
	class.controlPlane.template, err = r.getReference(ctx, class.clusterClass.Spec.ControlPlane.Ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get control plane template")
	}

	// Check if ClusterClass.spec.ControlPlane.MachineInfrastructure is set, as it's optional.
	if class.clusterClass.Spec.ControlPlane.MachineInfrastructure != nil && class.clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref != nil {
		// Get ClusterClass.spec.controlPlane.machineInfrastructure.
		class.controlPlane.infrastructureMachineTemplate, err = r.getReference(ctx, class.clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get control plane's machine template")
		}
	}

	// Loop over the machine deployments classes in ClusterClass
	// and fetch the related templates.
	for _, mdc := range class.clusterClass.Spec.Workers.MachineDeployments {
		mdTopologyClass := &machineDeploymentTopologyClass{}

		// Make sure to copy the metadata from the class, which is later layered
		// with the additional metadata defined in the Cluster's topology section
		// for the MachineDeployment that is created or updated.
		mdc.Template.Metadata.DeepCopyInto(&mdTopologyClass.metadata)

		// Get the infrastructure machine template.
		mdTopologyClass.infrastructureMachineTemplate, err = r.getReference(ctx, mdc.Template.Infrastructure.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get MachineDeployment in class %q infrastructure machine template", mdc.Class)
		}

		// Get the bootstrap machine template.
		mdTopologyClass.bootstrapTemplate, err = r.getReference(ctx, mdc.Template.Bootstrap.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get MachineDeployment in class %q bootstrap machine template", mdc.Class)
		}

		class.machineDeployments[mdc.Class] = mdTopologyClass
	}

	return class, nil
}
