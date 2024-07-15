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
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/topology/check"
	"sigs.k8s.io/cluster-api/internal/topology/names"
	"sigs.k8s.io/cluster-api/internal/topology/variables"
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

	if in.Spec.ControlPlane.MachineHealthCheck != nil {
		defaultNamespace(in.Spec.ControlPlane.MachineHealthCheck.RemediationTemplate, in.Namespace)
	}

	for i := range in.Spec.Workers.MachineDeployments {
		defaultNamespace(in.Spec.Workers.MachineDeployments[i].Template.Bootstrap.Ref, in.Namespace)
		defaultNamespace(in.Spec.Workers.MachineDeployments[i].Template.Infrastructure.Ref, in.Namespace)

		if in.Spec.Workers.MachineDeployments[i].MachineHealthCheck != nil {
			defaultNamespace(in.Spec.Workers.MachineDeployments[i].MachineHealthCheck.RemediationTemplate, in.Namespace)
		}
	}

	for i := range in.Spec.Workers.MachinePools {
		defaultNamespace(in.Spec.Workers.MachinePools[i].Template.Bootstrap.Ref, in.Namespace)
		defaultNamespace(in.Spec.Workers.MachinePools[i].Template.Infrastructure.Ref, in.Namespace)
	}

	return nil
}

func defaultNamespace(ref *corev1.ObjectReference, namespace string) {
	if ref != nil && ref.Namespace == "" {
		ref.Namespace = namespace
	}
}

// ValidateCreate implements validation for ClusterClass create.
func (webhook *ClusterClass) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	in, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}
	return nil, webhook.validate(ctx, nil, in)
}

// ValidateUpdate implements validation for ClusterClass update.
func (webhook *ClusterClass) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newClusterClass, ok := newObj.(*clusterv1.ClusterClass)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", newObj))
	}
	oldClusterClass, ok := oldObj.(*clusterv1.ClusterClass)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", oldObj))
	}
	return nil, webhook.validate(ctx, oldClusterClass, newClusterClass)
}

// ValidateDelete implements validation for ClusterClass delete.
func (webhook *ClusterClass) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	clusterClass, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}

	clusters, err := webhook.getClustersUsingClusterClass(ctx, clusterClass)
	if err != nil {
		return nil, apierrors.NewInternalError(errors.Wrapf(err, "could not retrieve Clusters using ClusterClass"))
	}

	if len(clusters) > 0 {
		// TODO(killianmuldoon): Improve error here to include the names of some clusters using the clusterClass.
		return nil, apierrors.NewForbidden(clusterv1.GroupVersion.WithResource("ClusterClass").GroupResource(), clusterClass.Name,
			fmt.Errorf("ClusterClass cannot be deleted because it is used by %d Cluster(s)", len(clusters)))
	}
	return nil, nil
}

func (webhook *ClusterClass) validate(ctx context.Context, oldClusterClass, newClusterClass *clusterv1.ClusterClass) error {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the web hook
	// must prevent creating new objects when the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterTopology feature flag is enabled",
		)
	}
	var allErrs field.ErrorList

	// Ensure all references are valid.
	allErrs = append(allErrs, check.ClusterClassReferencesAreValid(newClusterClass)...)

	// Ensure all MachineDeployment classes are unique.
	allErrs = append(allErrs, check.MachineDeploymentClassesAreUnique(newClusterClass)...)

	// Ensure all MachinePool classes are unique.
	allErrs = append(allErrs, check.MachinePoolClassesAreUnique(newClusterClass)...)

	// Ensure MachineHealthChecks are valid.
	allErrs = append(allErrs, validateMachineHealthCheckClasses(newClusterClass)...)

	// Ensure NamingStrategies are valid.
	allErrs = append(allErrs, validateNamingStrategies(newClusterClass)...)

	// Validate variables.
	var oldClusterClassVariables []clusterv1.ClusterClassVariable
	if oldClusterClass != nil {
		oldClusterClassVariables = oldClusterClass.Spec.Variables
	}
	allErrs = append(allErrs,
		variables.ValidateClusterClassVariables(ctx, oldClusterClassVariables, newClusterClass.Spec.Variables, field.NewPath("spec", "variables"))...,
	)

	// Validate patches.
	allErrs = append(allErrs, validatePatches(newClusterClass)...)

	// Validate metadata
	allErrs = append(allErrs, validateClusterClassMetadata(newClusterClass)...)

	// If this is an update run additional validation.
	if oldClusterClass != nil {
		// Ensure spec changes are compatible.
		allErrs = append(allErrs, check.ClusterClassesAreCompatible(oldClusterClass, newClusterClass)...)

		// Retrieve all clusters using the ClusterClass.
		clusters, err := webhook.getClustersUsingClusterClass(ctx, oldClusterClass)
		if err != nil {
			allErrs = append(allErrs, field.InternalError(field.NewPath(""),
				errors.Wrapf(err, "Clusters using ClusterClass %v can not be retrieved", oldClusterClass.Name)))
			return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("ClusterClass").GroupKind(), newClusterClass.Name, allErrs)
		}

		// Ensure no MachineDeploymentClass currently in use has been removed from the ClusterClass.
		allErrs = append(allErrs,
			webhook.validateRemovedMachineDeploymentClassesAreNotUsed(clusters, oldClusterClass, newClusterClass)...)

		// Ensure no MachinePoolClass currently in use has been removed from the ClusterClass.
		allErrs = append(allErrs,
			webhook.validateRemovedMachinePoolClassesAreNotUsed(clusters, oldClusterClass, newClusterClass)...)

		// Ensure no MachineHealthCheck currently in use has been removed from the ClusterClass.
		allErrs = append(allErrs,
			validateUpdatesToMachineHealthCheckClasses(clusters, oldClusterClass, newClusterClass)...)

		allErrs = append(allErrs,
			validateAutoscalerAnnotationsForClusterClass(clusters, newClusterClass)...)
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("ClusterClass").GroupKind(), newClusterClass.Name, allErrs)
	}
	return nil
}

// validateUpdatesToMachineHealthCheckClasses checks if the updates made to MachineHealthChecks are valid.
// It makes sure that if a MachineHealthCheck definition is dropped from the ClusterClass then none of the
// clusters using the ClusterClass rely on it to create a MachineHealthCheck.
// A cluster relies on an MachineHealthCheck in the ClusterClass if in cluster topology MachineHealthCheck
// is explicitly enabled and it does not provide a MachineHealthCheckOverride.
func validateUpdatesToMachineHealthCheckClasses(clusters []clusterv1.Cluster, oldClusterClass, newClusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	// Check if the MachineHealthCheck for the control plane is dropped.
	if oldClusterClass.Spec.ControlPlane.MachineHealthCheck != nil && newClusterClass.Spec.ControlPlane.MachineHealthCheck == nil {
		// Make sure that none of the clusters are using this MachineHealthCheck.
		clustersUsingMHC := []string{}
		for _, cluster := range clusters {
			if cluster.Spec.Topology.ControlPlane.MachineHealthCheck != nil &&
				cluster.Spec.Topology.ControlPlane.MachineHealthCheck.Enable != nil &&
				*cluster.Spec.Topology.ControlPlane.MachineHealthCheck.Enable &&
				cluster.Spec.Topology.ControlPlane.MachineHealthCheck.MachineHealthCheckClass.IsZero() {
				clustersUsingMHC = append(clustersUsingMHC, cluster.Name)
			}
		}
		if len(clustersUsingMHC) != 0 {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "controlPlane", "machineHealthCheck"),
				fmt.Sprintf("MachineHealthCheck cannot be deleted because it is used by Cluster(s) %q", strings.Join(clustersUsingMHC, ",")),
			))
		}
	}

	// For each MachineDeploymentClass check if the MachineHealthCheck definition is dropped.
	for _, newMdClass := range newClusterClass.Spec.Workers.MachineDeployments {
		oldMdClass := machineDeploymentClassOfName(oldClusterClass, newMdClass.Class)
		if oldMdClass == nil {
			// This is a new MachineDeploymentClass. Nothing to do here.
			continue
		}
		// If the MachineHealthCheck is dropped then check that no cluster is using it.
		if oldMdClass.MachineHealthCheck != nil && newMdClass.MachineHealthCheck == nil {
			clustersUsingMHC := []string{}
			for _, cluster := range clusters {
				if cluster.Spec.Topology.Workers == nil {
					continue
				}
				for _, mdTopology := range cluster.Spec.Topology.Workers.MachineDeployments {
					if mdTopology.Class == newMdClass.Class {
						if mdTopology.MachineHealthCheck != nil &&
							mdTopology.MachineHealthCheck.Enable != nil &&
							*mdTopology.MachineHealthCheck.Enable &&
							mdTopology.MachineHealthCheck.MachineHealthCheckClass.IsZero() {
							clustersUsingMHC = append(clustersUsingMHC, cluster.Name)
							break
						}
					}
				}
			}
			if len(clustersUsingMHC) != 0 {
				allErrs = append(allErrs, field.Forbidden(
					field.NewPath("spec", "workers", "machineDeployments").Key(newMdClass.Class).Child("machineHealthCheck"),
					fmt.Sprintf("MachineHealthCheck cannot be deleted because it is used by Cluster(s) %q", strings.Join(clustersUsingMHC, ",")),
				))
			}
		}
	}

	return allErrs
}

func (webhook *ClusterClass) validateRemovedMachineDeploymentClassesAreNotUsed(clusters []clusterv1.Cluster, oldClusterClass, newClusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	removedClasses := webhook.removedMachineDeploymentClasses(oldClusterClass, newClusterClass)
	// If no classes have been removed return early as no further checks are needed.
	if len(removedClasses) == 0 {
		return nil
	}
	// Error if any Cluster using the ClusterClass uses a MachineDeploymentClass that has been removed.
	for _, c := range clusters {
		for _, machineDeploymentTopology := range c.Spec.Topology.Workers.MachineDeployments {
			if removedClasses.Has(machineDeploymentTopology.Class) {
				// TODO(killianmuldoon): Improve error printing here so large scale changes don't flood the error log e.g. deduplication, only example usages given.
				// TODO: consider if we get the index of the MachineDeploymentClass being deleted
				allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "workers", "machineDeployments"),
					fmt.Sprintf("MachineDeploymentClass %q cannot be deleted because it is used by Cluster %q",
						machineDeploymentTopology.Class, c.Name),
				))
			}
		}
	}
	return allErrs
}

func (webhook *ClusterClass) validateRemovedMachinePoolClassesAreNotUsed(clusters []clusterv1.Cluster, oldClusterClass, newClusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	removedClasses := webhook.removedMachinePoolClasses(oldClusterClass, newClusterClass)
	// If no classes have been removed return early as no further checks are needed.
	if len(removedClasses) == 0 {
		return nil
	}
	// Error if any Cluster using the ClusterClass uses a MachinePoolClass that has been removed.
	for _, c := range clusters {
		for _, machinePoolTopology := range c.Spec.Topology.Workers.MachinePools {
			if removedClasses.Has(machinePoolTopology.Class) {
				// TODO(killianmuldoon): Improve error printing here so large scale changes don't flood the error log e.g. deduplication, only example usages given.
				// TODO: consider if we get the index of the MachinePoolClass being deleted
				allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "workers", "machinePools"),
					fmt.Sprintf("MachinePoolClass %q cannot be deleted because it is used by Cluster %q",
						machinePoolTopology.Class, c.Name),
				))
			}
		}
	}
	return allErrs
}

func (webhook *ClusterClass) removedMachineDeploymentClasses(oldClusterClass, newClusterClass *clusterv1.ClusterClass) sets.Set[string] {
	removedClasses := sets.Set[string]{}

	mdClasses := webhook.classNamesFromMDWorkerClass(newClusterClass.Spec.Workers)
	for _, oldClass := range oldClusterClass.Spec.Workers.MachineDeployments {
		if !mdClasses.Has(oldClass.Class) {
			removedClasses.Insert(oldClass.Class)
		}
	}
	return removedClasses
}

func (webhook *ClusterClass) removedMachinePoolClasses(oldClusterClass, newClusterClass *clusterv1.ClusterClass) sets.Set[string] {
	removedClasses := sets.Set[string]{}

	mpClasses := webhook.classNamesFromMPWorkerClass(newClusterClass.Spec.Workers)
	for _, oldClass := range oldClusterClass.Spec.Workers.MachinePools {
		if !mpClasses.Has(oldClass.Class) {
			removedClasses.Insert(oldClass.Class)
		}
	}
	return removedClasses
}

// classNamesFromMDWorkerClass returns the set of MachineDeployment class names.
func (webhook *ClusterClass) classNamesFromMDWorkerClass(w clusterv1.WorkersClass) sets.Set[string] {
	classes := sets.Set[string]{}
	for _, class := range w.MachineDeployments {
		classes.Insert(class.Class)
	}
	return classes
}

// classNamesFromMPWorkerClass returns the set of MachinePool class names.
func (webhook *ClusterClass) classNamesFromMPWorkerClass(w clusterv1.WorkersClass) sets.Set[string] {
	classes := sets.Set[string]{}
	for _, class := range w.MachinePools {
		classes.Insert(class.Class)
	}
	return classes
}

func (webhook *ClusterClass) getClustersUsingClusterClass(ctx context.Context, clusterClass *clusterv1.ClusterClass) ([]clusterv1.Cluster, error) {
	clusters := &clusterv1.ClusterList{}
	err := webhook.Client.List(ctx, clusters,
		client.MatchingFields{index.ClusterClassNameField: clusterClass.Name},
		client.InNamespace(clusterClass.Namespace),
	)
	if err != nil {
		return nil, err
	}
	return clusters.Items, nil
}

func getClusterClassVariablesMapWithReverseIndex(clusterClassVariables []clusterv1.ClusterClassVariable) (map[string]*clusterv1.ClusterClassVariable, map[string]int) {
	variablesMap := map[string]*clusterv1.ClusterClassVariable{}
	variablesIndexMap := map[string]int{}

	for i := range clusterClassVariables {
		variablesMap[clusterClassVariables[i].Name] = &clusterClassVariables[i]
		variablesIndexMap[clusterClassVariables[i].Name] = i
	}
	return variablesMap, variablesIndexMap
}

func validateMachineHealthCheckClasses(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	// Validate ControlPlane MachineHealthCheck if defined.
	if clusterClass.Spec.ControlPlane.MachineHealthCheck != nil {
		fldPath := field.NewPath("spec", "controlPlane", "machineHealthCheck")

		allErrs = append(allErrs, validateMachineHealthCheckClass(fldPath, clusterClass.Namespace,
			clusterClass.Spec.ControlPlane.MachineHealthCheck)...)

		// Ensure ControlPlane does not define a MachineHealthCheck if it does not define MachineInfrastructure.
		if clusterClass.Spec.ControlPlane.MachineInfrastructure == nil {
			allErrs = append(allErrs, field.Forbidden(
				fldPath.Child("machineInfrastructure"),
				"can be set only if spec.controlPlane.machineInfrastructure is set",
			))
		}
	}

	// Validate MachineDeployment MachineHealthChecks.
	for _, md := range clusterClass.Spec.Workers.MachineDeployments {
		if md.MachineHealthCheck == nil {
			continue
		}
		fldPath := field.NewPath("spec", "workers", "machineDeployments").Key(md.Class).Child("machineHealthCheck")

		allErrs = append(allErrs, validateMachineHealthCheckClass(fldPath, clusterClass.Namespace, md.MachineHealthCheck)...)
	}
	return allErrs
}

func validateNamingStrategies(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	if clusterClass.Spec.ControlPlane.NamingStrategy != nil && clusterClass.Spec.ControlPlane.NamingStrategy.Template != nil {
		name, err := names.ControlPlaneNameGenerator(*clusterClass.Spec.ControlPlane.NamingStrategy.Template, "cluster").GenerateName()
		templateFldPath := field.NewPath("spec", "controlPlane", "namingStrategy", "template")
		if err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					templateFldPath,
					*clusterClass.Spec.ControlPlane.NamingStrategy.Template,
					fmt.Sprintf("invalid ControlPlane name template: %v", err),
				))
		} else {
			for _, err := range validation.IsDNS1123Subdomain(name) {
				allErrs = append(allErrs, field.Invalid(templateFldPath, *clusterClass.Spec.ControlPlane.NamingStrategy.Template, err))
			}
		}
	}

	for _, md := range clusterClass.Spec.Workers.MachineDeployments {
		if md.NamingStrategy == nil || md.NamingStrategy.Template == nil {
			continue
		}
		name, err := names.MachineDeploymentNameGenerator(*md.NamingStrategy.Template, "cluster", "mdtopology").GenerateName()
		templateFldPath := field.NewPath("spec", "workers", "machineDeployments").Key(md.Class).Child("namingStrategy", "template")
		if err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					templateFldPath,
					*md.NamingStrategy.Template,
					fmt.Sprintf("invalid MachineDeployment name template: %v", err),
				))
		} else {
			for _, err := range validation.IsDNS1123Subdomain(name) {
				allErrs = append(allErrs, field.Invalid(templateFldPath, *md.NamingStrategy.Template, err))
			}
		}
	}

	for _, mp := range clusterClass.Spec.Workers.MachinePools {
		if mp.NamingStrategy == nil || mp.NamingStrategy.Template == nil {
			continue
		}
		name, err := names.MachinePoolNameGenerator(*mp.NamingStrategy.Template, "cluster", "mptopology").GenerateName()
		templateFldPath := field.NewPath("spec", "workers", "machinePools").Key(mp.Class).Child("namingStrategy", "template")
		if err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					templateFldPath,
					*mp.NamingStrategy.Template,
					fmt.Sprintf("invalid MachinePool name template: %v", err),
				))
		} else {
			for _, err := range validation.IsDNS1123Subdomain(name) {
				allErrs = append(allErrs, field.Invalid(templateFldPath, *mp.NamingStrategy.Template, err))
			}
		}
	}

	return allErrs
}

// validateMachineHealthCheckClass validates the MachineHealthCheckSpec fields defined in a MachineHealthCheckClass.
func validateMachineHealthCheckClass(fldPath *field.Path, namepace string, m *clusterv1.MachineHealthCheckClass) field.ErrorList {
	mhc := clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namepace,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			NodeStartupTimeout:  m.NodeStartupTimeout,
			MaxUnhealthy:        m.MaxUnhealthy,
			UnhealthyConditions: m.UnhealthyConditions,
			UnhealthyRange:      m.UnhealthyRange,
			RemediationTemplate: m.RemediationTemplate,
		}}

	return (&MachineHealthCheck{}).validateCommonFields(&mhc, fldPath)
}

func validateClusterClassMetadata(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, clusterClass.Spec.ControlPlane.Metadata.Validate(field.NewPath("spec", "controlPlane", "metadata"))...)
	for _, m := range clusterClass.Spec.Workers.MachineDeployments {
		allErrs = append(allErrs, m.Template.Metadata.Validate(field.NewPath("spec", "workers", "machineDeployments").Key(m.Class).Child("template", "metadata"))...)
	}
	for _, m := range clusterClass.Spec.Workers.MachinePools {
		allErrs = append(allErrs, m.Template.Metadata.Validate(field.NewPath("spec", "workers", "machinePools").Key(m.Class).Child("template", "metadata"))...)
	}
	return allErrs
}

// validateAutoscalerAnnotationsForClusterClass iterates over a list of Clusters that use a ClusterClass and returns
// errors if the ClusterClass contains autoscaler annotations while a Cluster has worker replicas.
func validateAutoscalerAnnotationsForClusterClass(clusters []clusterv1.Cluster, newClusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	for _, c := range clusters {
		allErrs = append(allErrs, validateAutoscalerAnnotationsForCluster(&c, newClusterClass)...)
	}
	return allErrs
}
