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

package webhooks

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/topology/check"
	"sigs.k8s.io/cluster-api/internal/topology/variables"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (webhook *ClusterClass) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.ClusterClass{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-cluster-x-k8s-io-v1beta1-clusterclass,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusterclasses,versions=v1beta1,name=validation.clusterclass.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-clusterclass,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusterclasses,versions=v1beta1,name=default.clusterclass.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ClusterClass implements a validation and defaulting webhook for ClusterClass.
type ClusterClass struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &ClusterClass{}
var _ webhook.CustomValidator = &ClusterClass{}

// Default implements defaulting for ClusterClass create and update.
func (webhook *ClusterClass) Default(_ context.Context, obj runtime.Object) error {
	in, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}
	// Default all namespaces in the references to the object namespace.
	defaultNamespace(in.Spec.Infrastructure.Ref, in.Namespace)
	defaultNamespace(in.Spec.ControlPlane.Ref, in.Namespace)

	if in.Spec.ControlPlane.MachineInfrastructure != nil {
		defaultNamespace(in.Spec.ControlPlane.MachineInfrastructure.Ref, in.Namespace)
	}

	for i := range in.Spec.Workers.MachineDeployments {
		defaultNamespace(in.Spec.Workers.MachineDeployments[i].Template.Bootstrap.Ref, in.Namespace)
		defaultNamespace(in.Spec.Workers.MachineDeployments[i].Template.Infrastructure.Ref, in.Namespace)
	}
	return nil
}

func defaultNamespace(ref *corev1.ObjectReference, namespace string) {
	if ref != nil && len(ref.Namespace) == 0 {
		ref.Namespace = namespace
	}
}

// ValidateCreate implements validation for ClusterClass create.
func (webhook *ClusterClass) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	in, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}
	return webhook.validate(ctx, nil, in)
}

// ValidateUpdate implements validation for ClusterClass update.
func (webhook *ClusterClass) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newClusterClass, ok := newObj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", newObj))
	}
	oldClusterClass, ok := oldObj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", oldObj))
	}
	return webhook.validate(ctx, oldClusterClass, newClusterClass)
}

// ValidateDelete implements validation for ClusterClass delete.
func (webhook *ClusterClass) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	clusterClass, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}

	clusters, err := webhook.getClustersUsingClusterClass(ctx, clusterClass)
	if err != nil {
		return apierrors.NewInternalError(errors.Wrapf(err, "could not retrieve Clusters using ClusterClass"))
	}

	if len(clusters) > 0 {
		// TODO(killianmuldoon): Improve error here to include the names of some clusters using the clusterClass.
		return apierrors.NewForbidden(clusterv1.GroupVersion.WithResource("ClusterClass").GroupResource(), clusterClass.Name,
			fmt.Errorf("cannot be deleted. %d clusters still using the ClusterClass", len(clusters)))
	}
	return nil
}

func (webhook *ClusterClass) validate(ctx context.Context, old, new *clusterv1.ClusterClass) error {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the web hook
	// must prevent creating new objects new case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterTopology feature flag is enabled",
		)
	}
	var allErrs field.ErrorList

	// Ensure all references are valid.
	allErrs = append(allErrs, check.ClusterClassReferencesAreValid(new)...)

	// Ensure all MachineDeployment classes are unique.
	allErrs = append(allErrs, check.MachineDeploymentClassesAreUnique(new)...)

	// If this is an update run additional validation.
	if old != nil {
		// Ensure spec changes are compatible.
		allErrs = append(allErrs, check.ClusterClassesAreCompatible(old, new)...)

		// Retrieve all clusters using the ClusterClass.
		clusters, err := webhook.getClustersUsingClusterClass(ctx, old)
		if err != nil {
			allErrs = append(allErrs, field.InternalError(field.NewPath(""),
				errors.Wrapf(err, "Clusters using ClusterClass %v can not be retrieved", old.Name)))
			return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("ClusterClass").GroupKind(), new.Name, allErrs)
		}

		// Ensure no MachineDeploymentClass currently in use has been removed from the ClusterClass.
		allErrs = append(allErrs, webhook.validateRemovedMachineDeploymentClassesAreNotUsed(clusters, old, new)...)
	}
	// Validate variables.
	allErrs = append(allErrs, variables.ValidateClusterClassVariables(new.Spec.Variables, field.NewPath("spec", "variables"))...)

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("ClusterClass").GroupKind(), new.Name, allErrs)
	}
	return nil
}

func (webhook *ClusterClass) validateRemovedMachineDeploymentClassesAreNotUsed(clusters []clusterv1.Cluster, old, new *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	removedClasses := webhook.removedMachineClasses(old, new)
	// If no classes have been removed return early as no further checks are needed.
	if len(removedClasses) == 0 {
		return nil
	}
	// Error if any Cluster using the ClusterClass uses a MachineDeploymentClass that has been removed.
	for _, c := range clusters {
		for _, machineDeploymentTopology := range c.Spec.Topology.Workers.MachineDeployments {
			if removedClasses.Has(machineDeploymentTopology.Class) {
				// TODO(killianmuldoon): Improve error printing here so large scale changes don't flood the error log e.g. deduplication, only example usages given.
				allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "workers", "machineDeployments"),
					fmt.Sprintf("MachineDeploymentClass %v is in use in MachineDeploymentTopology %v in Cluster %v. ClusterClass %v modification not allowed",
						machineDeploymentTopology.Class, machineDeploymentTopology.Name, c.Name, old.Name),
				))
			}
		}
	}
	return allErrs
}

func (webhook *ClusterClass) removedMachineClasses(old, new *clusterv1.ClusterClass) sets.String {
	removedClasses := sets.NewString()

	classes := webhook.classNamesFromWorkerClass(new.Spec.Workers)
	for _, oldClass := range old.Spec.Workers.MachineDeployments {
		if !classes.Has(oldClass.Class) {
			removedClasses.Insert(oldClass.Class)
		}
	}
	return removedClasses
}

// classNamesFromWorkerClass returns the set of MachineDeployment class names.
func (webhook *ClusterClass) classNamesFromWorkerClass(w clusterv1.WorkersClass) sets.String {
	classes := sets.NewString()
	for _, class := range w.MachineDeployments {
		classes.Insert(class.Class)
	}
	return classes
}

func (webhook *ClusterClass) getClustersUsingClusterClass(ctx context.Context, clusterClass *clusterv1.ClusterClass) ([]clusterv1.Cluster, error) {
	clusters := &clusterv1.ClusterList{}
	clustersUsingClusterClass := []clusterv1.Cluster{}
	err := webhook.Client.List(ctx, clusters,
		client.MatchingLabels{
			clusterv1.ClusterTopologyOwnedLabel: "",
		},
		client.InNamespace(clusterClass.Namespace),
	)
	if err != nil {
		return nil, err
	}
	for _, c := range clusters.Items {
		if c.Spec.Topology.Class == clusterClass.Name {
			clustersUsingClusterClass = append(clustersUsingClusterClass, c)
		}
	}
	return clustersUsingClusterClass, nil
}
