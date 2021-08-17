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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
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

	// Reconcile desired state of the MachineDeployment objects.
	return r.reconcileMachineDeployments(ctx, current, desired)
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
		return errors.Wrapf(err, "failed to create patch helper for %s/%s", current.cluster.GroupVersionKind(), current.cluster.Name)
	}
	if patchHelper.HasChanges() {
		log.Info("updating Cluster")
		if err := patchHelper.Patch(ctx); err != nil {
			return errors.Wrapf(err, "failed to patch %s/%s", current.cluster.GroupVersionKind(), current.cluster.Name)
		}
	}
	return nil
}

// reconcileMachineDeployments reconciles the desired state of the MachineDeployment objects.
func (r *ClusterReconciler) reconcileMachineDeployments(ctx context.Context, current, desired *clusterTopologyState) error {
	diff := calculateMachineDeploymentDiff(current.machineDeployments, desired.machineDeployments)

	// Create MachineDeployments.
	for _, mdTopologyName := range diff.toCreate {
		md := desired.machineDeployments[mdTopologyName]
		if err := r.createMachineDeployment(ctx, md); err != nil {
			return err
		}
	}

	// Update MachineDeployments.
	for _, mdTopologyName := range diff.toUpdate {
		currentMD := current.machineDeployments[mdTopologyName]
		desiredMD := desired.machineDeployments[mdTopologyName]
		if err := r.updateMachineDeployment(ctx, current.cluster.Name, currentMD, desiredMD); err != nil {
			return err
		}
	}

	// Delete MachineDeployments.
	for _, mdTopologyName := range diff.toDelete {
		md := current.machineDeployments[mdTopologyName]
		if err := r.deleteMachineDeployment(ctx, md); err != nil {
			return err
		}
	}

	return nil
}

// createMachineDeployment creates a MachineDeployment and the corresponding Templates.
func (r *ClusterReconciler) createMachineDeployment(ctx context.Context, md machineDeploymentTopologyState) error {
	log := ctrl.LoggerFrom(ctx)

	if _, err := r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
		desired: md.infrastructureMachineTemplate,
	}); err != nil {
		return errors.Wrapf(err, "failed to create %s/%s", md.object.GroupVersionKind(), md.object.Name)
	}

	if _, err := r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
		desired: md.bootstrapTemplate,
	}); err != nil {
		return errors.Wrapf(err, "failed to create %s/%s", md.object.GroupVersionKind(), md.object.Name)
	}

	log.Info("creating", md.object.GroupVersionKind().String(), md.object.GetName())
	if err := r.Client.Create(ctx, md.object.DeepCopy()); err != nil {
		return errors.Wrapf(err, "failed to create %s/%s", md.object.GroupVersionKind(), md.object.Name)
	}
	return nil
}

// updateMachineDeployment updates a MachineDeployment. Also rotates the corresponding Templates if necessary.
func (r *ClusterReconciler) updateMachineDeployment(ctx context.Context, clusterName string, currentMD, desiredMD machineDeploymentTopologyState) error {
	log := ctrl.LoggerFrom(ctx)

	cleanupOldInfrastructureTemplate, err := r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
		ref:     &desiredMD.object.Spec.Template.Spec.InfrastructureRef,
		current: currentMD.infrastructureMachineTemplate,
		desired: desiredMD.infrastructureMachineTemplate,
		templateNamer: func() string {
			return infrastructureMachineTemplateNamePrefix(clusterName, desiredMD.object.Name)
		},
		compatibilityChecker: checkReferencedObjectsAreCompatible,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to update %s/%s", currentMD.object.GroupVersionKind(), currentMD.object.Name)
	}

	cleanupOldBootstrapTemplate, err := r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
		ref:     desiredMD.object.Spec.Template.Spec.Bootstrap.ConfigRef,
		current: currentMD.bootstrapTemplate,
		desired: desiredMD.bootstrapTemplate,
		templateNamer: func() string {
			return bootstrapTemplateNamePrefix(clusterName, desiredMD.object.Name)
		},
		compatibilityChecker: checkReferencedObjectsAreInTheSameNamespace,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to update %s/%s", currentMD.object.GroupVersionKind(), currentMD.object.Name)
	}

	// Check differences between current and desired MachineDeployment, and eventually patch the current object.
	patchHelper, err := newMergePatchHelper(currentMD.object, desiredMD.object, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s/%s", currentMD.object.GroupVersionKind(), currentMD.object.Name)
	}
	if patchHelper.HasChanges() {
		log.Info("updating", currentMD.object.GroupVersionKind().String(), currentMD.object.GetName())
		if err := patchHelper.Patch(ctx); err != nil {
			return errors.Wrapf(err, "failed to update %s/%s", currentMD.object.GroupVersionKind(), currentMD.object.Kind)
		}
	}

	// We want to call both cleanup functions even if one of them fails to clean up as much as possible.
	return kerrors.NewAggregate([]error{cleanupOldInfrastructureTemplate(), cleanupOldBootstrapTemplate()})
}

// deleteMachineDeployment deletes a MachineDeployment.
func (r *ClusterReconciler) deleteMachineDeployment(ctx context.Context, md machineDeploymentTopologyState) error {
	log := ctrl.LoggerFrom(ctx)

	log.Info("deleting", md.object.GroupVersionKind().String(), md.object.GetName())
	if err := r.Client.Delete(ctx, md.object); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete %s/%s", md.object.GroupVersionKind(), md.object.Name)
	}
	return nil
}

type machineDeploymentDiff struct {
	toCreate, toUpdate, toDelete []string
}

// calculateMachineDeploymentDiff compares two maps of machineDeploymentTopologyState and calculates which
// MachineDeployments should be created, updated or deleted.
func calculateMachineDeploymentDiff(current, desired map[string]machineDeploymentTopologyState) machineDeploymentDiff {
	var diff machineDeploymentDiff

	for md := range desired {
		if _, ok := current[md]; ok {
			diff.toUpdate = append(diff.toUpdate, md)
		} else {
			diff.toCreate = append(diff.toCreate, md)
		}
	}

	for md := range current {
		if _, ok := desired[md]; !ok {
			diff.toDelete = append(diff.toDelete, md)
		}
	}

	return diff
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
			return errors.Wrapf(err, "failed to create %s/%s", desired.GroupVersionKind(), desired.GetKind())
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
		return errors.Wrapf(err, "failed to create patch helper for %s/%s", current.GroupVersionKind(), current.GetKind())
	}
	if patchHelper.HasChanges() {
		log.Info("updating", current.GroupVersionKind().String(), current.GetName())
		if err := patchHelper.Patch(ctx); err != nil {
			return errors.Wrapf(err, "failed to patch %s/%s", current.GroupVersionKind(), current.GetKind())
		}
	}
	return nil
}

type reconcileReferencedTemplateInput struct {
	ref                  *corev1.ObjectReference
	current              *unstructured.Unstructured
	desired              *unstructured.Unstructured
	templateNamer        func() string
	compatibilityChecker func(current, desired client.Object) error
}

// reconcileReferencedTemplate reconciles the desired state of a referenced Template.
// NOTE: According to Cluster API operational practices, when a referenced Template changes a template rotation is required:
// 1. create a new Template
// 2. update the reference
// 3. delete the old Template
// This function specifically takes care of the first step and updates the reference locally. So the remaining steps
// can be executed afterwards.
func (r *ClusterReconciler) reconcileReferencedTemplate(ctx context.Context, in reconcileReferencedTemplateInput) (func() error, error) {
	log := ctrl.LoggerFrom(ctx)
	cleanupFunc := func() error { return nil }

	// If there is no current object, create the desired object.
	if in.current == nil {
		log.Info("creating", in.desired.GroupVersionKind().String(), in.desired.GetName())
		if err := r.Client.Create(ctx, in.desired.DeepCopy()); err != nil {
			return nil, errors.Wrapf(err, "failed to create %s/%s", in.desired.GroupVersionKind(), in.desired.GetName())
		}
		return cleanupFunc, nil
	}

	if in.ref == nil {
		return nil, errors.Errorf("failed to rotate %s: ref should not be nil", in.desired.GroupVersionKind())
	}

	// Check if the current and desired referenced object are compatible.
	if err := in.compatibilityChecker(in.current, in.desired); err != nil {
		return nil, err
	}

	// Check differences between current and desired objects, and if there are changes eventually start the template rotation.
	patchHelper, err := newMergePatchHelper(in.current, in.desired, r.Client)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create patch helper for %s/%s", in.current.GroupVersionKind(), in.current.GetName())
	}

	if patchHelper.HasChanges() {
		// Create the new template.

		// NOTE: it is required to assign a new name, because during compute the desired object name is enforced to be equal to the current one.
		newName := in.templateNamer()
		in.desired.SetName(newName)

		log.Info(fmt.Sprintf("rotating %s from %s to %s", in.desired.GroupVersionKind(), in.current.GetName(), newName))

		log.Info("creating", in.desired.GroupVersionKind().String(), in.desired.GetName())
		if err := r.Client.Create(ctx, in.desired.DeepCopy()); err != nil {
			return nil, errors.Wrapf(err, "failed to create %s/%s", in.desired.GroupVersionKind(), in.desired.GetName())
		}

		// Update the reference with the new name.
		// NOTE: Updating the object hosting reference to the template is executed outside this func.
		in.ref.Name = newName

		// Set up a cleanup func for removing the old template.
		// NOTE: This function must be called after updating the object containing the reference to the Template.
		cleanupFunc = func() error {
			log.Info("deleting", in.desired.GroupVersionKind().String(), in.desired.GetName())
			if err := r.Client.Delete(ctx, in.current); err != nil {
				return errors.Wrapf(err, "failed to delete %s/%s", in.desired.GroupVersionKind(), in.desired.GetName())
			}
			return nil
		}
	}
	return cleanupFunc, nil
}

// checkReferencedObjectsAreStrictlyCompatible checks if two referenced objects are strictly compatible, meaning that
// they are compatible and the name of the objects do not change.
func checkReferencedObjectsAreStrictlyCompatible(current, desired client.Object) error {
	if current.GetName() != desired.GetName() {
		return errors.Errorf("invalid operation: it is not possible to change the name of %s/%s from %s to %s",
			current.GetObjectKind().GroupVersionKind(), current.GetName(), current.GetName(), desired.GetName())
	}
	return checkReferencedObjectsAreCompatible(current, desired)
}

// checkReferencedObjectsAreCompatible checks if two referenced objects are compatible, meaning that
// they are of the same GroupKind and in the same namespace.
func checkReferencedObjectsAreCompatible(current, desired client.Object) error {
	currentGK := current.GetObjectKind().GroupVersionKind().GroupKind()
	desiredGK := desired.GetObjectKind().GroupVersionKind().GroupKind()

	if currentGK.String() != desiredGK.String() {
		return errors.Errorf("invalid operation: it is not possible to change the GroupKind of %s/%s from %s to %s",
			current.GetObjectKind().GroupVersionKind(), current.GetName(), currentGK, desiredGK)
	}
	return checkReferencedObjectsAreInTheSameNamespace(current, desired)
}

// checkReferencedObjectsAreInTheSameNamespace checks if two referenced objects are in the same namespace.
func checkReferencedObjectsAreInTheSameNamespace(current, desired client.Object) error {
	// NOTE: this should never happen (webhooks prevent it), but checking for extra safety.
	if current.GetNamespace() != desired.GetNamespace() {
		return errors.Errorf("invalid operation: it is not possible to change the namespace of %s/%s from %s to %s",
			current.GetObjectKind().GroupVersionKind(), current.GetName(), current.GetNamespace(), desired.GetNamespace())
	}
	return nil
}
