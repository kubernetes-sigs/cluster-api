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

	"github.com/blang/semver"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/topology/check"
	"sigs.k8s.io/cluster-api/internal/topology/variables"
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
		clusterClass, err := webhook.getClusterClassForCluster(ctx, cluster)
		if err != nil {
			// Return early with errors if the ClusterClass can't be retrieved.
			return apierrors.NewInternalError(errors.Wrapf(err, "Cluster %s can't be validated. ClusterClass %s can not be retrieved.", cluster.Name, cluster.Spec.Topology.Class))
		}

		// We gather all defaulting errors and return them together.
		var allErrs field.ErrorList

		defaultedVariables, errs := variables.DefaultClusterVariables(cluster.Spec.Topology.Variables, clusterClass.Spec.Variables,
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

				defaultedVariables, errs := variables.DefaultMachineDeploymentVariables(md.Variables.Overrides, clusterClass.Spec.Variables,
					field.NewPath("spec", "topology", "workers", "machineDeployments").Index(i).Child("variables", "overrides"))
				if len(errs) > 0 {
					allErrs = append(allErrs, errs...)
				} else {
					md.Variables.Overrides = defaultedVariables
				}
			}
		}

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
func (webhook *Cluster) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}

func (webhook *Cluster) validate(ctx context.Context, oldCluster, newCluster *clusterv1.Cluster) error {
	var allErrs field.ErrorList
	if newCluster.Spec.InfrastructureRef != nil && newCluster.Spec.InfrastructureRef.Namespace != newCluster.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "infrastructureRef", "namespace"),
				newCluster.Spec.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if newCluster.Spec.ControlPlaneRef != nil && newCluster.Spec.ControlPlaneRef.Namespace != newCluster.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "controlPlaneRef", "namespace"),
				newCluster.Spec.ControlPlaneRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	// Validate the managed topology, if defined.
	if newCluster.Spec.Topology != nil {
		allErrs = append(allErrs, webhook.validateTopology(ctx, oldCluster, newCluster)...)
	}

	// On update.
	if oldCluster != nil {
		// Error if the update moves the cluster from Managed to Unmanaged i.e. the managed topology is removed on update.
		if oldCluster.Spec.Topology != nil && newCluster.Spec.Topology == nil {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "topology"),
				"cannot be removed from an existing Cluster",
			))
		}
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), newCluster.Name, allErrs)
	}
	return nil
}

func (webhook *Cluster) validateTopology(ctx context.Context, oldCluster, newCluster *clusterv1.Cluster) field.ErrorList {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the web hook
	// must prevent the usage of Cluster.Topology in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return field.ErrorList{
			field.Forbidden(
				field.NewPath("spec", "topology"),
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
				field.NewPath("spec", "topology", "class"),
				"class cannot be empty",
			),
		)
	}

	// version should be valid.
	if !version.KubeSemver.MatchString(newCluster.Spec.Topology.Version) {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "topology", "version"),
				newCluster.Spec.Topology.Version,
				"version must be a valid semantic version",
			),
		)
	}

	// clusterClass must exist.
	clusterClass := &clusterv1.ClusterClass{}
	// Check to see if the ClusterClass referenced in the Cluster currently exists.
	if err := webhook.Client.Get(ctx, client.ObjectKey{Namespace: newCluster.Namespace, Name: newCluster.Spec.Topology.Class}, clusterClass); err != nil {
		allErrs = append(
			allErrs, field.Invalid(
				field.NewPath("spec", "topology", "class"),
				newCluster.Name,
				fmt.Sprintf("ClusterClass with name %q could not be found", newCluster.Spec.Topology.Class)))
		return allErrs
	}

	allErrs = append(allErrs, check.MachineDeploymentTopologiesAreValidAndDefinedInClusterClass(newCluster, clusterClass)...)

	// Check if the variables defined in the ClusterClass are valid.
	allErrs = append(allErrs, variables.ValidateClusterVariables(newCluster.Spec.Topology.Variables, clusterClass.Spec.Variables,
		field.NewPath("spec", "topology", "variables"))...)

	if newCluster.Spec.Topology.Workers != nil {
		for i, md := range newCluster.Spec.Topology.Workers.MachineDeployments {
			// Continue if there are no variable overrides.
			if md.Variables == nil || len(md.Variables.Overrides) == 0 {
				continue
			}

			allErrs = append(allErrs, variables.ValidateTopLevelClusterVariablesExist(md.Variables.Overrides, newCluster.Spec.Topology.Variables,
				field.NewPath("spec", "topology", "workers", "machineDeployments").Index(i).Child("variables", "overrides"))...)
			allErrs = append(allErrs, variables.ValidateMachineDeploymentVariables(md.Variables.Overrides, clusterClass.Spec.Variables,
				field.NewPath("spec", "topology", "workers", "machineDeployments").Index(i).Child("variables", "overrides"))...)
		}
	}

	if oldCluster != nil { // On update
		// Topology or Class can not be added on update.
		if oldCluster.Spec.Topology == nil || oldCluster.Spec.Topology.Class == "" {
			allErrs = append(
				allErrs,
				field.Forbidden(
					field.NewPath("spec", "topology", "class"),
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
					field.NewPath("spec", "topology", "version"),
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
					field.NewPath("spec", "topology", "version"),
					oldCluster.Spec.Topology.Version,
					fmt.Sprintf("old version %q cannot be compared with %q", oldVersion, inVersion),
				),
			)
		}
		if inVersion.NE(semver.Version{}) && oldVersion.NE(semver.Version{}) && version.Compare(inVersion, oldVersion, version.WithBuildTags()) == -1 {
			allErrs = append(
				allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "version"),
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
					field.NewPath("spec", "topology", "version"),
					fmt.Sprintf("version cannot be increased from %q to %q", oldVersion, inVersion),
				),
			)
		}

		// If the ClusterClass referenced in the Topology has changed compatibility checks are needed.
		if oldCluster.Spec.Topology.Class != newCluster.Spec.Topology.Class {
			// Check to see if the ClusterClass referenced in the old version of the Cluster exists.
			oldClusterClass, err := webhook.getClusterClassForCluster(ctx, oldCluster)
			if err != nil {
				allErrs = append(
					allErrs, field.Forbidden(
						field.NewPath("spec", "topology", "class"),
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

func (webhook *Cluster) getClusterClassForCluster(ctx context.Context, cluster *clusterv1.Cluster) (*clusterv1.ClusterClass, error) {
	clusterClass := &clusterv1.ClusterClass{}
	// Check to see if the ClusterClass referenced in the old version of the Cluster exists.
	if err := webhook.Client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.Spec.Topology.Class}, clusterClass); err != nil {
		return nil, err
	}
	return clusterClass, nil
}
