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
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/mergepatch"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	"sigs.k8s.io/cluster-api/internal/topology/check"
)

const (
	createEventReason = "TopologyCreate"
	updateEventReason = "TopologyUpdate"
	deleteEventReason = "TopologyDelete"
)

// reconcileState reconciles the current and desired state of the managed Cluster topology.
// NOTE: We are assuming all the required objects are provided as input; also, in case of any error,
// the entire reconcile operation will fail. This might be improved in the future if support for reconciling
// subset of a topology will be implemented.
func (r *Reconciler) reconcileState(ctx context.Context, s *scope.Scope) error {
	log := tlog.LoggerFrom(ctx)
	log.Infof("Reconciling state for topology owned objects")

	// Reconcile the Cluster shim, a temporary object used a mean to collect
	// objects/templates that can be orphaned in case of errors during the
	// remaining part of the reconcile process.
	if err := r.reconcileClusterShim(ctx, s); err != nil {
		return err
	}

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

// Reconcile the Cluster shim, a temporary object used a mean to collect objects/templates
// that might be orphaned in case of errors during the remaining part of the reconcile process.
func (r *Reconciler) reconcileClusterShim(ctx context.Context, s *scope.Scope) error {
	shim := clusterShim(s.Current.Cluster)

	// If we are going to create the InfrastructureCluster or the ControlPlane object, then
	// add a temporary cluster-shim object and use it as an additional owner.
	// This will ensure the objects will be garbage collected in case of errors in between
	// creating InfrastructureCluster/ControlPlane objects and updating the Cluster with the
	// references to above objects.
	if s.Current.InfrastructureCluster == nil || s.Current.ControlPlane.Object == nil {
		if err := r.Client.Create(ctx, shim); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "failed to create the cluster shim object")
			}
			if err := r.Client.Get(ctx, client.ObjectKeyFromObject(shim), shim); err != nil {
				return errors.Wrapf(err, "failed to read the cluster shim object")
			}
		}
		// Enforce type meta back given that it gets blanked out by Get.
		shim.Kind = "Secret"
		shim.APIVersion = corev1.SchemeGroupVersion.String()

		// Add the shim as a temporary owner for the InfrastructureCluster.
		ownerRefs := s.Desired.InfrastructureCluster.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(shim))
		s.Desired.InfrastructureCluster.SetOwnerReferences(ownerRefs)

		// Add the shim as a temporary owner for the ControlPlane.
		ownerRefs = s.Desired.ControlPlane.Object.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(shim))
		s.Desired.ControlPlane.Object.SetOwnerReferences(ownerRefs)
	}

	// If the InfrastructureCluster and the ControlPlane objects have been already created
	// in previous reconciliation, check if they have already been reconciled by the ClusterController
	// by verifying the ownerReference for the Cluster is present.
	//
	// When the Cluster and the shim object are both owners,
	// it's safe for us to remove the shim and garbage collect any potential orphaned resource.
	if s.Current.InfrastructureCluster != nil && s.Current.ControlPlane.Object != nil {
		clusterOwnsAll := hasOwnerReferenceFrom(s.Current.InfrastructureCluster, s.Current.Cluster) &&
			hasOwnerReferenceFrom(s.Current.ControlPlane.Object, s.Current.Cluster)
		shimOwnsAtLeastOne := hasOwnerReferenceFrom(s.Current.InfrastructureCluster, shim) ||
			hasOwnerReferenceFrom(s.Current.ControlPlane.Object, shim)

		if clusterOwnsAll && shimOwnsAtLeastOne {
			if err := r.Client.Delete(ctx, shim); err != nil {
				if !apierrors.IsNotFound(err) {
					return errors.Wrapf(err, "failed to delete the cluster shim object")
				}
			}
		}
	}
	return nil
}

func clusterShim(c *clusterv1.Cluster) *corev1.Secret {
	shim := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-shim", c.Name),
			Namespace: c.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*ownerReferenceTo(c),
			},
		},
		Type: clusterv1.ClusterSecretType,
	}
	return shim
}

func hasOwnerReferenceFrom(obj, owner client.Object) bool {
	for _, o := range obj.GetOwnerReferences() {
		if o.Kind == owner.GetObjectKind().GroupVersionKind().Kind && o.Name == owner.GetName() {
			return true
		}
	}
	return false
}

// reconcileInfrastructureCluster reconciles the desired state of the InfrastructureCluster object.
func (r *Reconciler) reconcileInfrastructureCluster(ctx context.Context, s *scope.Scope) error {
	ctx, _ = tlog.LoggerFrom(ctx).WithObject(s.Desired.InfrastructureCluster).Into(ctx)
	return r.reconcileReferencedObject(ctx, reconcileReferencedObjectInput{
		cluster: s.Current.Cluster,
		current: s.Current.InfrastructureCluster,
		desired: s.Desired.InfrastructureCluster,
		opts: []mergepatch.HelperOption{
			mergepatch.IgnorePaths(contract.InfrastructureCluster().IgnorePaths()),
		},
	})
}

// reconcileControlPlane works to bring the current state of a managed topology in line with the desired state. This involves
// updating the cluster where needed.
func (r *Reconciler) reconcileControlPlane(ctx context.Context, s *scope.Scope) error {
	// If the clusterClass mandates the controlPlane has infrastructureMachines, reconcile it.
	if s.Blueprint.HasControlPlaneInfrastructureMachine() {
		ctx, _ := tlog.LoggerFrom(ctx).WithObject(s.Desired.ControlPlane.InfrastructureMachineTemplate).Into(ctx)

		cpInfraRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(s.Desired.ControlPlane.Object)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile %s", tlog.KObj{Obj: s.Desired.ControlPlane.InfrastructureMachineTemplate})
		}

		// Create or update the MachineInfrastructureTemplate of the control plane.
		if err = r.reconcileReferencedTemplate(ctx, reconcileReferencedTemplateInput{
			cluster:              s.Current.Cluster,
			ref:                  cpInfraRef,
			current:              s.Current.ControlPlane.InfrastructureMachineTemplate,
			desired:              s.Desired.ControlPlane.InfrastructureMachineTemplate,
			compatibilityChecker: check.ObjectsAreCompatible,
			templateNamePrefix:   controlPlaneInfrastructureMachineTemplateNamePrefix(s.Current.Cluster.Name),
		},
		); err != nil {
			return err
		}

		// The controlPlaneObject.Spec.machineTemplate.infrastructureRef has to be updated in the desired object
		err = contract.ControlPlane().MachineTemplate().InfrastructureRef().Set(s.Desired.ControlPlane.Object, refToUnstructured(cpInfraRef))
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile %s", tlog.KObj{Obj: s.Desired.ControlPlane.Object})
		}
	}

	// Create or update the ControlPlaneObject for the ControlPlaneState.
	ctx, _ = tlog.LoggerFrom(ctx).WithObject(s.Desired.ControlPlane.Object).Into(ctx)
	if err := r.reconcileReferencedObject(ctx, reconcileReferencedObjectInput{
		cluster:       s.Current.Cluster,
		current:       s.Current.ControlPlane.Object,
		desired:       s.Desired.ControlPlane.Object,
		versionGetter: contract.ControlPlane().Version().Get,
		opts: []mergepatch.HelperOption{
			mergepatch.AuthoritativePaths{
				// Note: we want to be authoritative WRT machine's metadata labels and annotations.
				// This has the nice benefit that it greatly simplify the UX around ControlPlaneClass.Metadata and
				// ControlPlaneTopology.Metadata, given that changes are reflected into generated objects without
				// accounting for instance specific changes like we do for other maps into spec.
				// Note: nested metadata have only labels and annotations, so it is possible to override the entire
				// parent struct.
				contract.ControlPlane().MachineTemplate().Metadata().Path(),
			},
		},
	}); err != nil {
		return err
	}

	// If the ControlPlane has defined a current or desired MachineHealthCheck attempt to reconcile it.
	if s.Desired.ControlPlane.MachineHealthCheck != nil || s.Current.ControlPlane.MachineHealthCheck != nil {
		// Set the ControlPlane Object and the Cluster as owners for the MachineHealthCheck to ensure object garbage collection
		// in case something happens before the MHC sets ownership to the Cluster.
		if s.Desired.ControlPlane.MachineHealthCheck != nil {
			s.Desired.ControlPlane.MachineHealthCheck.SetOwnerReferences([]metav1.OwnerReference{
				*ownerReferenceTo(s.Desired.ControlPlane.Object),
			})
		}

		// Reconcile the current and desired state of the MachineHealthCheck.
		if err := r.reconcileMachineHealthCheck(ctx, s.Current.ControlPlane.MachineHealthCheck, s.Desired.ControlPlane.MachineHealthCheck); err != nil {
			return err
		}
	}
	return nil
}

// reconcileMachineHealthCheck creates, updates, deletes or leaves untouched a MachineHealthCheck depending on the difference between the
// current state and the desired state.
func (r *Reconciler) reconcileMachineHealthCheck(ctx context.Context, current, desired *clusterv1.MachineHealthCheck) error {
	log := tlog.LoggerFrom(ctx)

	// If a current MachineHealthCheck doesn't exist but there is a desired MachineHealthCheck attempt to create.
	if current == nil && desired != nil {
		// First ensure the ownerReferences are valid.
		refs := []metav1.OwnerReference{}
		for _, ref := range desired.OwnerReferences {
			currentRef := ref
			ownerRef, err := r.resolveOwnerReferenceIfIncomplete(ctx, desired.Namespace, &currentRef)
			if err != nil {
				return errors.Wrapf(err, "failed to resolve owner reference %v for %s", ref, tlog.KObj{Obj: desired})
			}
			refs = append(refs, *ownerRef)
		}
		desired.OwnerReferences = refs

		log.Infof("Creating %s", tlog.KObj{Obj: desired})
		if err := r.Client.Create(ctx, desired); err != nil {
			return errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: desired})
		}
		r.recorder.Eventf(desired, corev1.EventTypeNormal, createEventReason, "Created %q", tlog.KObj{Obj: desired})
		return nil
	}

	// If a current MachineHealthCheck exists but there is no desired MachineHealthCheck attempt to delete.
	if current != nil && desired == nil {
		log.Infof("Deleting %s", tlog.KObj{Obj: current})
		if err := r.Client.Delete(ctx, current); err != nil {
			// If the object to be deleted is not found don't throw an error.
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete %s", tlog.KObj{Obj: current})
			}
		}
		r.recorder.Eventf(current, corev1.EventTypeNormal, deleteEventReason, "Deleted %q", tlog.KObj{Obj: current})
		return nil
	}

	ctx, log = log.WithObject(current).Into(ctx)

	// Check differences between current and desired MachineHealthChecks, and patch if required.
	// NOTE: we want to be authoritative on the entire spec because the users are
	// expected to change MHC fields from the ClusterClass only.
	patchHelper, err := mergepatch.NewHelper(current, desired, r.Client, mergepatch.AuthoritativePaths{contract.Path{"spec"}})
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: current})
	}
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: current})
		return nil
	}

	log.Infof("Patching %s", tlog.KObj{Obj: current})
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: current})
	}
	r.recorder.Eventf(current, corev1.EventTypeNormal, updateEventReason, "Updated %q", tlog.KObj{Obj: current})
	return nil
}

// resolveOwnerReferenceIfIncomplete checks if an OwnerReferences is valid. If not it attempts to get the referenced object
// to create a valid ownerReference.
func (r *Reconciler) resolveOwnerReferenceIfIncomplete(ctx context.Context, namespace string, reference *metav1.OwnerReference) (*metav1.OwnerReference, error) {
	var owner *unstructured.Unstructured
	var err error

	// First check that the ownerReference has at least Name Kind and APIVersion defined.
	if reference.Name == "" || reference.APIVersion == "" || reference.Kind == "" {
		return nil, errors.Errorf("ownerReference not valid: %v", reference)
	}

	// If the UID field is not empty this is a fully valid reference and can be returned and used.
	if reference.UID != "" {
		return reference, nil
	}

	// Get the object set an ownerReference for the MachineHealthCheck.
	owner, err = external.Get(ctx, r.Client, &corev1.ObjectReference{
		Kind:       reference.Kind,
		Name:       reference.Name,
		APIVersion: reference.APIVersion},
		namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s %q in namespace %q", reference.Kind, reference.Name, namespace)
	}
	return ownerReferenceTo(owner), nil
}

// reconcileCluster reconciles the desired state of the Cluster object.
// NOTE: this assumes reconcileInfrastructureCluster and reconcileControlPlane being already completed;
// most specifically, after a Cluster is created it is assumed that the reference to the InfrastructureCluster /
// ControlPlane objects should never change (only the content of the objects can change).
func (r *Reconciler) reconcileCluster(ctx context.Context, s *scope.Scope) error {
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
	r.recorder.Eventf(s.Current.Cluster, corev1.EventTypeNormal, updateEventReason, "Updated %q", tlog.KObj{Obj: s.Current.Cluster})
	return nil
}

// reconcileMachineDeployments reconciles the desired state of the MachineDeployment objects.
func (r *Reconciler) reconcileMachineDeployments(ctx context.Context, s *scope.Scope) error {
	diff := calculateMachineDeploymentDiff(s.Current.MachineDeployments, s.Desired.MachineDeployments)

	// Create MachineDeployments.
	for _, mdTopologyName := range diff.toCreate {
		md := s.Desired.MachineDeployments[mdTopologyName]
		if err := r.createMachineDeployment(ctx, s.Current.Cluster, md); err != nil {
			return err
		}
	}

	// Update MachineDeployments.
	for _, mdTopologyName := range diff.toUpdate {
		currentMD := s.Current.MachineDeployments[mdTopologyName]
		desiredMD := s.Desired.MachineDeployments[mdTopologyName]
		if err := r.updateMachineDeployment(ctx, s.Current.Cluster, mdTopologyName, currentMD, desiredMD); err != nil {
			return err
		}
	}

	// Delete MachineDeployments.
	for _, mdTopologyName := range diff.toDelete {
		md := s.Current.MachineDeployments[mdTopologyName]
		if err := r.deleteMachineDeployment(ctx, s.Current.Cluster, md); err != nil {
			return err
		}
	}
	return nil
}

// createMachineDeployment creates a MachineDeployment and the corresponding Templates.
func (r *Reconciler) createMachineDeployment(ctx context.Context, cluster *clusterv1.Cluster, md *scope.MachineDeploymentState) error {
	log := tlog.LoggerFrom(ctx).WithMachineDeployment(md.Object)

	infraCtx, _ := log.WithObject(md.InfrastructureMachineTemplate).Into(ctx)
	if err := r.reconcileReferencedTemplate(infraCtx, reconcileReferencedTemplateInput{
		cluster: cluster,
		desired: md.InfrastructureMachineTemplate,
	}); err != nil {
		return errors.Wrapf(err, "failed to create %s", md.Object.Kind)
	}

	bootstrapCtx, _ := log.WithObject(md.BootstrapTemplate).Into(ctx)
	if err := r.reconcileReferencedTemplate(bootstrapCtx, reconcileReferencedTemplateInput{
		cluster: cluster,
		desired: md.BootstrapTemplate,
	}); err != nil {
		return errors.Wrapf(err, "failed to create %s", md.Object.Kind)
	}

	log = log.WithObject(md.Object)
	log.Infof(fmt.Sprintf("Creating %s", tlog.KObj{Obj: md.Object}))
	if err := r.Client.Create(ctx, md.Object.DeepCopy()); err != nil {
		return createErrorWithoutObjectName(err, md.Object)
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, createEventReason, "Created %q", tlog.KObj{Obj: md.Object})

	// If the MachineDeployment has defined a MachineHealthCheck set the OwnerReference and reconcile it.
	if md.MachineHealthCheck != nil {
		// Set the MachineDeployment Object as owner to ensure object garbage collection in case something
		// happens before the MHC sets ownership to the Cluster.
		md.MachineHealthCheck.SetOwnerReferences([]metav1.OwnerReference{
			*ownerReferenceTo(md.Object),
		})
		if err := r.reconcileMachineHealthCheck(ctx, nil, md.MachineHealthCheck); err != nil {
			return err
		}
	}
	return nil
}

// updateMachineDeployment updates a MachineDeployment. Also rotates the corresponding Templates if necessary.
func (r *Reconciler) updateMachineDeployment(ctx context.Context, cluster *clusterv1.Cluster, mdTopologyName string, currentMD, desiredMD *scope.MachineDeploymentState) error {
	log := tlog.LoggerFrom(ctx).WithMachineDeployment(desiredMD.Object)

	infraCtx, _ := log.WithObject(desiredMD.InfrastructureMachineTemplate).Into(ctx)
	if err := r.reconcileReferencedTemplate(infraCtx, reconcileReferencedTemplateInput{
		cluster:              cluster,
		ref:                  &desiredMD.Object.Spec.Template.Spec.InfrastructureRef,
		current:              currentMD.InfrastructureMachineTemplate,
		desired:              desiredMD.InfrastructureMachineTemplate,
		templateNamePrefix:   infrastructureMachineTemplateNamePrefix(cluster.Name, mdTopologyName),
		compatibilityChecker: check.ObjectsAreCompatible,
	}); err != nil {
		return errors.Wrapf(err, "failed to reconcile %s", tlog.KObj{Obj: currentMD.Object})
	}

	bootstrapCtx, _ := log.WithObject(desiredMD.BootstrapTemplate).Into(ctx)
	if err := r.reconcileReferencedTemplate(bootstrapCtx, reconcileReferencedTemplateInput{
		cluster:              cluster,
		ref:                  desiredMD.Object.Spec.Template.Spec.Bootstrap.ConfigRef,
		current:              currentMD.BootstrapTemplate,
		desired:              desiredMD.BootstrapTemplate,
		templateNamePrefix:   bootstrapTemplateNamePrefix(cluster.Name, mdTopologyName),
		compatibilityChecker: check.ObjectsAreInTheSameNamespace,
	}); err != nil {
		return errors.Wrapf(err, "failed to reconcile %s", tlog.KObj{Obj: currentMD.Object})
	}

	// Patch MachineHealthCheck for the MachineDeployment.
	if desiredMD.MachineHealthCheck != nil || currentMD.MachineHealthCheck != nil {
		if err := r.reconcileMachineHealthCheck(ctx, currentMD.MachineHealthCheck, desiredMD.MachineHealthCheck); err != nil {
			return err
		}
	}

	// Check differences between current and desired MachineDeployment, and eventually patch the current object.
	log = log.WithObject(desiredMD.Object)
	patchHelper, err := mergepatch.NewHelper(currentMD.Object, desiredMD.Object, r.Client, mergepatch.AuthoritativePaths{
		// Note: we want to be authoritative WRT machine's metadata labels and annotations.
		// This has the nice benefit that it greatly simplify the UX around MachineDeploymentClass.Metadata and
		// MachineDeploymentTopology.Metadata, given that changes are reflected into generated objects without
		// accounting for instance specific changes like we do for other maps into spec.
		// Note: nested metadata have only labels and annotations, so it is possible to override the entire
		// parent struct.
		{"spec", "template", "metadata"},
		// Note: we want to be authoritative for the selector too, because if the selector and metadata.labels
		// change, the metadata.labels might not match the selector anymore, if we don't delete outdated labels
		// from the selector.
		{"spec", "selector"},
		// Note: We want to be authoritative for the failureDomain set in the MachineDeployment
		// spec.template.spec.failureDomain. This ensures that a change to the MachineDeploymentTopology failureDomain
		// is reconciled correctly.
		{"spec", "template", "spec", "failureDomain"},
	})
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
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, updateEventReason, "Updated %q%s", tlog.KObj{Obj: currentMD.Object}, logMachineDeploymentVersionChange(currentMD.Object, desiredMD.Object))

	// We want to call both cleanup functions even if one of them fails to clean up as much as possible.
	return nil
}

func logMachineDeploymentVersionChange(current, desired *clusterv1.MachineDeployment) string {
	if current.Spec.Template.Spec.Version == nil || desired.Spec.Template.Spec.Version == nil {
		return ""
	}

	if *current.Spec.Template.Spec.Version != *desired.Spec.Template.Spec.Version {
		return fmt.Sprintf(" with version change from %s to %s", *current.Spec.Template.Spec.Version, *desired.Spec.Template.Spec.Version)
	}
	return ""
}

// deleteMachineDeployment deletes a MachineDeployment.
func (r *Reconciler) deleteMachineDeployment(ctx context.Context, cluster *clusterv1.Cluster, md *scope.MachineDeploymentState) error {
	log := tlog.LoggerFrom(ctx).WithMachineDeployment(md.Object).WithObject(md.Object)

	// delete MachineHealthCheck for the MachineDeployment.
	if md.MachineHealthCheck != nil {
		if err := r.reconcileMachineHealthCheck(ctx, md.MachineHealthCheck, nil); err != nil {
			return err
		}
	}
	log.Infof("Deleting %s", tlog.KObj{Obj: md.Object})
	if err := r.Client.Delete(ctx, md.Object); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete %s", tlog.KObj{Obj: md.Object})
	}
	r.recorder.Eventf(cluster, corev1.EventTypeNormal, deleteEventReason, "Deleted %q", tlog.KObj{Obj: md.Object})
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

type unstructuredVersionGetter func(obj *unstructured.Unstructured) (*string, error)

type reconcileReferencedObjectInput struct {
	cluster       *clusterv1.Cluster
	current       *unstructured.Unstructured
	desired       *unstructured.Unstructured
	versionGetter unstructuredVersionGetter
	opts          []mergepatch.HelperOption
}

// reconcileReferencedObject reconciles the desired state of the referenced object.
// NOTE: After a referenced object is created it is assumed that the reference should
// never change (only the content of the object can eventually change). Thus, we are checking for strict compatibility.
func (r *Reconciler) reconcileReferencedObject(ctx context.Context, in reconcileReferencedObjectInput) error {
	log := tlog.LoggerFrom(ctx)

	// If there is no current object, create it.
	if in.current == nil {
		log.Infof("Creating %s", tlog.KObj{Obj: in.desired})

		desiredWithManagedFieldAnnotation, err := mergepatch.DeepCopyWithManagedFieldAnnotation(in.desired)
		if err != nil {
			return errors.Wrapf(err, "failed to create a copy of %s with the managed field annotation", tlog.KObj{Obj: in.desired})
		}
		if err := r.Client.Create(ctx, desiredWithManagedFieldAnnotation); err != nil {
			return createErrorWithoutObjectName(err, desiredWithManagedFieldAnnotation)
		}
		r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, createEventReason, "Created %q", tlog.KObj{Obj: in.desired})
		return nil
	}

	// Check if the current and desired referenced object are compatible.
	if allErrs := check.ObjectsAreStrictlyCompatible(in.current, in.desired); len(allErrs) > 0 {
		return allErrs.ToAggregate()
	}

	// Check differences between current and desired state, and eventually patch the current object.
	patchHelper, err := mergepatch.NewHelper(in.current, in.desired, r.Client, in.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: in.current})
	}
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: in.desired})
		return nil
	}

	log.Infof("Patching %s", tlog.KObj{Obj: in.desired})
	if err := patchHelper.Patch(ctx); err != nil {
		return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: in.current})
	}
	r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, updateEventReason, "Updated %q%s", tlog.KObj{Obj: in.desired}, logUnstructuredVersionChange(in.current, in.desired, in.versionGetter))
	return nil
}

func logUnstructuredVersionChange(current, desired *unstructured.Unstructured, versionGetter unstructuredVersionGetter) string {
	if versionGetter == nil {
		return ""
	}

	currentVersion, err := versionGetter(current)
	if err != nil || currentVersion == nil {
		return ""
	}
	desiredVersion, err := versionGetter(desired)
	if err != nil || desiredVersion == nil {
		return ""
	}

	if *currentVersion != *desiredVersion {
		return fmt.Sprintf(" with version change from %s to %s", *currentVersion, *desiredVersion)
	}
	return ""
}

type reconcileReferencedTemplateInput struct {
	cluster              *clusterv1.Cluster
	ref                  *corev1.ObjectReference
	current              *unstructured.Unstructured
	desired              *unstructured.Unstructured
	templateNamePrefix   string
	compatibilityChecker func(current, desired client.Object) field.ErrorList
}

// reconcileReferencedTemplate reconciles the desired state of a referenced Template.
// NOTE: According to Cluster API operational practices, when a referenced Template changes a template rotation is required:
// 1. create a new Template
// 2. update the reference
// 3. delete the old Template
// This function specifically takes care of the first step and updates the reference locally. So the remaining steps
// can be executed afterwards.
// NOTE: This func has a side effect in case of template rotation, changing both the desired object and the object reference.
func (r *Reconciler) reconcileReferencedTemplate(ctx context.Context, in reconcileReferencedTemplateInput) error {
	log := tlog.LoggerFrom(ctx)

	// If there is no current object, create the desired object.
	if in.current == nil {
		log.Infof("Creating %s", tlog.KObj{Obj: in.desired})
		desiredWithManagedFieldAnnotation, err := mergepatch.DeepCopyWithManagedFieldAnnotation(in.desired)
		if err != nil {
			return errors.Wrapf(err, "failed to create a copy of %s with the managed field annotation", tlog.KObj{Obj: in.desired})
		}
		if err := r.Client.Create(ctx, desiredWithManagedFieldAnnotation); err != nil {
			return createErrorWithoutObjectName(err, desiredWithManagedFieldAnnotation)
		}
		r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, createEventReason, "Created %q", tlog.KObj{Obj: in.desired})
		return nil
	}

	if in.ref == nil {
		return errors.Errorf("failed to rotate %s: ref should not be nil", in.desired.GroupVersionKind())
	}

	// Check if the current and desired referenced object are compatible.
	if allErrs := in.compatibilityChecker(in.current, in.desired); len(allErrs) > 0 {
		return allErrs.ToAggregate()
	}

	// Check differences between current and desired objects, and if there are changes eventually start the template rotation.
	patchHelper, err := mergepatch.NewHelper(in.current, in.desired, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: in.current})
	}

	// Return if no changes are detected.
	if !patchHelper.HasChanges() {
		log.V(3).Infof("No changes for %s", tlog.KObj{Obj: in.desired})
		return nil
	}

	// If there are no changes in the spec, and thus only changes in metadata, instead of doing a full template
	// rotation we patch the object in place. This avoids recreating machines.
	if !patchHelper.HasSpecChanges() {
		log.Infof("Patching %s", tlog.KObj{Obj: in.desired})
		if err := patchHelper.Patch(ctx); err != nil {
			return errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: in.desired})
		}
		r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, updateEventReason, "Updated %q (metadata changes)", tlog.KObj{Obj: in.desired})
		return nil
	}

	// Create the new template.

	// NOTE: it is required to assign a new name, because during compute the desired object name is enforced to be equal to the current one.
	// TODO: find a way to make side effect more explicit
	newName := names.SimpleNameGenerator.GenerateName(in.templateNamePrefix)
	in.desired.SetName(newName)

	log.Infof("Rotating %s, new name %s", tlog.KObj{Obj: in.current}, newName)
	log.Infof("Creating %s", tlog.KObj{Obj: in.desired})
	desiredWithManagedFieldAnnotation, err := mergepatch.DeepCopyWithManagedFieldAnnotation(in.desired)
	if err != nil {
		return errors.Wrapf(err, "failed to create a copy of %s with the managed field annotation", tlog.KObj{Obj: in.desired})
	}
	if err := r.Client.Create(ctx, desiredWithManagedFieldAnnotation); err != nil {
		return createErrorWithoutObjectName(err, desiredWithManagedFieldAnnotation)
	}
	r.recorder.Eventf(in.cluster, corev1.EventTypeNormal, createEventReason, "Created %q as a replacement for %q (template rotation)", tlog.KObj{Obj: in.desired}, in.ref.Name)

	// Update the reference with the new name.
	// NOTE: Updating the object hosting reference to the template is executed outside this func.
	// TODO: find a way to make side effect more explicit
	in.ref.Name = newName

	return nil
}

// createErrorWithoutObjectName removes the name of the object from the error message. As each new Create call involves an
// object with a unique generated name each error appears to be a different error. As the errors are being surfaced in a condition
// on the Cluster, the name is removed here to prevent each creation error from triggering a new reconciliation.
func createErrorWithoutObjectName(err error, obj client.Object) error {
	var statusError *apierrors.StatusError
	if errors.As(err, &statusError) {
		if statusError.Status().Details != nil {
			var causes []string
			for _, cause := range statusError.Status().Details.Causes {
				causes = append(causes, fmt.Sprintf("%s: %s: %s", cause.Type, cause.Field, cause.Message))
			}
			var msg string
			if len(causes) > 0 {
				msg = fmt.Sprintf("failed to create %s.%s: %s", statusError.Status().Details.Kind, statusError.Status().Details.Group, strings.Join(causes, " "))
			} else {
				msg = fmt.Sprintf("failed to create %s.%s", statusError.Status().Details.Kind, statusError.Status().Details.Group)
			}
			// Replace the statusError message with the constructed message.
			statusError.ErrStatus.Message = msg
			return statusError
		}
	}
	// If this isn't a StatusError return a more generic error with the object details.
	return errors.Wrapf(err, "failed to create %s", tlog.KObj{Obj: obj})
}
