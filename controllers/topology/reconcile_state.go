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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reconcileState reconciles the current and desired state of the managed Cluster topology.
// NOTE: We are assuming all the required objects are provided as input; also, in case of any error,
// the entire reconcile operation will fail. This might be improved in the future if support for reconciling
// subset of a topology will be implemented.
func (r *ClusterReconciler) reconcileState(ctx context.Context, current, desired *clusterTopologyState) error {
	// Reconcile desired state of the InfrastructureCluster object.
	if err := r.reconcileInfrastructureCluster(ctx, current, desired); err != nil {
		return err
	}

	// TODO: reconcile control plane

	// Reconcile desired state of the InfrastructureCluster object.
	if err := r.reconcileCluster(ctx, current, desired); err != nil {
		return err
	}

	// TODO: reconcile machine deployments

	return nil
}

// reconcileInfrastructureCluster reconciles the desired state of the InfrastructureCluster object.
func (r *ClusterReconciler) reconcileInfrastructureCluster(ctx context.Context, current, desired *clusterTopologyState) error {
	return r.reconcileReferencedObject(ctx, current.infrastructureCluster, desired.infrastructureCluster)
}

// reconcileCluster reconciles the desired state of the Cluster object.
// NOTE: this assumes reconcileInfrastructureCluster and reconcileControlPlane being already completed;
// most specifically, after a Cluster is created it is assumed that the reference to the InfrastructureCluster /
// ControlPlane objects should never change (only the content of the objects can change).
func (r *ClusterReconciler) reconcileCluster(ctx context.Context, current, desired *clusterTopologyState) error {
	log := ctrl.LoggerFrom(ctx)

	// Check differences between current and desired state, and eventually patch the current object.
	patchHelper, err := newMergePatchHelper(current.cluster, desired.cluster, r.Client)
	if err != nil {
		return errors.Wrap(err, "failed to create patch helper for the Cluster object")
	}
	if patchHelper.HasChanges() {
		log.Info("updating Cluster")
		if err := patchHelper.Patch(ctx); err != nil {
			return errors.Wrap(err, "failed to patch the Cluster object")
		}
	}
	return nil
}

// reconcileReferencedObject reconciles the desired state of the referenced object.
// NOTE: After a referenced object is created it is assumed that the reference should
// never change (only the content of the object can eventually change). Thus, we are checking for strict compatibility.
func (r *ClusterReconciler) reconcileReferencedObject(ctx context.Context, current, desired *unstructured.Unstructured) error {
	log := ctrl.LoggerFrom(ctx)

	// If there is no current object, create it.
	if current == nil {
		log.Info("creating", desired.GroupVersionKind().String(), desired.GetName())
		if err := r.Client.Create(ctx, desired.DeepCopy()); err != nil {
			return errors.Wrapf(err, "failed to create the %s object", desired.GetKind())
		}
		return nil
	}

	// Check if the current and desired referenced object are compatible.
	if err := checkReferencedObjectsAreStrictlyCompatible(current, desired); err != nil {
		return err
	}

	// Check differences between current and desired state, and eventually patch the current object.
	patchHelper, err := newMergePatchHelper(current, desired, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for the %s object", current.GetKind())
	}
	if patchHelper.HasChanges() {
		log.Info("updating", current.GroupVersionKind().String(), current.GetName())
		if err := patchHelper.Patch(ctx); err != nil {
			return errors.Wrapf(err, "failed to patch the %s object", current.GetKind())
		}
	}
	return nil
}

// checkReferencedObjectsAreCompatible check if two referenced objects are compatible, meaning that
// they are of the same GroupKind.
func checkReferencedObjectsAreCompatible(current, desired client.Object) error {
	currentGK := current.GetObjectKind().GroupVersionKind().GroupKind()
	desiredGK := desired.GetObjectKind().GroupVersionKind().GroupKind()

	if currentGK.String() != desiredGK.String() {
		return errors.Errorf("invalid operation: it is not possible to change GroupKind from %s to %s", currentGK.String(), desiredGK.String())
	}

	// NOTE: this should never happen (webhooks prevent it), but checking for extra safety.
	if current.GetNamespace() != desired.GetNamespace() {
		return errors.Errorf("invalid operation: it is not possible to change namespace from %s to %s", current.GetNamespace(), desired.GetNamespace())
	}

	return nil
}

// checkReferencedObjectsAreStrictlyCompatible check if two referenced objects are strictly compatible, meaning that
// they are compatible and the name of the objects does not change.
func checkReferencedObjectsAreStrictlyCompatible(current, desired client.Object) error {
	if current.GetName() != desired.GetName() {
		return errors.Errorf("invalid operation: it is not possible to change names from %s to %s", current.GetName(), desired.GetName())
	}
	return checkReferencedObjectsAreCompatible(current, desired)
}
