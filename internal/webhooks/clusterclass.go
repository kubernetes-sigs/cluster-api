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
	"reflect"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/topology/check"
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

	for i := range in.Spec.Workers.MachineDeployments {
		defaultNamespace(in.Spec.Workers.MachineDeployments[i].Template.Bootstrap.Ref, in.Namespace)
		defaultNamespace(in.Spec.Workers.MachineDeployments[i].Template.Infrastructure.Ref, in.Namespace)
	}
	return nil
}

func defaultNamespace(ref *corev1.ObjectReference, namespace string) {
	if ref != nil && ref.Namespace == "" {
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
			fmt.Errorf("ClusterClass cannot be deleted because it is used by %d Cluster(s)", len(clusters)))
	}
	return nil
}

func (webhook *ClusterClass) validate(ctx context.Context, oldClusterClass, newClusterClass *clusterv1.ClusterClass) error {
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
	allErrs = append(allErrs, check.ClusterClassReferencesAreValid(newClusterClass)...)

	// Ensure all MachineDeployment classes are unique.
	allErrs = append(allErrs, check.MachineDeploymentClassesAreUnique(newClusterClass)...)

	// Ensure MachineHealthChecks are valid.
	allErrs = append(allErrs, validateMachineHealthCheckClasses(newClusterClass)...)

	// Validate variables.
	allErrs = append(allErrs,
		variables.ValidateClusterClassVariables(newClusterClass.Spec.Variables, field.NewPath("spec", "variables"))...,
	)

	// Validate patches.
	allErrs = append(allErrs, validatePatches(newClusterClass)...)

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

		// Ensure no Variable would be invalidated by the update in spec
		allErrs = append(allErrs,
			validateVariableUpdates(clusters, oldClusterClass, newClusterClass, field.NewPath("spec", "variables"))...)
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("ClusterClass").GroupKind(), newClusterClass.Name, allErrs)
	}
	return nil
}

// validateVariableUpdates checks if the updates made to ClusterClassVariables are valid.
// It retrieves a list of variables of interest and validates:
// 1) Altered ClusterClassVariables defined on any exiting Cluster are still valid with the updated Schema.
// 2) Removed ClusterClassVariables are not in use on any Cluster using the ClusterClass.
// 3) Added ClusterClassVariables defined on any exiting Cluster are still valid with the updated Schema.
// 4) Required ClusterClassVariables are defined on each Cluster using the ClusterClass.
func validateVariableUpdates(clusters []clusterv1.Cluster, oldClusterClass, newClusterClass *clusterv1.ClusterClass, path *field.Path) field.ErrorList {
	tracker := map[string][]string{}

	// Get the old ClusterClassVariables as a map
	oldVars, _ := getClusterClassVariablesMapWithReverseIndex(oldClusterClass.Spec.Variables)

	// Get the new ClusterClassVariables as a map with an index linking them to their place in the ClusterClass Variable array.
	// Note: The index is used to improve the error recording below.
	newVars, clusterClassVariablesIndex := getClusterClassVariablesMapWithReverseIndex(newClusterClass.Spec.Variables)

	// Compute the diff between old and new ClusterClassVariables.
	varsDiff := getClusterClassVariablesForValidation(oldVars, newVars)

	errorInfo := errorAggregator{}
	allClusters := []string{}

	// Validate the variable values on each Cluster ensuring they are still compatible with the new ClusterClass.
	for _, cluster := range clusters {
		allClusters = append(allClusters, cluster.Name)
		for _, c := range cluster.Spec.Topology.Variables {
			// copy variable to avoid memory aliasing.
			clusterVar := c

			// Add Cluster Variable entry in clusterVariableReferences to track where it is in use.
			tracker[clusterVar.Name] = append(tracker[clusterVar.Name], cluster.Name)

			// 1) Error if a variable with a schema altered in the update is no longer valid on the Cluster.
			if alteredVar, ok := varsDiff[variableValidationKey{clusterVar.Name, altered}]; ok {
				if errs := variables.ValidateClusterVariable(&clusterVar, alteredVar, field.NewPath("")); len(errs) > 0 {
					errorInfo.add(alteredVar.Name, altered, cluster.Name)
				}
				continue
			}

			// 2) Error if a variable removed in the update is still in use in some Clusters.
			if _, ok := varsDiff[variableValidationKey{clusterVar.Name, removed}]; ok {
				errorInfo.add(clusterVar.Name, removed, cluster.Name)
				continue
			}

			// 3) Error if a variable has been added in the update check is no longer valid on the Cluster.
			// NOTE: This can't occur in normal circumstances as a variable must be defined in a ClusterClass in order to be introduced in
			// a Cluster. This check may catch errors in cases involving broken Clusters.
			if addedVar, ok := varsDiff[variableValidationKey{clusterVar.Name, added}]; ok {
				if errs := variables.ValidateClusterVariable(&clusterVar, addedVar, field.NewPath("")); len(errs) > 0 {
					errorInfo.add(addedVar.Name, added, cluster.Name)
				}
				continue
			}
		}

		if cluster.Spec.Topology.Workers == nil {
			continue
		}

		for _, md := range cluster.Spec.Topology.Workers.MachineDeployments {
			// Continue if there are no variable overrides.
			if md.Variables == nil || len(md.Variables.Overrides) == 0 {
				continue
			}

			for _, c := range md.Variables.Overrides {
				// copy variable to avoid memory aliasing.
				clusterVar := c

				// 1) Error if a variable with a schema altered in the update is no longer valid on the Cluster.
				if alteredVar, ok := varsDiff[variableValidationKey{clusterVar.Name, altered}]; ok {
					if errs := variables.ValidateClusterVariable(&clusterVar, alteredVar, field.NewPath("")); len(errs) > 0 {
						errorInfo.add(fmt.Sprintf("%s/%s", md.Name, alteredVar.Name), altered, cluster.Name)
					}
					continue
				}

				// 2) Error if a variable removed in the update is still in use in some Clusters.
				if _, ok := varsDiff[variableValidationKey{clusterVar.Name, removed}]; ok {
					errorInfo.add(fmt.Sprintf("%s/%s", md.Name, clusterVar.Name), removed, cluster.Name)
					continue
				}

				// 3) Error if a variable has been added in the update check is no longer valid on the Cluster.
				// NOTE: This can't occur in normal circumstances as a variable must be defined in a ClusterClass in order to be introduced in
				// a Cluster. This check may catch errors in cases involving broken Clusters.
				if addedVar, ok := varsDiff[variableValidationKey{clusterVar.Name, added}]; ok {
					if errs := variables.ValidateClusterVariable(&clusterVar, addedVar, field.NewPath("")); len(errs) > 0 {
						errorInfo.add(fmt.Sprintf("%s/%s", md.Name, addedVar.Name), added, cluster.Name)
					}
					continue
				}
			}
		}
	}

	// 4) Error if a required variable is not defined top-level in every cluster using the ClusterClass.
	for v := range varsDiff {
		if v.action == required {
			clustersMissingVariable := clustersWithoutVar(allClusters, tracker[v.name])
			if len(clustersMissingVariable) > 0 {
				errorInfo.add(v.name, v.action, clustersMissingVariable...)
			}
		}
	}

	// Aggregate the errors from the validation checks and return one error message of each type for each variable with a list of violating clusters.
	return aggregateErrors(errorInfo, clusterClassVariablesIndex, path)
}

type errorAggregator map[variableValidationKey][]string

func (e errorAggregator) add(name string, action variableValidationType, references ...string) {
	e[variableValidationKey{name, action}] = append(e[variableValidationKey{name, action}], references...)
}

func aggregateErrors(errs map[variableValidationKey][]string, clusterClassVariablesIndex map[string]int, path *field.Path) field.ErrorList {
	var allErrs, addedErrs, alteredErrs, removedErrs, requiredErrs field.ErrorList
	// Look through the errors and append aggregated error messages naming the Clusters blocking changes to existing variables.
	for variable, clusters := range errs {
		switch variable.action {
		case altered:
			alteredErrs = append(alteredErrs,
				field.Forbidden(path.Index(clusterClassVariablesIndex[variable.name]),
					fmt.Sprintf("variable schema change for %s is invalid. It failed validation in Clusters %v", variable.name, clusters),
				))
		case removed:
			removedErrs = append(removedErrs,
				field.Forbidden(path.Index(clusterClassVariablesIndex[variable.name]),
					fmt.Sprintf("variable %s can not be removed. It is used in Clusters %v", variable.name, clusters),
				))
		case added:
			addedErrs = append(addedErrs,
				field.Forbidden(path.Index(clusterClassVariablesIndex[variable.name]),
					fmt.Sprintf("variable %s can not be added. It failed validation in Clusters %v", variable.name, clusters),
				))
		case required:
			requiredErrs = append(requiredErrs,
				field.Forbidden(path.Index(clusterClassVariablesIndex[variable.name]),
					fmt.Sprintf("variable %v is required but is not defined in Clusters %v", variable.name, clusters),
				))
		}
	}

	// Add the slices of errors one by one to maintain a defined order.
	allErrs = append(allErrs, alteredErrs...)
	allErrs = append(allErrs, removedErrs...)
	allErrs = append(allErrs, addedErrs...)
	allErrs = append(allErrs, requiredErrs...)

	return allErrs
}

func clustersWithoutVar(allClusters, clustersWithVar []string) []string {
	clustersWithVarMap := make(map[string]interface{})
	for _, cluster := range clustersWithVar {
		clustersWithVarMap[cluster] = nil
	}
	var clusterDiff []string
	for _, cluster := range allClusters {
		if _, found := clustersWithVarMap[cluster]; !found {
			clusterDiff = append(clusterDiff, cluster)
		}
	}
	return clusterDiff
}

type variableValidationType int

const (
	added variableValidationType = iota
	altered
	removed
	required
)

// variableValidationKey holds the name of the variable and a validation action to perform on it.
type variableValidationKey struct {
	name   string
	action variableValidationType
}

// getClusterClassVariablesForValidation returns a struct with the four classes of ClusterClass Variables that require validation:
// - added which have been added to the ClusterClass on update.
// - altered which have had their schemas changed on update.
// - removed which have been removed from the ClusterClass on update.
// - required (with 'required' : true) from the new ClusterClass.
func getClusterClassVariablesForValidation(oldVars, newVars map[string]*clusterv1.ClusterClassVariable) map[variableValidationKey]*clusterv1.ClusterClassVariable {
	out := map[variableValidationKey]*clusterv1.ClusterClassVariable{}

	// Compare the old Variable map to the new one to discover which variables have been removed and which have been altered.
	for k, v := range oldVars {
		if _, ok := newVars[k]; !ok {
			out[variableValidationKey{action: removed, name: k}] = v
		} else if !reflect.DeepEqual(v.Schema, newVars[k].Schema) {
			out[variableValidationKey{action: altered, name: k}] = newVars[k]
		}
	}
	// Compare the new ClusterClassVariables to the new ClusterClassVariables to find out what variables have been added and which are now required.
	for k, v := range newVars {
		if _, ok := oldVars[k]; !ok {
			out[variableValidationKey{action: added, name: k}] = v
		}
		if v.Required {
			out[variableValidationKey{action: required, name: v.Name}] = v
		}
	}
	return out
}

func (webhook *ClusterClass) validateRemovedMachineDeploymentClassesAreNotUsed(clusters []clusterv1.Cluster, oldClusterClass, newClusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	removedClasses := webhook.removedMachineClasses(oldClusterClass, newClusterClass)
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

func (webhook *ClusterClass) removedMachineClasses(oldClusterClass, newClusterClass *clusterv1.ClusterClass) sets.String {
	removedClasses := sets.NewString()

	classes := webhook.classNamesFromWorkerClass(newClusterClass.Spec.Workers)
	for _, oldClass := range oldClusterClass.Spec.Workers.MachineDeployments {
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
		// Ensure ControlPlane does not define a MachineHealthCheck if it does not define MachineInfrastructure.
		if clusterClass.Spec.ControlPlane.MachineInfrastructure == nil {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "controlPlane", "machineHealthCheck"),
				"can be set only if spec.controlPlane.machineInfrastructure is set",
			))
		}
		// Ensure ControlPlane MachineHealthCheck defines UnhealthyConditions.
		if len(clusterClass.Spec.ControlPlane.MachineHealthCheck.UnhealthyConditions) == 0 {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "controlPlane", "machineHealthCheck", "unhealthyConditions"),
				"must be defined and have at least one value",
			))
		}
	}

	// Ensure MachineDeployment MachineHealthChecks define UnhealthyConditions.
	for i, md := range clusterClass.Spec.Workers.MachineDeployments {
		if md.MachineHealthCheck == nil {
			continue
		}
		if len(md.MachineHealthCheck.UnhealthyConditions) == 0 {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "workers", "machineDeployments", "machineHealthCheck").Index(i).Child("unhealthyConditions"),
				"must be defined and have at least one value",
			))
		}
	}
	return allErrs
}
