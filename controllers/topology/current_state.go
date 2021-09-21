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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	tlog "sigs.k8s.io/cluster-api/controllers/topology/internal/log"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getCurrentState gets information about the current state of a Cluster by inspecting the state of the InfrastructureCluster,
// the ControlPlane, and the MachineDeployments associated with the Cluster.
func (r *ClusterReconciler) getCurrentState(ctx context.Context, s *scope.Scope) (*scope.ClusterState, error) {
	// NOTE: current scope has been already initialized with the Cluster.
	currentState := s.Current

	// Reference to the InfrastructureCluster can be nil and is expected to be on the first reconcile.
	// In this case the method should still be allowed to continue.
	if currentState.Cluster.Spec.InfrastructureRef != nil {
		infra, err := r.getCurrentInfrastructureClusterState(ctx, currentState.Cluster)
		if err != nil {
			return nil, err
		}
		currentState.InfrastructureCluster = infra
	}

	// Reference to the ControlPlane can be nil, and is expected to be on the first reconcile. In this case the method
	// should still be allowed to continue.
	currentState.ControlPlane = &scope.ControlPlaneState{}
	if currentState.Cluster.Spec.ControlPlaneRef != nil {
		cp, err := r.getCurrentControlPlaneState(ctx, currentState.Cluster, s.Blueprint)
		if err != nil {
			return nil, err
		}
		currentState.ControlPlane = cp
	}

	// A Cluster may have zero or more MachineDeployments and a Cluster is expected to have zero MachineDeployments on
	// first reconcile.
	m, err := r.getCurrentMachineDeploymentState(ctx, currentState.Cluster)
	if err != nil {
		return nil, err
	}
	currentState.MachineDeployments = m

	return currentState, nil
}

// getCurrentInfrastructureClusterState looks for the state of the InfrastructureCluster. If a reference is set but not
// found, either from an error or the object not being found, an error is thrown.
func (r *ClusterReconciler) getCurrentInfrastructureClusterState(ctx context.Context, cluster *clusterv1.Cluster) (*unstructured.Unstructured, error) {
	infra, err := r.getReference(ctx, cluster.Spec.InfrastructureRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", tlog.KRef{Ref: cluster.Spec.InfrastructureRef})
	}
	return infra, nil
}

// getCurrentControlPlaneState returns information on the ControlPlane being used by the Cluster. If a reference is not found,
// an error is thrown. If the ControlPlane requires MachineInfrastructure according to its ClusterClass an error will be
// thrown if the ControlPlane has no MachineTemplates.
func (r *ClusterReconciler) getCurrentControlPlaneState(ctx context.Context, cluster *clusterv1.Cluster, blueprint *scope.ClusterBlueprint) (*scope.ControlPlaneState, error) {
	var err error
	res := &scope.ControlPlaneState{}

	// Get the control plane object.
	res.Object, err = r.getReference(ctx, cluster.Spec.ControlPlaneRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", tlog.KRef{Ref: cluster.Spec.ControlPlaneRef})
	}

	// If the clusterClass does not mandate the controlPlane has infrastructureMachines, return.
	if !blueprint.HasControlPlaneInfrastructureMachine() {
		return res, nil
	}

	// Otherwise, get the control plane machine infrastructureMachine template.
	machineInfrastructureRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(res.Object)
	if err != nil {
		return res, errors.Wrapf(err, "failed to get InfrastructureMachineTemplate reference for %s", tlog.KObj{Obj: res.Object})
	}
	res.InfrastructureMachineTemplate, err = r.getReference(ctx, machineInfrastructureRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get InfrastructureMachineTemplate for %s", tlog.KObj{Obj: res.Object})
	}

	return res, nil
}

// getCurrentMachineDeploymentState queries for all MachineDeployments and filters them for their linked Cluster and
// whether they are managed by a ClusterClass using labels. A Cluster may have zero or more MachineDeployments. Zero is
// expected on first reconcile. If MachineDeployments are found for the Cluster their Infrastructure and Bootstrap references
// are inspected. Where these are not found the function will throw an error.
func (r *ClusterReconciler) getCurrentMachineDeploymentState(ctx context.Context, cluster *clusterv1.Cluster) (map[string]*scope.MachineDeploymentState, error) {
	state := make(scope.MachineDeploymentsStateMap)

	// List all the machine deployments in the current cluster and in a managed topology.
	md := &clusterv1.MachineDeploymentList{}
	err := r.APIReader.List(ctx, md,
		client.MatchingLabels{
			clusterv1.ClusterLabelName:          cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel: "",
		},
		client.InNamespace(cluster.Namespace),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read MachineDeployments for managed topology")
	}

	// Loop over each machine deployment and create the current
	// state by retrieving all required references.
	for i := range md.Items {
		m := &md.Items[i]

		// Retrieve the name which is assigned in Cluster's topology
		// from a well-defined label.
		mdTopologyName, ok := m.ObjectMeta.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]
		if !ok || len(mdTopologyName) == 0 {
			return nil, fmt.Errorf("failed to find label %s in %s", clusterv1.ClusterTopologyMachineDeploymentLabelName, tlog.KObj{Obj: m})
		}

		// Make sure that the name of the MachineDeployment stays unique.
		// If we've already have seen a MachineDeployment with the same name
		// this is an error, probably caused from manual modifications or a race condition.
		if _, ok := state[mdTopologyName]; ok {
			return nil, fmt.Errorf("duplicate %s found for label %s: %s", tlog.KObj{Obj: m}, clusterv1.ClusterTopologyMachineDeploymentLabelName, mdTopologyName)
		}

		// Gets the BootstrapTemplate
		bootstrapRef := m.Spec.Template.Spec.Bootstrap.ConfigRef
		if bootstrapRef == nil {
			return nil, fmt.Errorf("%s does not have a reference to a Bootstrap Config", tlog.KObj{Obj: m})
		}
		b, err := r.getReference(ctx, bootstrapRef)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("%s Bootstrap reference could not be retrieved", tlog.KObj{Obj: m}))
		}

		// Gets the InfrastructureMachineTemplate
		infraRef := m.Spec.Template.Spec.InfrastructureRef
		if infraRef.Name == "" {
			return nil, fmt.Errorf("%s does not have a reference to a InfrastructureMachineTemplate", tlog.KObj{Obj: m})
		}
		i, err := r.getReference(ctx, &infraRef)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("%s Infrastructure reference could not be retrieved", tlog.KObj{Obj: m}))
		}

		state[mdTopologyName] = &scope.MachineDeploymentState{
			Object:                        m,
			BootstrapTemplate:             b,
			InfrastructureMachineTemplate: i,
		}
	}
	return state, nil
}
