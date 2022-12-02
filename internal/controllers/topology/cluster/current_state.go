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
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	"sigs.k8s.io/cluster-api/util/labels"
)

// getCurrentState gets information about the current state of a Cluster by inspecting the state of the InfrastructureCluster,
// the ControlPlane, and the MachineDeployments associated with the Cluster.
func (r *Reconciler) getCurrentState(ctx context.Context, s *scope.Scope) (*scope.ClusterState, error) {
	// NOTE: current scope has been already initialized with the Cluster.
	currentState := s.Current

	// Reference to the InfrastructureCluster can be nil and is expected to be on the first reconcile.
	// In this case the method should still be allowed to continue.
	if currentState.Cluster.Spec.InfrastructureRef != nil {
		infra, err := r.getCurrentInfrastructureClusterState(ctx, s.Blueprint.InfrastructureClusterTemplate, currentState.Cluster)
		if err != nil {
			return nil, err
		}
		currentState.InfrastructureCluster = infra
	}

	// Reference to the ControlPlane can be nil, and is expected to be on the first reconcile. In this case the method
	// should still be allowed to continue.
	currentState.ControlPlane = &scope.ControlPlaneState{}
	if currentState.Cluster.Spec.ControlPlaneRef != nil {
		cp, err := r.getCurrentControlPlaneState(ctx, s.Blueprint.ControlPlane, s.Blueprint.HasControlPlaneInfrastructureMachine(), currentState.Cluster)
		if err != nil {
			return nil, err
		}
		currentState.ControlPlane = cp
	}

	// A Cluster may have zero or more MachineDeployments and a Cluster is expected to have zero MachineDeployments on
	// first reconcile.
	m, err := r.getCurrentMachineDeploymentState(ctx, s.Blueprint.MachineDeployments, currentState.Cluster)
	if err != nil {
		return nil, err
	}
	currentState.MachineDeployments = m

	return currentState, nil
}

// getCurrentInfrastructureClusterState looks for the state of the InfrastructureCluster. If a reference is set but not
// found, either from an error or the object not being found, an error is thrown.
func (r *Reconciler) getCurrentInfrastructureClusterState(ctx context.Context, blueprintInfrastructureClusterTemplate *unstructured.Unstructured, cluster *clusterv1.Cluster) (*unstructured.Unstructured, error) {
	ref, err := alignRefAPIVersion(blueprintInfrastructureClusterTemplate, cluster.Spec.InfrastructureRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", tlog.KRef{Ref: cluster.Spec.InfrastructureRef})
	}
	infra, err := r.getReference(ctx, ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", tlog.KRef{Ref: cluster.Spec.InfrastructureRef})
	}
	// check that the referenced object has the ClusterTopologyOwnedLabel label.
	// Nb. This is to make sure that a managed topology cluster does not have a reference to an object that is not
	// owned by the topology.
	if !labels.IsTopologyOwned(infra) {
		return nil, fmt.Errorf("infra cluster object %s referenced from cluster %s is not topology owned", tlog.KObj{Obj: infra}, tlog.KObj{Obj: cluster})
	}
	return infra, nil
}

// getCurrentControlPlaneState returns information on the ControlPlane being used by the Cluster. If a reference is not found,
// an error is thrown. If the ControlPlane requires MachineInfrastructure according to its ClusterClass an error will be
// thrown if the ControlPlane has no MachineTemplates.
func (r *Reconciler) getCurrentControlPlaneState(ctx context.Context, blueprintControlPlane *scope.ControlPlaneBlueprint, blueprintHasControlPlaneInfrastructureMachine bool, cluster *clusterv1.Cluster) (*scope.ControlPlaneState, error) {
	var err error
	res := &scope.ControlPlaneState{}

	// Get the control plane object.
	ref, err := alignRefAPIVersion(blueprintControlPlane.Template, cluster.Spec.ControlPlaneRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", tlog.KRef{Ref: cluster.Spec.ControlPlaneRef})
	}
	res.Object, err = r.getReference(ctx, ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", tlog.KRef{Ref: cluster.Spec.ControlPlaneRef})
	}
	// check that the referenced object has the ClusterTopologyOwnedLabel label.
	// Nb. This is to make sure that a managed topology cluster does not have a reference to an object that is not
	// owned by the topology.
	if !labels.IsTopologyOwned(res.Object) {
		return nil, fmt.Errorf("control plane object %s referenced from cluster %s is not topology owned", tlog.KObj{Obj: res.Object}, tlog.KObj{Obj: cluster})
	}

	// If the clusterClass does not mandate the controlPlane has infrastructureMachines, return.
	if !blueprintHasControlPlaneInfrastructureMachine {
		return res, nil
	}

	// Otherwise, get the control plane machine infrastructureMachine template.
	machineInfrastructureRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(res.Object)
	if err != nil {
		return res, errors.Wrapf(err, "failed to get InfrastructureMachineTemplate reference for %s", tlog.KObj{Obj: res.Object})
	}
	ref, err = alignRefAPIVersion(blueprintControlPlane.InfrastructureMachineTemplate, machineInfrastructureRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get InfrastructureMachineTemplate for %s", tlog.KObj{Obj: res.Object})
	}
	res.InfrastructureMachineTemplate, err = r.getReference(ctx, ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get InfrastructureMachineTemplate for %s", tlog.KObj{Obj: res.Object})
	}
	// check that the referenced object has the ClusterTopologyOwnedLabel label.
	// Nb. This is to make sure that a managed topology cluster does not have a reference to an object that is not
	// owned by the topology.
	if !labels.IsTopologyOwned(res.InfrastructureMachineTemplate) {
		return nil, fmt.Errorf("control plane InfrastructureMachineTemplate object %s referenced from cluster %s is not topology owned", tlog.KObj{Obj: res.InfrastructureMachineTemplate}, tlog.KObj{Obj: cluster})
	}

	mhc := &clusterv1.MachineHealthCheck{}
	// MachineHealthCheck always has the same name and namespace as the ControlPlane object it belongs to.
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: res.Object.GetNamespace(), Name: res.Object.GetName()}, mhc); err != nil {
		// Not every ControlPlane will have an associated MachineHealthCheck. If no MachineHealthCheck is found return without error.
		if apierrors.IsNotFound(err) {
			return res, nil
		}
		return nil, errors.Wrapf(err, "failed to get MachineHealthCheck for %s", tlog.KObj{Obj: res.Object})
	}
	res.MachineHealthCheck = mhc
	return res, nil
}

// getCurrentMachineDeploymentState queries for all MachineDeployments and filters them for their linked Cluster and
// whether they are managed by a ClusterClass using labels. A Cluster may have zero or more MachineDeployments. Zero is
// expected on first reconcile. If MachineDeployments are found for the Cluster their Infrastructure and Bootstrap references
// are inspected. Where these are not found the function will throw an error.
func (r *Reconciler) getCurrentMachineDeploymentState(ctx context.Context, blueprintMachineDeployments map[string]*scope.MachineDeploymentBlueprint, cluster *clusterv1.Cluster) (map[string]*scope.MachineDeploymentState, error) {
	state := make(scope.MachineDeploymentsStateMap)

	// List all the machine deployments in the current cluster and in a managed topology.
	md := &clusterv1.MachineDeploymentList{}
	err := r.APIReader.List(ctx, md,
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:          cluster.Name,
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
		mdTopologyName, ok := m.ObjectMeta.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]
		if !ok || mdTopologyName == "" {
			return nil, fmt.Errorf("failed to find label %s in %s", clusterv1.ClusterTopologyMachineDeploymentNameLabel, tlog.KObj{Obj: m})
		}

		// Make sure that the name of the MachineDeployment stays unique.
		// If we've already seen a MachineDeployment with the same name
		// this is an error, probably caused from manual modifications or a race condition.
		if _, ok := state[mdTopologyName]; ok {
			return nil, fmt.Errorf("duplicate %s found for label %s: %s", tlog.KObj{Obj: m}, clusterv1.ClusterTopologyMachineDeploymentNameLabel, mdTopologyName)
		}

		// Gets the bootstrapRef.
		bootstrapRef := m.Spec.Template.Spec.Bootstrap.ConfigRef
		if bootstrapRef == nil {
			return nil, fmt.Errorf("%s does not have a reference to a Bootstrap Config", tlog.KObj{Obj: m})
		}
		// Gets the infraRef.
		infraRef := &m.Spec.Template.Spec.InfrastructureRef
		if infraRef.Name == "" {
			return nil, fmt.Errorf("%s does not have a reference to a InfrastructureMachineTemplate", tlog.KObj{Obj: m})
		}

		// If the mdTopology exists in the Cluster, lookup the corresponding mdBluePrint and align
		// the apiVersions in the bootstrapRef and infraRef.
		// If the mdTopology doesn't exist, do nothing (this can happen if the mdTopology was deleted).
		// **Note** We can't check if the MachineDeployment has a DeletionTimestamp, because at this point it could not be set yet.
		if mdTopologyExistsInCluster, mdClassName := getMDClassName(cluster, mdTopologyName); mdTopologyExistsInCluster {
			mdBluePrint, ok := blueprintMachineDeployments[mdClassName]
			if !ok {
				return nil, fmt.Errorf("failed to find MachineDeployment class %s in ClusterClass", mdClassName)
			}
			bootstrapRef, err = alignRefAPIVersion(mdBluePrint.BootstrapTemplate, bootstrapRef)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("%s Bootstrap reference could not be retrieved", tlog.KObj{Obj: m}))
			}
			infraRef, err = alignRefAPIVersion(mdBluePrint.InfrastructureMachineTemplate, infraRef)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("%s Infrastructure reference could not be retrieved", tlog.KObj{Obj: m}))
			}
		}

		// Get the BootstrapTemplate.
		bootstrapTemplate, err := r.getReference(ctx, bootstrapRef)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("%s Bootstrap reference could not be retrieved", tlog.KObj{Obj: m}))
		}
		// check that the referenced object has the ClusterTopologyOwnedLabel label.
		// Nb. This is to make sure that a managed topology cluster does not have a reference to an object that is not
		// owned by the topology.
		if !labels.IsTopologyOwned(bootstrapTemplate) {
			return nil, fmt.Errorf("BootstrapTemplate object %s referenced from MD %s is not topology owned", tlog.KObj{Obj: bootstrapTemplate}, tlog.KObj{Obj: m})
		}

		// Get the InfraMachineTemplate.
		infraMachineTemplate, err := r.getReference(ctx, infraRef)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("%s Infrastructure reference could not be retrieved", tlog.KObj{Obj: m}))
		}
		// check that the referenced object has the ClusterTopologyOwnedLabel label.
		// Nb. This is to make sure that a managed topology cluster does not have a reference to an object that is not
		// owned by the topology.
		if !labels.IsTopologyOwned(infraMachineTemplate) {
			return nil, fmt.Errorf("InfrastructureMachineTemplate object %s referenced from MD %s is not topology owned", tlog.KObj{Obj: infraMachineTemplate}, tlog.KObj{Obj: m})
		}

		// Gets the MachineHealthCheck.
		mhc := &clusterv1.MachineHealthCheck{}
		// MachineHealthCheck always has the same name and namespace as the MachineDeployment it belongs to.
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name}, mhc); err != nil {
			// reset the machineHealthCheck to nil if there is an error.
			mhc = nil

			// Each MachineDeployment isn't required to have a MachineHealthCheck. Ignore the error if it's of the type not found, but return any other error.
			if !apierrors.IsNotFound(err) {
				return nil, errors.Wrap(err, fmt.Sprintf("failed to get MachineHealthCheck for %s", tlog.KObj{Obj: m}))
			}
		}

		state[mdTopologyName] = &scope.MachineDeploymentState{
			Object:                        m,
			BootstrapTemplate:             bootstrapTemplate,
			InfrastructureMachineTemplate: infraMachineTemplate,
			MachineHealthCheck:            mhc,
		}
	}
	return state, nil
}

// alignRefAPIVersion returns an aligned copy of the currentRef so it matches the apiVersion in ClusterClass.
// This is required so the topology controller can diff current and desired state objects of the same
// version during reconcile.
// If group or kind was changed in the ClusterClass, an exact copy of the currentRef is returned because
// it will end up in a diff and a rollout anyway.
// Only bootstrap template refs in a ClusterClass can change their group and kind.
func alignRefAPIVersion(templateFromClusterClass *unstructured.Unstructured, currentRef *corev1.ObjectReference) (*corev1.ObjectReference, error) {
	currentGV, err := schema.ParseGroupVersion(currentRef.APIVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse apiVersion: %q", currentRef.APIVersion)
	}

	apiVersion := currentRef.APIVersion
	// Use apiVersion from ClusterClass if group and kind is the same.
	if templateFromClusterClass.GroupVersionKind().Group == currentGV.Group &&
		templateFromClusterClass.GetKind() == currentRef.Kind {
		apiVersion = templateFromClusterClass.GetAPIVersion()
	}

	return &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       currentRef.Kind,
		Namespace:  currentRef.Namespace,
		Name:       currentRef.Name,
	}, nil
}

// getMDClassName retrieves the MDClass name by looking up the MDTopology in the Cluster.
func getMDClassName(cluster *clusterv1.Cluster, mdTopologyName string) (bool, string) {
	if cluster.Spec.Topology.Workers == nil {
		return false, ""
	}

	for _, mdTopology := range cluster.Spec.Topology.Workers.MachineDeployments {
		if mdTopology.Name == mdTopologyName {
			return true, mdTopology.Class
		}
	}
	return false, ""
}
