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
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getCurrentState gets information about the current state of a Cluster by inspecting the state of the InfrastructureCluster,
// the ControlPlane, and the MachineDeployments associated with the Cluster.
func (r *ClusterReconciler) getCurrentState(ctx context.Context, cluster *clusterv1.Cluster, class *clusterv1.ClusterClass) (*clusterTopologyState, error) {
	clusterState := clusterTopologyState{
		cluster: cluster,
	}

	// Reference to the InfrastructureCluster can be nil and is expected to be on the first reconcile.
	// In this case the method should still be allowed to continue.
	if cluster.Spec.InfrastructureRef != nil {
		infra, err := r.getCurrentInfrastructureClusterState(ctx, cluster)
		if err != nil {
			return nil, err
		}
		clusterState.infrastructureCluster = infra
	}

	// Reference to the ControlPlane can be nil, and is expected to be on the first reconcile. In this case the method
	// should still be allowed to continue.
	if cluster.Spec.ControlPlaneRef != nil {
		cp, err := r.getCurrentControlPlaneState(ctx, cluster, class)
		if err != nil {
			return nil, err
		}
		clusterState.controlPlane = cp
	}

	// A Cluster may have zero or more MachineDeployments and a Cluster is expected to have zero MachineDeployments on
	// first reconcile.
	m, err := r.getCurrentMachineDeploymentState(ctx, cluster)
	if err != nil {
		return nil, err
	}
	clusterState.machineDeployments = m
	return &clusterState, nil
}

// getCurrentInfrastructureClusterState looks for the state of the InfrastructureCluster. If a reference is set but not
// found, either from an error or the object not being found, an error is thrown.
func (r *ClusterReconciler) getCurrentInfrastructureClusterState(ctx context.Context, cluster *clusterv1.Cluster) (*unstructured.Unstructured, error) {
	infra, err := r.getReference(ctx, cluster.Spec.InfrastructureRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s %s", cluster.Spec.InfrastructureRef.Kind, cluster.Spec.InfrastructureRef.Name)
	}
	return infra, nil
}

// getCurrentControlPlaneState returns information on the ControlPlane being used by the Cluster. If a reference is not found,
// an error is thrown. If the ControlPlane requires MachineInfrastructure according to its ClusterClass an error will be
// thrown if the ControlPlane has no MachineTemplates.
func (r *ClusterReconciler) getCurrentControlPlaneState(ctx context.Context, cluster *clusterv1.Cluster, class *clusterv1.ClusterClass) (controlPlaneTopologyState, error) {
	cp, err := r.getReference(ctx, cluster.Spec.ControlPlaneRef)
	if err != nil {
		return controlPlaneTopologyState{}, errors.Wrapf(err, "failed to read %s %s", cluster.Spec.ControlPlaneRef.Kind, cluster.Spec.ControlPlaneRef.Name)
	}
	// Some ControlPlane providers may not require MachineInfrastructure to run. This check returns early if the field
	// indicating MachineInfrastructure is required is not found in the ClusterClass of the given Cluster.
	if class.Spec.ControlPlane.MachineInfrastructure == nil {
		return controlPlaneTopologyState{
			cp,
			nil,
		}, nil
	}

	cpInfraRef, err := getNestedRef(cp, "spec", "machineTemplate", "infrastructureRef")
	if err != nil {
		return controlPlaneTopologyState{cp, nil}, errors.Wrapf(err, "failed to get InfrastructureMachineTemplate reference for %s, %s", cp.GetKind(), cp.GetName())
	}
	infra, err := r.getReference(ctx, cpInfraRef)
	if err != nil {
		return controlPlaneTopologyState{cp, nil}, errors.Wrapf(err, "failed to get InfrastructureMachineTemplate reference for %s, %s", cp.GetKind(), cp.GetName())
	}
	return controlPlaneTopologyState{
		cp,
		infra,
	}, nil
}

// getCurrentMachineDeploymentState queries for all MachineDeployments and filters them for their linked Cluster and
// whether they are managed by a ClusterClass using labels. A Cluster may have zero or more MachineDeployments. Zero is
// expected on first reconcile. If MachineDeployments are found for the Cluster their Infrastructure and Bootstrap references
// are inspected. Where these are not found the function will throw an error.
func (r *ClusterReconciler) getCurrentMachineDeploymentState(ctx context.Context, cluster *clusterv1.Cluster) (map[string]machineDeploymentTopologyState, error) {
	mDeploymentState := make(map[string]machineDeploymentTopologyState)

	md := &clusterv1.MachineDeploymentList{}
	err := r.Client.List(ctx, md, client.MatchingLabels{
		clusterv1.ClusterLabelName:         cluster.Name,
		clusterv1.ClusterTopologyLabelName: "",
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to read MachineDeployments for managed topology")
	}
	for _, m := range md.Items {
		machineDeployment := m
		mdTopologyName, ok := m.ObjectMeta.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]
		if !ok {
			return nil, fmt.Errorf("failed to find label %s in %s", clusterv1.ClusterTopologyMachineDeploymentLabelName, machineDeployment.Name)
		}
		if mdTopologyName == "" {
			return nil, fmt.Errorf("no value for MachineDeploymentTopoplogyName for %s", machineDeployment.Name)
		}
		if _, ok := mDeploymentState[mdTopologyName]; ok {
			return nil, fmt.Errorf("duplicate machine deployment %s found for label %s: %s", machineDeployment.Name, clusterv1.ClusterTopologyMachineDeploymentLabelName, mdTopologyName)
		}
		infraRef := &m.Spec.Template.Spec.InfrastructureRef
		if infraRef == nil {
			return nil, fmt.Errorf("MachineDeployment %s does not have a reference to a InfrastructureMachineTemplate", machineDeployment.Name)
		}

		bootstrapRef := m.Spec.Template.Spec.Bootstrap.ConfigRef
		if bootstrapRef == nil {
			return nil, fmt.Errorf("MachineDeployment %s does not have a reference to a Bootstrap Config", machineDeployment.Name)
		}

		i, err := r.getReference(ctx, infraRef)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("MachineDeployment %s Infrastructure reference could not be retrieved", machineDeployment.Name))
		}
		b, err := r.getReference(ctx, bootstrapRef)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("MachineDeployment %s Bootstrap reference could not be retrieved", machineDeployment.Name))
		}
		mDeploymentState[mdTopologyName] = machineDeploymentTopologyState{
			object:                        &machineDeployment,
			bootstrapTemplate:             b,
			infrastructureMachineTemplate: i,
		}
	}
	return mDeploymentState, nil
}
