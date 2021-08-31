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
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/check"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	tlog "sigs.k8s.io/cluster-api/controllers/topology/internal/log"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/mergepatch"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reconcileState reconciles the current and desired state of the managed Cluster topology.
// NOTE: We are assuming all the required objects are provided as input; also, in case of any error,
// the entire reconcile operation will fail. This might be improved in the future if support for reconciling
// subset of a topology will be implemented.
func (r *ClusterReconciler) reconcileState(ctx context.Context, s *scope.Scope) error {
	log := tlog.LoggerFrom(ctx)
	log.Infof("Reconciling state for topology owned objects")

	// Reconcile desired state of the InfrastructureCluster object.
	if err := r.reconcileInfrastructureCluster(ctx, s); err != nil {
		return err
	}

	// Reconcile desired state of the ControlPlane object.
	if err := r.reconcileControlPlane(ctx, s); err != nil {
		return err
	}

	// Reconcile desired state of the Cluster object.
	if err := r.reconcileCluster(ctx, s); err != nil {
		return err
	}

	// Reconcile desired state of the MachineDeployment objects.
	return r.reconcileMachineDeployments(ctx, s)
}

// reconcileInfrastructureCluster reconciles the desired state of the InfrastructureCluster object.
func (r *ClusterReconciler) reconcileInfrastructureCluster(ctx context.Context, s *scope.Scope) error {
	ctx, _ = tlog.LoggerFrom(ctx).WithObject(s.Desired.InfrastructureCluster).Into(ctx)
	return r.reconcileReferencedObject(ctx, s.Current.InfrastructureCluster, s.Desired.InfrastructureCluster, mergepatch.IgnorePaths(contract.InfrastructureCluster().IgnorePaths()))
}

// reconcileControlPlane works to bring the current state of a managed topology in line with the desired state. This involves
// updating the cluster where needed.
func (r *ClusterReconciler) reconcileControlPlane(ctx context.Context, s *scope.Scope) error {
	// Set a default nil return function for the cleanup operation.
	cleanup := func() error { return nil }

	// If the clusterClass mandates the controlPlane has infrastructureMachines, reconcile it.
	if s.Blueprint.HasControlPlaneInfrastructureMachine() {
		ctx, _ := tlog.LoggerFrom(ctx).WithObject(s.Desired.ControlPlane.InfrastructureMachineTemplate).Into(ctx)

		cpInfraRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(s.Desired.ControlPlane.Object)
		if err != nil {
			return errors.Wrapf(err, "failed to update %s", tlog.KObj{Obj: s.Desired.ControlPlane.InfrastructureMachineTemplate})
		}

		// Create or update the MachineInfrastructureTemplate of the control plane.
		cleanup, err = r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
			ref:                  cpInfraRef,
			current:              s.Current.ControlPlane.InfrastructureMachineTemplate,
			desired:              s.Desired.ControlPlane.InfrastructureMachineTemplate,
			compatibilityChecker: check.ReferencedObjectsAreCompatible,
			templateNamePrefix:   controlPlaneInfrastructureMachineTemplateNamePrefix(s.Current.Cluster.Name),
		},
		)
		if err != nil {
			return errors.Wrapf(err, "failed to update %s", tlog.KObj{Obj: s.Desired.ControlPlane.InfrastructureMachineTemplate})
		}

		// The controlPlaneObject.Spec.machineTemplate.infrastructureRef has to be updated in the desired object
		err = contract.ControlPlane().MachineTemplate().InfrastructureRef().Set(s.Desired.ControlPlane.Object, refToUnstructured(cpInfraRef))
		if err != nil {
			return kerrors.NewAggregate([]error{
				errors.Wrapf(err, "failed to update %s", tlog.KObj{Obj: s.Desired.ControlPlane.Object}),
				cleanup(),
			})
		}
	}

	// Create or update the ControlPlaneObject for the ControlPlaneState.
	ctx, _ = tlog.LoggerFrom(ctx).WithObject(s.Desired.ControlPlane.Object).Into(ctx)
	if err := r.reconcileReferencedObject(ctx, s.Current.ControlPlane.Object, s.Desired.ControlPlane.Object); err != nil {
		return kerrors.NewAggregate([]error{
			errors.Wrapf(err, "failed to update %s", tlog.KObj{Obj: s.Desired.ControlPlane.Object}),
			cleanup(),
		})
	}

	// At this point we've updated the ControlPlane object and, where required, the ControlPlane InfrastructureMachineTemplate
	// without error. Run the cleanup in order to delete the old InfrastructureMachineTemplate if template rotation was done during update.
	return cleanup()
}

// reconcileCluster reconciles the desired state of the Cluster object.
// NOTE: this assumes reconcileInfrastructureCluster and reconcileControlPlane being already completed;
// most specifically, after a Cluster is created it is assumed that the reference to the InfrastructureCluster /
// ControlPlane objects should never change (only the content of the objects can change).
func (r *ClusterReconciler) reconcileCluster(ctx context.Context, s *scope.Scope) error {
	ctx, log := tlog.LoggerFrom(ctx).WithObject(s.Desired.Cluster).Into(ctx)

	// Check differences between current and desired state, and eventually patch the current object.
	patchHelper, err := mergepatch.NewHelper(s.Current.Cluster, s.Desired.Cluster, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: s.Current.Cluster})
	}
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: s.Current.Cluster})
		return nil
	}

	log.Infof("Patching %s", tlog.KObj{Obj: s.Current.Cluster})
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: s.Current.Cluster})
	}
	return nil
}

// reconcileMachineDeployments reconciles the desired state of the MachineDeployment objects.
func (r *ClusterReconciler) reconcileMachineDeployments(ctx context.Context, s *scope.Scope) error {
	diff := calculateMachineDeploymentDiff(s.Current.MachineDeployments, s.Desired.MachineDeployments)

	// Create MachineDeployments.
	for _, mdTopologyName := range diff.toCreate {
		md := s.Desired.MachineDeployments[mdTopologyName]
		if err := r.createMachineDeployment(ctx, md); err != nil {
			return err
		}
	}

	// Update MachineDeployments.
	for _, mdTopologyName := range diff.toUpdate {
		currentMD := s.Current.MachineDeployments[mdTopologyName]
		desiredMD := s.Desired.MachineDeployments[mdTopologyName]
		if err := r.updateMachineDeployment(ctx, s.Current.Cluster.Name, mdTopologyName, currentMD, desiredMD); err != nil {
			return err
		}
	}

	// Delete MachineDeployments.
	for _, mdTopologyName := range diff.toDelete {
		md := s.Current.MachineDeployments[mdTopologyName]
		if err := r.deleteMachineDeployment(ctx, md); err != nil {
			return err
		}
	}

	return nil
}

// createMachineDeployment creates a MachineDeployment and the corresponding Templates.
func (r *ClusterReconciler) createMachineDeployment(ctx context.Context, md *scope.MachineDeploymentState) error {
	log := tlog.LoggerFrom(ctx).WithMachineDeployment(md.Object)

	ctx, _ = log.WithObject(md.InfrastructureMachineTemplate).Into(ctx)
	if _, err := r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
		desired: md.InfrastructureMachineTemplate,
	}); err != nil {
		return errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: md.Object})
	}

	ctx, _ = log.WithObject(md.BootstrapTemplate).Into(ctx)
	if _, err := r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
		desired: md.BootstrapTemplate,
	}); err != nil {
		return errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: md.Object})
	}

	log = log.WithObject(md.Object)
	log.Infof(fmt.Sprintf("Creating %s", tlog.KObj{Obj: md.Object}))
	if err := r.Client.Create(ctx, md.Object.DeepCopy()); err != nil {
		return errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: md.Object})
	}
	return nil
}

// updateMachineDeployment updates a MachineDeployment. Also rotates the corresponding Templates if necessary.
func (r *ClusterReconciler) updateMachineDeployment(ctx context.Context, clusterName string, mdTopologyName string, currentMD, desiredMD *scope.MachineDeploymentState) error {
	log := tlog.LoggerFrom(ctx).WithMachineDeployment(desiredMD.Object)

	ctx, _ = log.WithObject(desiredMD.InfrastructureMachineTemplate).Into(ctx)
	if _, err := r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
		ref:                  &desiredMD.Object.Spec.Template.Spec.InfrastructureRef,
		current:              currentMD.InfrastructureMachineTemplate,
		desired:              desiredMD.InfrastructureMachineTemplate,
		templateNamePrefix:   infrastructureMachineTemplateNamePrefix(clusterName, mdTopologyName),
		compatibilityChecker: check.ReferencedObjectsAreCompatible,
	}); err != nil {
		return errors.Wrapf(err, "failed to update %s", tlog.KObj{Obj: currentMD.Object})
	}

	ctx, _ = log.WithObject(desiredMD.BootstrapTemplate).Into(ctx)
	if _, err := r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
		ref:                  desiredMD.Object.Spec.Template.Spec.Bootstrap.ConfigRef,
		current:              currentMD.BootstrapTemplate,
		desired:              desiredMD.BootstrapTemplate,
		templateNamePrefix:   bootstrapTemplateNamePrefix(clusterName, mdTopologyName),
		compatibilityChecker: check.ObjectsAreInTheSameNamespace,
	}); err != nil {
		return errors.Wrapf(err, "failed to update %s", tlog.KObj{Obj: currentMD.Object})
	}

	// Check differences between current and desired MachineDeployment, and eventually patch the current object.
	log = log.WithObject(desiredMD.Object)
	patchHelper, err := mergepatch.NewHelper(currentMD.Object, desiredMD.Object, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: currentMD.Object})
	}
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: currentMD.Object})
		return nil
	}

	log.Infof("Patching %s", tlog.KObj{Obj: currentMD.Object})
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: currentMD.Object})
	}

	// We want to call both cleanup functions even if one of them fails to clean up as much as possible.
	return nil
}

// deleteMachineDeployment deletes a MachineDeployment.
func (r *ClusterReconciler) deleteMachineDeployment(ctx context.Context, md *scope.MachineDeploymentState) error {
	log := tlog.LoggerFrom(ctx).WithMachineDeployment(md.Object).WithObject(md.Object)

	log.Infof("Deleting %s", tlog.KObj{Obj: md.Object})
	if err := r.Client.Delete(ctx, md.Object); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete %s", tlog.KObj{Obj: md.Object})
	}
	return nil
}

type machineDeploymentDiff struct {
	toCreate, toUpdate, toDelete []string
}

// calculateMachineDeploymentDiff compares two maps of MachineDeploymentState and calculates which
// MachineDeployments should be created, updated or deleted.
func calculateMachineDeploymentDiff(current, desired map[string]*scope.MachineDeploymentState) machineDeploymentDiff {
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
func (r *ClusterReconciler) reconcileReferencedObject(ctx context.Context, current, desired *unstructured.Unstructured, opts ...mergepatch.HelperOption) error {
	log := tlog.LoggerFrom(ctx)

	// If there is no current object, create it.
	if current == nil {
		log.Infof("Creating %s", tlog.KObj{Obj: desired})
		if err := r.Client.Create(ctx, desired.DeepCopy()); err != nil {
			return errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: desired})
		}
		return nil
	}

	// Check if the current and desired referenced object are compatible.
	if err := check.ReferencedObjectsAreStrictlyCompatible(current, desired); err != nil {
		return err
	}

	// Check differences between current and desired state, and eventually patch the current object.
	patchHelper, err := mergepatch.NewHelper(current, desired, r.Client, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: current})
	}
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: desired})
		return nil
	}

	log.Infof("Patching %s", tlog.KObj{Obj: desired})
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: current})
	}
	return nil
}

type reconcileReferencedTemplateInput struct {
	ref                  *corev1.ObjectReference
	current              *unstructured.Unstructured
	desired              *unstructured.Unstructured
	templateNamePrefix   string
	compatibilityChecker func(current, desired client.Object) error
}

// reconcileReferencedTemplate reconciles the desired state of a referenced Template.
// NOTE: According to Cluster API operational practices, when a referenced Template changes a template rotation is required:
// 1. create a new Template
// 2. update the reference
// 3. delete the old Template
// This function specifically takes care of the first step and updates the reference locally. So the remaining steps
// can be executed afterwards.
// NOTE: This func has a side effect in case of template rotation, changing both the desired object and the object reference.
func (r *ClusterReconciler) reconcileReferencedTemplate(ctx context.Context, in reconcileReferencedTemplateInput) (func() error, error) {
	log := tlog.LoggerFrom(ctx)

	cleanupFunc := func() error { return nil }

	// If there is no current object, create the desired object.
	if in.current == nil {
		log.Infof("Creating %s", tlog.KObj{Obj: in.desired})
		if err := r.Client.Create(ctx, in.desired.DeepCopy()); err != nil {
			return nil, errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: in.desired})
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
	patchHelper, err := mergepatch.NewHelper(in.current, in.desired, r.Client)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: in.current})
	}

	// Return if no changes are detected.
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: in.desired})
		return cleanupFunc, nil
	}

	// Create the new template.

	// NOTE: it is required to assign a new name, because during compute the desired object name is enforced to be equal to the current one.
	// TODO: find a way to make side effect more explicit
	newName := names.SimpleNameGenerator.GenerateName(in.templateNamePrefix)
	in.desired.SetName(newName)

	log.Infof("Rotating %s, new name %s", tlog.KObj{Obj: in.current}, newName)
	log.Infof("Creating %s", tlog.KObj{Obj: in.desired})
	if err := r.Client.Create(ctx, in.desired.DeepCopy()); err != nil {
		return nil, errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: in.desired})
	}

	// Update the reference with the new name.
	// NOTE: Updating the object hosting reference to the template is executed outside this func.
	// TODO: find a way to make side effect more explicit
	in.ref.Name = newName

	// Set up a cleanup func for removing the old template.
	// NOTE: This function must be called after updating the object containing the reference to the Template.
	return func() error {
		log.Infof("Deleting %s", tlog.KObj{Obj: in.current})
		if err := r.Client.Delete(ctx, in.current); err != nil {
			return errors.Wrapf(err, "failed to delete %s", tlog.KObj{Obj: in.desired})
		}
		return nil
	}, nil
}
