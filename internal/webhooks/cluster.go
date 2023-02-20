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
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/topology/check"
	"sigs.k8s.io/cluster-api/internal/topology/variables"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/version"
)

// SetupWebhookWithManager sets up Cluster webhooks.
func (webhook *Cluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-cluster-x-k8s-io-v1beta1-cluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=validation.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-cluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=default.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// Cluster implements a validating and defaulting webhook for Cluster.
type Cluster struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &Cluster{}
var _ webhook.CustomValidator = &Cluster{}

// Default satisfies the defaulting webhook interface.
func (webhook *Cluster) Default(ctx context.Context, obj runtime.Object) error {
	// We gather all defaulting errors and return them together.
	var allErrs field.ErrorList

	cluster, ok := obj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}

	if cluster.Spec.InfrastructureRef != nil && cluster.Spec.InfrastructureRef.Namespace == "" {
		cluster.Spec.InfrastructureRef.Namespace = cluster.Namespace
	}

	if cluster.Spec.ControlPlaneRef != nil && cluster.Spec.ControlPlaneRef.Namespace == "" {
		cluster.Spec.ControlPlaneRef.Namespace = cluster.Namespace
	}

	// Additional defaulting if the Cluster uses a managed topology.
	if cluster.Spec.Topology != nil {
		// Tolerate version strings without a "v" prefix: prepend it if it's not there.
		if !strings.HasPrefix(cluster.Spec.Topology.Version, "v") {
			cluster.Spec.Topology.Version = "v" + cluster.Spec.Topology.Version
		}
		clusterClass, err := webhook.pollClusterClassForCluster(ctx, cluster)
		if err != nil {
			// If the ClusterClass can't be found ignore the error.
			if apierrors.IsNotFound(err) {
				return nil
			}
			return apierrors.NewInternalError(errors.Wrapf(err, "Cluster %s can't be defaulted. Valid ClusterClass %s can not be retrieved", cluster.Name, cluster.Spec.Topology.Class))
		}

		allErrs = append(allErrs, DefaultVariables(cluster, clusterClass)...)
		if len(allErrs) > 0 {
			return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), cluster.Name, allErrs)
		}
	}
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Cluster) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	cluster, ok := obj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}
	return webhook.validate(ctx, nil, cluster)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Cluster) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newCluster, ok := newObj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", newObj))
	}
	oldCluster, ok := oldObj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", oldObj))
	}
	return webhook.validate(ctx, oldCluster, newCluster)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Cluster) ValidateDelete(_ context.Context, _ runtime.Object) error {
	return nil
}

func (webhook *Cluster) validate(ctx context.Context, oldCluster, newCluster *clusterv1.Cluster) error {
	var allErrs field.ErrorList
	// The Cluster name is used as a label value. This check ensures that names which are not valid label values are rejected.
	if errs := validation.IsValidLabelValue(newCluster.Name); len(errs) != 0 {
		for _, err := range errs {
			allErrs = append(
				allErrs,
				field.Invalid(
					field.NewPath("metadata", "name"),
					newCluster.Name,
					fmt.Sprintf("must be a valid label value %s", err),
				),
			)
		}
	}
	specPath := field.NewPath("spec")
	if newCluster.Spec.InfrastructureRef != nil && newCluster.Spec.InfrastructureRef.Namespace != newCluster.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("infrastructureRef", "namespace"),
				newCluster.Spec.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if newCluster.Spec.ControlPlaneRef != nil && newCluster.Spec.ControlPlaneRef.Namespace != newCluster.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("controlPlaneRef", "namespace"),
				newCluster.Spec.ControlPlaneRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}
	if newCluster.Spec.ClusterNetwork != nil {
		// Ensure that the CIDR blocks defined under ClusterNetwork are valid.
		if newCluster.Spec.ClusterNetwork.Pods != nil {
			allErrs = append(allErrs, validateCIDRBlocks(specPath.Child("clusterNetwork", "pods", "cidrBlocks"),
				newCluster.Spec.ClusterNetwork.Pods.CIDRBlocks)...)
		}

		if newCluster.Spec.ClusterNetwork.Services != nil {
			allErrs = append(allErrs, validateCIDRBlocks(specPath.Child("clusterNetwork", "services", "cidrBlocks"),
				newCluster.Spec.ClusterNetwork.Services.CIDRBlocks)...)
		}
	}

	topologyPath := specPath.Child("topology")

	// Validate the managed topology, if defined.
	if newCluster.Spec.Topology != nil {
		allErrs = append(allErrs, webhook.validateTopology(ctx, oldCluster, newCluster, topologyPath)...)
	}

	// On update.
	if oldCluster != nil {
		// Error if the update moves the cluster from Managed to Unmanaged i.e. the managed topology is removed on update.
		if oldCluster.Spec.Topology != nil && newCluster.Spec.Topology == nil {
			allErrs = append(allErrs, field.Forbidden(
				topologyPath,
				"cannot be removed from an existing Cluster",
			))
		}
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), newCluster.Name, allErrs)
	}
	return nil
}

func (webhook *Cluster) validateTopology(ctx context.Context, oldCluster, newCluster *clusterv1.Cluster, fldPath *field.Path) field.ErrorList {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the web hook
	// must prevent the usage of Cluster.Topology in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return field.ErrorList{
			field.Forbidden(
				fldPath,
				"can be set only if the ClusterTopology feature flag is enabled",
			),
		}
	}

	var allErrs field.ErrorList

	// class should be defined.
	if newCluster.Spec.Topology.Class == "" {
		allErrs = append(
			allErrs,
			field.Required(
				fldPath.Child("class"),
				"class cannot be empty",
			),
		)
	}

	// version should be valid.
	if !version.KubeSemver.MatchString(newCluster.Spec.Topology.Version) {
		allErrs = append(
			allErrs,
			field.Invalid(
				fldPath.Child("version"),
				newCluster.Spec.Topology.Version,
				"version must be a valid semantic version",
			),
		)
	}

	// Get the ClusterClass referenced in the Cluster.
	clusterClass, clusterClassGetErr := webhook.pollClusterClassForCluster(ctx, newCluster)
	if clusterClassGetErr != nil && !apierrors.IsNotFound(clusterClassGetErr) {
		// If the error is anything other than "Not Found" return all errors at this point.
		allErrs = append(
			allErrs, field.InternalError(
				fldPath.Child("class"),
				clusterClassGetErr))
		return allErrs
	}
	if clusterClassGetErr == nil {
		// If there's no error validate the Cluster based on the ClusterClass.
		allErrs = append(allErrs, ValidateClusterForClusterClass(newCluster, clusterClass, fldPath)...)
	}
	if oldCluster != nil { // On update
		// The ClusterClass must exist to proceed with update validation. Return an error if the ClusterClass was
		// not found.
		if clusterClassGetErr != nil {
			allErrs = append(
				allErrs, field.InternalError(
					fldPath.Child("class"),
					clusterClassGetErr))
			return allErrs
		}

		// Topology or Class can not be added on update unless ClusterTopologyUnsafeUpdateClassNameAnnotation is set.
		if oldCluster.Spec.Topology == nil || oldCluster.Spec.Topology.Class == "" {
			if _, ok := newCluster.Annotations[clusterv1.ClusterTopologyUnsafeUpdateClassNameAnnotation]; ok {
				return allErrs
			}

			allErrs = append(
				allErrs,
				field.Forbidden(
					fldPath.Child("class"),
					"class cannot be set on an existing Cluster",
				),
			)
			// return early here if there is no class to compare.
			return allErrs
		}

		// Version could only be increased.
		inVersion, err := semver.ParseTolerant(newCluster.Spec.Topology.Version)
		if err != nil {
			allErrs = append(
				allErrs,
				field.Invalid(
					fldPath.Child("version"),
					newCluster.Spec.Topology.Version,
					"version must be a valid semantic version",
				),
			)
		}
		oldVersion, err := semver.ParseTolerant(oldCluster.Spec.Topology.Version)
		if err != nil {
			// NOTE: this should never happen. Nevertheless, handling this for extra caution.
			allErrs = append(
				allErrs,
				field.Invalid(
					fldPath.Child("version"),
					oldCluster.Spec.Topology.Version,
					fmt.Sprintf("old version %q cannot be compared with %q", oldVersion, inVersion),
				),
			)
		}
		if inVersion.NE(semver.Version{}) && oldVersion.NE(semver.Version{}) && version.Compare(inVersion, oldVersion, version.WithBuildTags()) == -1 {
			allErrs = append(
				allErrs,
				field.Invalid(
					fldPath.Child("version"),
					newCluster.Spec.Topology.Version,
					fmt.Sprintf("version cannot be decreased from %q to %q", oldVersion, inVersion),
				),
			)
		}
		// A +2 minor version upgrade is not allowed.
		ceilVersion := semver.Version{
			Major: oldVersion.Major,
			Minor: oldVersion.Minor + 2,
			Patch: 0,
		}
		if inVersion.GTE(ceilVersion) {
			allErrs = append(
				allErrs,
				field.Forbidden(
					fldPath.Child("version"),
					fmt.Sprintf("version cannot be increased from %q to %q", oldVersion, inVersion),
				),
			)
		}

		// If the ClusterClass referenced in the Topology has changed compatibility checks are needed.
		if oldCluster.Spec.Topology.Class != newCluster.Spec.Topology.Class {
			// Check to see if the ClusterClass referenced in the old version of the Cluster exists.
			oldClusterClass, err := webhook.pollClusterClassForCluster(ctx, oldCluster)
			if err != nil {
				allErrs = append(
					allErrs, field.Forbidden(
						fldPath.Child("class"),
						fmt.Sprintf("ClusterClass with name %q could not be found, change from class %[1]q to class %q cannot be validated",
							oldCluster.Spec.Topology.Class, newCluster.Spec.Topology.Class)))

				// Return early with errors if the ClusterClass can't be retrieved.
				return allErrs
			}

			// Check if the new and old ClusterClasses are compatible with one another.
			allErrs = append(allErrs, check.ClusterClassesAreCompatible(oldClusterClass, clusterClass)...)
		}
	}
	return allErrs
}

func validateMachineHealthChecks(cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	if cluster.Spec.Topology.ControlPlane.MachineHealthCheck != nil {
		fldPath := field.NewPath("spec", "topology", "controlPlane", "machineHealthCheck")

		// Validate ControlPlane MachineHealthCheck if defined.
		if !cluster.Spec.Topology.ControlPlane.MachineHealthCheck.MachineHealthCheckClass.IsZero() {
			// Ensure ControlPlane does not define a MachineHealthCheck if the ClusterClass does not define MachineInfrastructure.
			if clusterClass.Spec.ControlPlane.MachineInfrastructure == nil {
				allErrs = append(allErrs, field.Forbidden(
					fldPath,
					"can be set only if spec.controlPlane.machineInfrastructure is set in ClusterClass",
				))
			}
			allErrs = append(allErrs, validateMachineHealthCheckClass(fldPath, cluster.Namespace,
				&cluster.Spec.Topology.ControlPlane.MachineHealthCheck.MachineHealthCheckClass)...)
		}

		// If MachineHealthCheck is explicitly enabled then make sure that a MachineHealthCheck definition is
		// available either in the Cluster topology or in the ClusterClass.
		// (One of these definitions will be used in the controller to create the MachineHealthCheck)

		// Check if the machineHealthCheck is explicitly enabled in the ControlPlaneTopology.
		if cluster.Spec.Topology.ControlPlane.MachineHealthCheck.Enable != nil && *cluster.Spec.Topology.ControlPlane.MachineHealthCheck.Enable {
			// Ensure the MHC is defined in at least one of the ControlPlaneTopology of the Cluster or the ControlPlaneClass of the ClusterClass.
			if cluster.Spec.Topology.ControlPlane.MachineHealthCheck.MachineHealthCheckClass.IsZero() && clusterClass.Spec.ControlPlane.MachineHealthCheck == nil {
				allErrs = append(allErrs, field.Forbidden(
					fldPath.Child("enable"),
					fmt.Sprintf("cannot be set to %t as MachineHealthCheck definition is not available in the Cluster topology or the ClusterClass", *cluster.Spec.Topology.ControlPlane.MachineHealthCheck.Enable),
				))
			}
		}
	}

	if cluster.Spec.Topology.Workers != nil {
		for i, md := range cluster.Spec.Topology.Workers.MachineDeployments {
			if md.MachineHealthCheck != nil {
				fldPath := field.NewPath("spec", "topology", "workers", "machineDeployments", "machineHealthCheck").Index(i)

				// Validate the MachineDeployment MachineHealthCheck if defined.
				if !md.MachineHealthCheck.MachineHealthCheckClass.IsZero() {
					allErrs = append(allErrs, validateMachineHealthCheckClass(fldPath, cluster.Namespace,
						&md.MachineHealthCheck.MachineHealthCheckClass)...)
				}

				// If MachineHealthCheck is explicitly enabled then make sure that a MachineHealthCheck definition is
				// available either in the Cluster topology or in the ClusterClass.
				// (One of these definitions will be used in the controller to create the MachineHealthCheck)
				mdClass := machineDeploymentClassOfName(clusterClass, md.Class)
				if mdClass != nil { // Note: we skip handling the nil case here as it is already handled in previous validations.
					// Check if the machineHealthCheck is explicitly enabled in the machineDeploymentTopology.
					if md.MachineHealthCheck.Enable != nil && *md.MachineHealthCheck.Enable {
						// Ensure the MHC is defined in at least one of the MachineDeploymentTopology of the Cluster or the MachineDeploymentClass of the ClusterClass.
						if md.MachineHealthCheck.MachineHealthCheckClass.IsZero() && mdClass.MachineHealthCheck == nil {
							allErrs = append(allErrs, field.Forbidden(
								fldPath.Child("enable"),
								fmt.Sprintf("cannot be set to %t as MachineHealthCheck definition is not available in the Cluster topology or the ClusterClass", *md.MachineHealthCheck.Enable),
							))
						}
					}
				}
			}
		}
	}

	return allErrs
}

// machineDeploymentClassOfName find a MachineDeploymentClass of the given name in the provided ClusterClass.
// Returns nil if it can not find one.
// TODO: Check if there is already a helper function that can do this.
func machineDeploymentClassOfName(clusterClass *clusterv1.ClusterClass, name string) *clusterv1.MachineDeploymentClass {
	for _, mdClass := range clusterClass.Spec.Workers.MachineDeployments {
		if mdClass.Class == name {
			return &mdClass
		}
	}
	return nil
}

// validateCIDRBlocks ensures the passed CIDR is valid.
func validateCIDRBlocks(fldPath *field.Path, cidrs []string) field.ErrorList {
	var allErrs field.ErrorList
	for i, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Index(i),
				cidr,
				err.Error()))
		}
	}
	return allErrs
}

// DefaultVariables defaults variables in the Cluster based on information in the ClusterClass.
func DefaultVariables(cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	if cluster == nil {
		return field.ErrorList{field.InternalError(field.NewPath(""), errors.New("Cluster can not be nil"))}
	}
	if clusterClass == nil {
		return field.ErrorList{field.InternalError(field.NewPath(""), errors.New("ClusterClass can not be nil"))}
	}

	defaultedVariables, errs := variables.DefaultClusterVariables(cluster.Spec.Topology.Variables,
		//TODO: Update this function to directly us ClusterClassStatusVariables to take care of multiple definitions per variable name.
		clusterClassStatusVariablesToVariables(clusterClass.Status.Variables),
		field.NewPath("spec", "topology", "variables"))
	if len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	} else {
		cluster.Spec.Topology.Variables = defaultedVariables
	}

	if cluster.Spec.Topology.Workers != nil {
		for i, md := range cluster.Spec.Topology.Workers.MachineDeployments {
			// Continue if there are no variable overrides.
			if md.Variables == nil || len(md.Variables.Overrides) == 0 {
				continue
			}
			defaultedVariables, errs := variables.DefaultMachineDeploymentVariables(md.Variables.Overrides,
				//TODO: Update this function to directly us ClusterClassStatusVariables to take care of multiple definitions per variable name.
				clusterClassStatusVariablesToVariables(clusterClass.Status.Variables),
				field.NewPath("spec", "topology", "workers", "machineDeployments").Index(i).Child("variables", "overrides"))
			if len(errs) > 0 {
				allErrs = append(allErrs, errs...)
			} else {
				md.Variables.Overrides = defaultedVariables
			}
		}
	}
	return allErrs
}

// ValidateClusterForClusterClass uses information in the ClusterClass to validate the Cluster.
func ValidateClusterForClusterClass(cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if cluster == nil {
		return field.ErrorList{field.InternalError(field.NewPath(""), errors.New("Cluster can not be nil"))}
	}
	if clusterClass == nil {
		return field.ErrorList{field.InternalError(field.NewPath(""), errors.New("ClusterClass can not be nil"))}
	}
	allErrs = append(allErrs, check.MachineDeploymentTopologiesAreValidAndDefinedInClusterClass(cluster, clusterClass)...)

	// validate the MachineHealthChecks defined in the cluster topology
	allErrs = append(allErrs, validateMachineHealthChecks(cluster, clusterClass)...)

	// Check if the variables defined in the ClusterClass are valid.
	allErrs = append(allErrs, variables.ValidateClusterVariables(cluster.Spec.Topology.Variables,
		//TODO: Update this function to directly us ClusterClassStatusVariables to take care of multiple definitions per variable name.
		clusterClassStatusVariablesToVariables(clusterClass.Status.Variables),
		fldPath.Child("variables"))...)
	if true || len(allErrs) > 0 {
		ccJSON, err := json.Marshal(clusterClass)
		if err != nil {
			fmt.Printf("Debug: unexpected err:\n%v\n", err)
		}
		fmt.Printf("Debug:\n%v\n", ccJSON)
	}

	if cluster.Spec.Topology.Workers != nil {
		for i, md := range cluster.Spec.Topology.Workers.MachineDeployments {
			// Continue if there are no variable overrides.
			if md.Variables == nil || len(md.Variables.Overrides) == 0 {
				continue
			}
			allErrs = append(allErrs, variables.ValidateMachineDeploymentVariables(md.Variables.Overrides,
				//TODO: Update this function to directly us ClusterClassStatusVariables to take care of multiple definitions per variable name.
				clusterClassStatusVariablesToVariables(clusterClass.Status.Variables),
				fldPath.Child("workers", "machineDeployments").Index(i).Child("variables", "overrides"))...)
		}
	}

	return allErrs
}

// pollClusterClassForCluster will retry getting the ClusterClass referenced in the Cluster for two seconds.
func (webhook *Cluster) pollClusterClassForCluster(ctx context.Context, cluster *clusterv1.Cluster) (*clusterv1.ClusterClass, error) {
	clusterClass := &clusterv1.ClusterClass{}
	var clusterClassGetErr error
	_ = util.PollImmediate(200*time.Millisecond, 2*time.Second, func() (bool, error) {
		if clusterClassGetErr = webhook.Client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.Spec.Topology.Class}, clusterClass); clusterClassGetErr != nil {
			return false, nil //nolint:nilerr
		}

		// Return an error if the ClusterClass has not successfully reconciled because variables aren't correctly
		// reconciled.
		// TODO: Decide whether to check generation here. This requires creating templates before creating the Cluster and
		// may interfere with the way clusterctl move works.
		if !conditions.Has(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition) ||
			conditions.IsFalse(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition) {
			clusterClassGetErr = errors.New("ClusterClass is not up to date. If this persists check ClusterClass status")
			return false, nil
		}
		return true, nil
	})
	return clusterClass, clusterClassGetErr
}

// TODO: This function will not be needed once per-definition defaulting and validation is implemented.
func clusterClassStatusVariablesToVariables(vars []clusterv1.ClusterClassStatusVariable) []clusterv1.ClusterClassVariable {
	var ccVars []clusterv1.ClusterClassVariable
	for _, v := range vars {
		for _, d := range v.Definitions {
			ccVars = append(ccVars, clusterv1.ClusterClassVariable{
				Required: d.Required,
				Schema:   d.Schema,
				Name:     v.Name,
			})
		}
	}
	return ccVars
}
