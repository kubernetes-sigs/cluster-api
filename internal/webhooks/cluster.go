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
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/topology/check"
	"sigs.k8s.io/cluster-api/internal/topology/variables"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/version"
)

// SetupWebhookWithManager sets up Cluster webhooks.
func (webhook *Cluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if webhook.decoder == nil {
		webhook.decoder = admission.NewDecoder(mgr.GetScheme())
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-cluster-x-k8s-io-v1beta1-cluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=validation.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-cluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=default.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ClusterCacheTrackerReader is a scoped-down interface from ClusterCacheTracker that only allows to get a reader client.
type ClusterCacheTrackerReader interface {
	GetReader(ctx context.Context, cluster client.ObjectKey) (client.Reader, error)
}

// Cluster implements a validating and defaulting webhook for Cluster.
type Cluster struct {
	Client  client.Reader
	Tracker ClusterCacheTrackerReader

	decoder admission.Decoder
}

var _ webhook.CustomDefaulter = &Cluster{}
var _ webhook.CustomValidator = &Cluster{}

var errClusterClassNotReconciled = errors.New("ClusterClass is not successfully reconciled")

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

		if cluster.GetClassKey().Name == "" {
			allErrs = append(
				allErrs,
				field.Required(
					field.NewPath("spec", "topology", "class"),
					"class cannot be empty",
				),
			)
			return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), cluster.Name, allErrs)
		}

		if cluster.Spec.Topology.ControlPlane.MachineHealthCheck != nil &&
			cluster.Spec.Topology.ControlPlane.MachineHealthCheck.MachineHealthCheckClass.RemediationTemplate != nil &&
			cluster.Spec.Topology.ControlPlane.MachineHealthCheck.MachineHealthCheckClass.RemediationTemplate.Namespace == "" {
			cluster.Spec.Topology.ControlPlane.MachineHealthCheck.MachineHealthCheckClass.RemediationTemplate.Namespace = cluster.Namespace
		}

		if cluster.Spec.Topology.Workers != nil {
			for i := range cluster.Spec.Topology.Workers.MachineDeployments {
				md := cluster.Spec.Topology.Workers.MachineDeployments[i]
				if md.MachineHealthCheck != nil &&
					md.MachineHealthCheck.MachineHealthCheckClass.RemediationTemplate != nil &&
					md.MachineHealthCheck.MachineHealthCheckClass.RemediationTemplate.Namespace == "" {
					md.MachineHealthCheck.MachineHealthCheckClass.RemediationTemplate.Namespace = cluster.Namespace
				}
			}
		}

		clusterClass, err := webhook.pollClusterClassForCluster(ctx, cluster)
		if err != nil {
			// If the ClusterClass can't be found or is not up to date ignore the error.
			if apierrors.IsNotFound(err) || errors.Is(err, errClusterClassNotReconciled) {
				return nil
			}
			return apierrors.NewInternalError(errors.Wrapf(err, "Cluster %s can't be defaulted. ClusterClass %s can not be retrieved", cluster.Name, cluster.GetClassKey().Name))
		}

		// Validate cluster class variables transitions that may be enforced by CEL validation rules on variables.
		// If no request found in context, then this has not come via a webhook request, so skip validation of old cluster.
		var oldCluster *clusterv1.Cluster
		req, err := admission.RequestFromContext(ctx)

		if err == nil && len(req.OldObject.Raw) > 0 {
			oldCluster = &clusterv1.Cluster{}
			if err := webhook.decoder.DecodeRaw(req.OldObject, oldCluster); err != nil {
				return apierrors.NewBadRequest(errors.Wrap(err, "failed to decode old cluster object").Error())
			}
		}

		// Doing both defaulting and validating here prevents a race condition where the ClusterClass could be
		// different in the defaulting and validating webhook.
		allErrs = append(allErrs, DefaultAndValidateVariables(ctx, cluster, oldCluster, clusterClass)...)
		if len(allErrs) > 0 {
			return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), cluster.Name, allErrs)
		}
	}
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Cluster) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cluster, ok := obj.(*clusterv1.Cluster)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}
	return webhook.validate(ctx, nil, cluster)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Cluster) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newCluster, ok := newObj.(*clusterv1.Cluster)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", newObj))
	}
	oldCluster, ok := oldObj.(*clusterv1.Cluster)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", oldObj))
	}
	return webhook.validate(ctx, oldCluster, newCluster)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Cluster) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *Cluster) validate(ctx context.Context, oldCluster, newCluster *clusterv1.Cluster) (admission.Warnings, error) {
	var allErrs field.ErrorList
	var allWarnings admission.Warnings
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
		topologyWarnings, topologyErrs := webhook.validateTopology(ctx, oldCluster, newCluster, topologyPath)
		allWarnings = append(allWarnings, topologyWarnings...)
		allErrs = append(allErrs, topologyErrs...)
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
		return allWarnings, apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), newCluster.Name, allErrs)
	}
	return allWarnings, nil
}

func (webhook *Cluster) validateTopology(ctx context.Context, oldCluster, newCluster *clusterv1.Cluster, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	var allWarnings admission.Warnings

	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the web hook
	// must prevent the usage of Cluster.Topology in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return allWarnings, field.ErrorList{
			field.Forbidden(
				fldPath,
				"can be set only if the ClusterTopology feature flag is enabled",
			),
		}
	}

	var allErrs field.ErrorList

	// class should be defined.
	if newCluster.GetClassKey().Name == "" {
		allErrs = append(
			allErrs,
			field.Required(
				fldPath.Child("class"),
				"class cannot be empty",
			),
		)
		// Return early if there is no defined class to validate.
		return allWarnings, allErrs
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

	// metadata in topology should be valid
	allErrs = append(allErrs, validateTopologyMetadata(newCluster.Spec.Topology, fldPath)...)

	// ensure deprecationFrom is not set
	allErrs = append(allErrs, validateTopologyDefinitionFrom(newCluster.Spec.Topology, fldPath)...)

	// upgrade concurrency should be a numeric value.
	if concurrency, ok := newCluster.Annotations[clusterv1.ClusterTopologyUpgradeConcurrencyAnnotation]; ok {
		concurrencyAnnotationField := field.NewPath("metadata", "annotations", clusterv1.ClusterTopologyUpgradeConcurrencyAnnotation)
		concurrencyInt, err := strconv.Atoi(concurrency)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(
				concurrencyAnnotationField,
				concurrency,
				errors.Wrap(err, "could not parse the value of the annotation").Error(),
			))
		} else if concurrencyInt < 1 {
			allErrs = append(allErrs, field.Invalid(
				concurrencyAnnotationField,
				concurrency,
				"value cannot be less than 1",
			))
		}
	}

	// Get the ClusterClass referenced in the Cluster.
	clusterClass, warnings, clusterClassPollErr := webhook.validateClusterClassExistsAndIsReconciled(ctx, newCluster)
	// If the error is anything other than "NotFound" or "NotReconciled" return all errors.
	if clusterClassPollErr != nil && !(apierrors.IsNotFound(clusterClassPollErr) || errors.Is(clusterClassPollErr, errClusterClassNotReconciled)) {
		allErrs = append(
			allErrs, field.InternalError(
				fldPath.Child("class"),
				clusterClassPollErr))
		return allWarnings, allErrs
	}

	// Add the warnings if no error was returned.
	allWarnings = append(allWarnings, warnings...)

	// If there's no error validate the Cluster based on the ClusterClass.
	if clusterClassPollErr == nil {
		allErrs = append(allErrs, ValidateClusterForClusterClass(newCluster, clusterClass)...)
	}

	// Validate the Cluster and associated ClusterClass' autoscaler annotations.
	allErrs = append(allErrs, validateAutoscalerAnnotationsForCluster(newCluster, clusterClass)...)

	if oldCluster != nil { // On update
		// The ClusterClass must exist to proceed with update validation. Return an error if the ClusterClass was
		// not found.
		if apierrors.IsNotFound(clusterClassPollErr) {
			allErrs = append(
				allErrs, field.InternalError(
					fldPath.Child("class"),
					clusterClassPollErr))
			return allWarnings, allErrs
		}

		// Topology or Class can not be added on update unless ClusterTopologyUnsafeUpdateClassNameAnnotation is set.
		if oldCluster.Spec.Topology == nil || oldCluster.GetClassKey().Name == "" {
			if _, ok := newCluster.Annotations[clusterv1.ClusterTopologyUnsafeUpdateClassNameAnnotation]; ok {
				return allWarnings, allErrs
			}

			allErrs = append(
				allErrs,
				field.Forbidden(
					fldPath.Child("class"),
					"class cannot be set on an existing Cluster",
				),
			)
			// return early here if there is no class to compare.
			return allWarnings, allErrs
		}

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
					"old version must be a valid semantic version",
				),
			)
		}

		if _, ok := newCluster.GetAnnotations()[clusterv1.ClusterTopologyUnsafeUpdateVersionAnnotation]; ok {
			log := ctrl.LoggerFrom(ctx)
			warningMsg := fmt.Sprintf("Skipping version validation for Cluster because annotation %q is set.", clusterv1.ClusterTopologyUnsafeUpdateVersionAnnotation)
			log.Info(warningMsg)
			allWarnings = append(allWarnings, warningMsg)
		} else {
			if err := webhook.validateTopologyVersion(ctx, fldPath.Child("version"), newCluster.Spec.Topology.Version, inVersion, oldVersion, oldCluster); err != nil {
				allErrs = append(allErrs, err)
			}
		}

		// If the ClusterClass referenced in the Topology has changed compatibility checks are needed.
		if oldCluster.GetClassKey() != newCluster.GetClassKey() {
			// Check to see if the ClusterClass referenced in the old version of the Cluster exists.
			oldClusterClass, err := webhook.pollClusterClassForCluster(ctx, oldCluster)
			if err != nil {
				allErrs = append(
					allErrs, field.Forbidden(
						fldPath.Child("class"),
						fmt.Sprintf("valid ClusterClass with name %q could not be retrieved, change from class %[1]q to class %q cannot be validated. Error: %s",
							oldCluster.GetClassKey(), newCluster.GetClassKey(), err.Error())))

				// Return early with errors if the ClusterClass can't be retrieved.
				return allWarnings, allErrs
			}

			// Check if the new and old ClusterClasses are compatible with one another.
			allErrs = append(allErrs, check.ClusterClassesAreCompatible(oldClusterClass, clusterClass)...)
		}
	}

	return allWarnings, allErrs
}

func (webhook *Cluster) validateTopologyVersion(ctx context.Context, fldPath *field.Path, fldValue string, inVersion, oldVersion semver.Version, oldCluster *clusterv1.Cluster) *field.Error {
	// Version could only be increased.
	if inVersion.NE(semver.Version{}) && oldVersion.NE(semver.Version{}) && version.Compare(inVersion, oldVersion, version.WithBuildTags()) == -1 {
		return field.Invalid(
			fldPath,
			fldValue,
			fmt.Sprintf("version cannot be decreased from %q to %q", oldVersion, inVersion),
		)
	}

	// A +2 minor version upgrade is not allowed.
	ceilVersion := semver.Version{
		Major: oldVersion.Major,
		Minor: oldVersion.Minor + 2,
		Patch: 0,
	}
	if inVersion.GTE(ceilVersion) {
		return field.Invalid(
			fldPath,
			fldValue,
			fmt.Sprintf("version cannot be increased from %q to %q", oldVersion, inVersion),
		)
	}

	// Only check the following cases if the minor version increases by 1 (we already return above for >= 2).
	ceilVersion = semver.Version{
		Major: oldVersion.Major,
		Minor: oldVersion.Minor + 1,
		Patch: 0,
	}

	// Return early if its not a minor version upgrade.
	if !inVersion.GTE(ceilVersion) {
		return nil
	}

	allErrs := []error{}
	// minor version cannot be increased if control plane is upgrading or not yet on the current version
	if err := validateTopologyControlPlaneVersion(ctx, webhook.Client, oldCluster, oldVersion); err != nil {
		allErrs = append(allErrs, fmt.Errorf("blocking version update due to ControlPlane version check: %v", err))
	}

	// minor version cannot be increased if MachineDeployments are upgrading or not yet on the current version
	if err := validateTopologyMachineDeploymentVersions(ctx, webhook.Client, oldCluster, oldVersion); err != nil {
		allErrs = append(allErrs, fmt.Errorf("blocking version update due to MachineDeployment version check: %v", err))
	}

	// minor version cannot be increased if MachinePools are upgrading or not yet on the current version
	if err := validateTopologyMachinePoolVersions(ctx, webhook.Client, webhook.Tracker, oldCluster, oldVersion); err != nil {
		allErrs = append(allErrs, fmt.Errorf("blocking version update due to MachinePool version check: %v", err))
	}

	if len(allErrs) > 0 {
		return field.Invalid(
			fldPath,
			fldValue,
			fmt.Sprintf("minor version update cannot happen at this time: %v", kerrors.NewAggregate(allErrs)),
		)
	}

	return nil
}

func validateTopologyControlPlaneVersion(ctx context.Context, ctrlClient client.Reader, oldCluster *clusterv1.Cluster, oldVersion semver.Version) error {
	cp, err := external.Get(ctx, ctrlClient, oldCluster.Spec.ControlPlaneRef, oldCluster.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to get ControlPlane object")
	}

	cpVersionString, err := contract.ControlPlane().Version().Get(cp)
	if err != nil {
		return errors.Wrap(err, "failed to get ControlPlane version")
	}

	cpVersion, err := semver.ParseTolerant(*cpVersionString)
	if err != nil {
		// NOTE: this should never happen. Nevertheless, handling this for extra caution.
		return errors.New("failed to parse version of ControlPlane")
	}
	if cpVersion.NE(oldVersion) {
		return fmt.Errorf("ControlPlane version %q does not match the current version %q", cpVersion, oldVersion)
	}

	provisioning, err := contract.ControlPlane().IsProvisioning(cp)
	if err != nil {
		return errors.Wrap(err, "failed to check if ControlPlane is provisioning")
	}

	if provisioning {
		return errors.New("ControlPlane is currently provisioning")
	}

	upgrading, err := contract.ControlPlane().IsUpgrading(cp)
	if err != nil {
		return errors.Wrap(err, "failed to check if ControlPlane is upgrading")
	}

	if upgrading {
		return errors.New("ControlPlane is still completing a previous upgrade")
	}

	return nil
}

func validateTopologyMachineDeploymentVersions(ctx context.Context, ctrlClient client.Reader, oldCluster *clusterv1.Cluster, oldVersion semver.Version) error {
	// List all the machine deployments in the current cluster and in a managed topology.
	// FROM: current_state.go getCurrentMachineDeploymentState
	mds := &clusterv1.MachineDeploymentList{}
	err := ctrlClient.List(ctx, mds,
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:          oldCluster.Name,
			clusterv1.ClusterTopologyOwnedLabel: "",
		},
		client.InNamespace(oldCluster.Namespace),
	)
	if err != nil {
		return errors.Wrap(err, "failed to read MachineDeployments for managed topology")
	}

	if len(mds.Items) == 0 {
		return nil
	}

	mdUpgradingNames := []string{}

	for i := range mds.Items {
		md := &mds.Items[i]

		mdVersion, err := semver.ParseTolerant(*md.Spec.Template.Spec.Version)
		if err != nil {
			// NOTE: this should never happen. Nevertheless, handling this for extra caution.
			return errors.Wrapf(err, "failed to parse MachineDeployment's %q version %q", klog.KObj(md), *md.Spec.Template.Spec.Version)
		}

		if mdVersion.NE(oldVersion) {
			mdUpgradingNames = append(mdUpgradingNames, md.Name)
			continue
		}

		upgrading, err := check.IsMachineDeploymentUpgrading(ctx, ctrlClient, md)
		if err != nil {
			return errors.Wrap(err, "failed to check if MachineDeployment is upgrading")
		}
		if upgrading {
			mdUpgradingNames = append(mdUpgradingNames, md.Name)
		}
	}

	if len(mdUpgradingNames) > 0 {
		return fmt.Errorf("there are MachineDeployments still completing a previous upgrade: [%s]", strings.Join(mdUpgradingNames, ", "))
	}

	return nil
}

func validateTopologyMachinePoolVersions(ctx context.Context, ctrlClient client.Reader, tracker ClusterCacheTrackerReader, oldCluster *clusterv1.Cluster, oldVersion semver.Version) error {
	// List all the machine pools in the current cluster and in a managed topology.
	// FROM: current_state.go getCurrentMachinePoolState
	mps := &expv1.MachinePoolList{}
	err := ctrlClient.List(ctx, mps,
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:          oldCluster.Name,
			clusterv1.ClusterTopologyOwnedLabel: "",
		},
		client.InNamespace(oldCluster.Namespace),
	)
	if err != nil {
		return errors.Wrap(err, "failed to read MachinePools for managed topology")
	}

	// Return early
	if len(mps.Items) == 0 {
		return nil
	}

	wlClient, err := tracker.GetReader(ctx, client.ObjectKeyFromObject(oldCluster))
	if err != nil {
		return errors.Wrap(err, "unable to get client for workload cluster")
	}

	mpUpgradingNames := []string{}

	for i := range mps.Items {
		mp := &mps.Items[i]

		mpVersion, err := semver.ParseTolerant(*mp.Spec.Template.Spec.Version)
		if err != nil {
			// NOTE: this should never happen. Nevertheless, handling this for extra caution.
			return errors.Wrapf(err, "failed to parse MachinePool's %q version %q", klog.KObj(mp), *mp.Spec.Template.Spec.Version)
		}

		if mpVersion.NE(oldVersion) {
			mpUpgradingNames = append(mpUpgradingNames, mp.Name)
			continue
		}

		upgrading, err := check.IsMachinePoolUpgrading(ctx, wlClient, mp)
		if err != nil {
			return errors.Wrap(err, "failed to check if MachinePool is upgrading")
		}
		if upgrading {
			mpUpgradingNames = append(mpUpgradingNames, mp.Name)
		}
	}

	if len(mpUpgradingNames) > 0 {
		return fmt.Errorf("there are MachinePools still completing a previous upgrade: [%s]", strings.Join(mpUpgradingNames, ", "))
	}

	return nil
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
		for i := range cluster.Spec.Topology.Workers.MachineDeployments {
			md := cluster.Spec.Topology.Workers.MachineDeployments[i]
			if md.MachineHealthCheck != nil {
				fldPath := field.NewPath("spec", "topology", "workers", "machineDeployments").Key(md.Name).Child("machineHealthCheck")

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

// DefaultAndValidateVariables defaults and validates variables in the Cluster and MachineDeployment/MachinePool topologies based
// on the definitions in the ClusterClass.
func DefaultAndValidateVariables(ctx context.Context, cluster, oldCluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, DefaultVariables(cluster, clusterClass)...)

	// Capture variables from old cluster if it is present to be used in validation for transitions that may be specified
	// via CEL validation rules.
	var (
		oldClusterVariables, oldCPOverrides []clusterv1.ClusterVariable
		oldMDVariables                      map[string][]clusterv1.ClusterVariable
		oldMPVariables                      map[string][]clusterv1.ClusterVariable
	)
	if oldCluster != nil {
		oldClusterVariables = oldCluster.Spec.Topology.Variables
		if oldCluster.Spec.Topology.ControlPlane.Variables != nil {
			oldCPOverrides = oldCluster.Spec.Topology.ControlPlane.Variables.Overrides
		}

		if oldCluster.Spec.Topology.Workers != nil {
			oldMDVariables = make(map[string][]clusterv1.ClusterVariable, len(oldCluster.Spec.Topology.Workers.MachineDeployments))
			for _, md := range oldCluster.Spec.Topology.Workers.MachineDeployments {
				if md.Variables != nil {
					oldMDVariables[md.Name] = md.Variables.Overrides
				}
			}

			oldMPVariables = make(map[string][]clusterv1.ClusterVariable, len(oldCluster.Spec.Topology.Workers.MachinePools))
			for _, mp := range oldCluster.Spec.Topology.Workers.MachinePools {
				if mp.Variables != nil {
					oldMPVariables[mp.Name] = mp.Variables.Overrides
				}
			}
		}
	}

	// Variables must be validated in the defaulting webhook. Variable definitions are stored in the ClusterClass status
	// and are patched in the ClusterClass reconcile.

	// Validate cluster-wide variables.
	allErrs = append(allErrs, variables.ValidateClusterVariables(
		ctx,
		cluster.Spec.Topology.Variables,
		oldClusterVariables,
		clusterClass.Status.Variables,
		field.NewPath("spec", "topology", "variables"))...)

	// Validate ControlPlane variable overrides.
	if cluster.Spec.Topology.ControlPlane.Variables != nil && len(cluster.Spec.Topology.ControlPlane.Variables.Overrides) > 0 {
		allErrs = append(allErrs, variables.ValidateControlPlaneVariables(
			ctx,
			cluster.Spec.Topology.ControlPlane.Variables.Overrides,
			oldCPOverrides,
			clusterClass.Status.Variables,
			field.NewPath("spec", "topology", "controlPlane", "variables", "overrides"))...,
		)
	}

	if cluster.Spec.Topology.Workers != nil {
		// Validate MachineDeployment variable overrides.
		for _, md := range cluster.Spec.Topology.Workers.MachineDeployments {
			// Continue if there are no variable overrides.
			if md.Variables == nil || len(md.Variables.Overrides) == 0 {
				continue
			}
			allErrs = append(allErrs, variables.ValidateMachineVariables(
				ctx,
				md.Variables.Overrides,
				oldMDVariables[md.Name],
				clusterClass.Status.Variables,
				field.NewPath("spec", "topology", "workers", "machineDeployments").Key(md.Name).Child("variables", "overrides"))...,
			)
		}

		// Validate MachinePool variable overrides.
		for _, mp := range cluster.Spec.Topology.Workers.MachinePools {
			// Continue if there are no variable overrides.
			if mp.Variables == nil || len(mp.Variables.Overrides) == 0 {
				continue
			}
			allErrs = append(allErrs, variables.ValidateMachineVariables(
				ctx,
				mp.Variables.Overrides,
				oldMPVariables[mp.Name],
				clusterClass.Status.Variables,
				field.NewPath("spec", "topology", "workers", "machinePools").Key(mp.Name).Child("variables", "overrides"))...,
			)
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

	// Default cluster-wide variables.
	defaultedVariables, errs := variables.DefaultClusterVariables(cluster.Spec.Topology.Variables, clusterClass.Status.Variables,
		field.NewPath("spec", "topology", "variables"))
	if len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	} else {
		cluster.Spec.Topology.Variables = defaultedVariables
	}

	// Default ControlPlane variable overrides.
	if cluster.Spec.Topology.ControlPlane.Variables != nil && len(cluster.Spec.Topology.ControlPlane.Variables.Overrides) > 0 {
		defaultedVariables, errs := variables.DefaultMachineVariables(cluster.Spec.Topology.ControlPlane.Variables.Overrides, clusterClass.Status.Variables,
			field.NewPath("spec", "topology", "controlPlane", "variables", "overrides"))
		if len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		} else {
			cluster.Spec.Topology.ControlPlane.Variables.Overrides = defaultedVariables
		}
	}

	if cluster.Spec.Topology.Workers != nil {
		// Default MachineDeployment variable overrides.
		for _, md := range cluster.Spec.Topology.Workers.MachineDeployments {
			// Continue if there are no variable overrides.
			if md.Variables == nil || len(md.Variables.Overrides) == 0 {
				continue
			}
			defaultedVariables, errs := variables.DefaultMachineVariables(md.Variables.Overrides, clusterClass.Status.Variables,
				field.NewPath("spec", "topology", "workers", "machineDeployments").Key(md.Name).Child("variables", "overrides"))
			if len(errs) > 0 {
				allErrs = append(allErrs, errs...)
			} else {
				md.Variables.Overrides = defaultedVariables
			}
		}

		// Default MachinePool variable overrides.
		for _, mp := range cluster.Spec.Topology.Workers.MachinePools {
			// Continue if there are no variable overrides.
			if mp.Variables == nil || len(mp.Variables.Overrides) == 0 {
				continue
			}
			defaultedVariables, errs := variables.DefaultMachineVariables(mp.Variables.Overrides, clusterClass.Status.Variables,
				field.NewPath("spec", "topology", "workers", "machinePools").Key(mp.Name).Child("variables", "overrides"))
			if len(errs) > 0 {
				allErrs = append(allErrs, errs...)
			} else {
				mp.Variables.Overrides = defaultedVariables
			}
		}
	}
	return allErrs
}

// ValidateClusterForClusterClass uses information in the ClusterClass to validate the Cluster.
func ValidateClusterForClusterClass(cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	if cluster == nil {
		return field.ErrorList{field.InternalError(field.NewPath(""), errors.New("Cluster can not be nil"))}
	}
	if clusterClass == nil {
		return field.ErrorList{field.InternalError(field.NewPath(""), errors.New("ClusterClass can not be nil"))}
	}
	allErrs = append(allErrs, check.MachineDeploymentTopologiesAreValidAndDefinedInClusterClass(cluster, clusterClass)...)

	allErrs = append(allErrs, check.MachinePoolTopologiesAreValidAndDefinedInClusterClass(cluster, clusterClass)...)

	// Validate the MachineHealthChecks defined in the cluster topology.
	allErrs = append(allErrs, validateMachineHealthChecks(cluster, clusterClass)...)
	return allErrs
}

// validateClusterClassExistsAndIsReconciled will try to get the ClusterClass referenced in the Cluster. If it does not exist or is not reconciled it will add a warning.
// In any other case it will return an error.
func (webhook *Cluster) validateClusterClassExistsAndIsReconciled(ctx context.Context, newCluster *clusterv1.Cluster) (*clusterv1.ClusterClass, admission.Warnings, error) {
	var allWarnings admission.Warnings
	clusterClass, clusterClassPollErr := webhook.pollClusterClassForCluster(ctx, newCluster)
	if clusterClassPollErr != nil {
		// Add a warning if the Class does not exist or if it has not been successfully reconciled.
		switch {
		case apierrors.IsNotFound(clusterClassPollErr):
			allWarnings = append(allWarnings,
				fmt.Sprintf(
					"Cluster refers to ClusterClass %s, but this ClusterClass does not exist. "+
						"Cluster topology has not been fully validated. "+
						"The ClusterClass must be created to reconcile the Cluster", newCluster.GetClassKey()),
			)
		case errors.Is(clusterClassPollErr, errClusterClassNotReconciled):
			allWarnings = append(allWarnings,
				fmt.Sprintf(
					"Cluster refers to ClusterClass %s, but this ClusterClass hasn't been successfully reconciled. "+
						"Cluster topology has not been fully validated. "+
						"Please take a look at the ClusterClass status", newCluster.GetClassKey()),
			)
		// If there's any other error return a generic warning with the error message.
		default:
			allWarnings = append(allWarnings,
				fmt.Sprintf(
					"Cluster refers to ClusterClass %s, but this ClusterClass could not be retrieved. "+
						"Cluster topology has not been fully validated: %s", newCluster.GetClassKey(), clusterClassPollErr.Error()),
			)
		}
	}
	return clusterClass, allWarnings, clusterClassPollErr
}

// pollClusterClassForCluster will retry getting the ClusterClass referenced in the Cluster for two seconds.
func (webhook *Cluster) pollClusterClassForCluster(ctx context.Context, cluster *clusterv1.Cluster) (*clusterv1.ClusterClass, error) {
	clusterClass := &clusterv1.ClusterClass{}
	var clusterClassPollErr error
	_ = wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		if clusterClassPollErr = webhook.Client.Get(ctx, cluster.GetClassKey(), clusterClass); clusterClassPollErr != nil {
			return false, nil //nolint:nilerr
		}

		if clusterClassPollErr = clusterClassIsReconciled(clusterClass); clusterClassPollErr != nil {
			return false, nil //nolint:nilerr
		}
		clusterClassPollErr = nil
		return true, nil
	})
	if clusterClassPollErr != nil {
		return nil, clusterClassPollErr
	}
	return clusterClass, nil
}

// clusterClassIsReconciled returns errClusterClassNotReconciled if the ClusterClass has not successfully reconciled or if the
// ClusterClass variables have not been successfully reconciled.
func clusterClassIsReconciled(clusterClass *clusterv1.ClusterClass) error {
	// If the clusterClass metadata generation does not match the status observed generation, the ClusterClass has not been successfully reconciled.
	if clusterClass.Generation != clusterClass.Status.ObservedGeneration {
		return errClusterClassNotReconciled
	}
	// If the clusterClass does not have ClusterClassVariablesReconciled==True, the ClusterClass has not been successfully reconciled.
	if !conditions.Has(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition) ||
		conditions.IsFalse(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition) {
		return errClusterClassNotReconciled
	}
	return nil
}

func validateTopologyMetadata(topology *clusterv1.Topology, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, topology.ControlPlane.Metadata.Validate(fldPath.Child("controlPlane", "metadata"))...)
	if topology.Workers != nil {
		for _, md := range topology.Workers.MachineDeployments {
			allErrs = append(allErrs, md.Metadata.Validate(
				fldPath.Child("workers", "machineDeployments").Key(md.Name).Child("metadata"),
			)...)
		}
		for _, mp := range topology.Workers.MachinePools {
			allErrs = append(allErrs, mp.Metadata.Validate(
				fldPath.Child("workers", "machinePools").Key(mp.Name).Child("metadata"),
			)...)
		}
	}
	return allErrs
}

func validateTopologyDefinitionFrom(topology *clusterv1.Topology, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for _, variable := range topology.Variables {
		if variable.DefinitionFrom != "" { //nolint:staticcheck // Intentionally using the deprecated field here to check that it is not set.
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("variables").Key(variable.Name),
				string(variable.Value.Raw),
				fmt.Sprintf("variable %q has DefinitionFrom set", variable.Name)),
			)
		}
	}

	if topology.ControlPlane.Variables != nil {
		for _, variable := range topology.ControlPlane.Variables.Overrides {
			if variable.DefinitionFrom != "" { //nolint:staticcheck // Intentionally using the deprecated field here to check that it is not set.
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("controlPlane", "variables", "overrides").Key(variable.Name),
					string(variable.Value.Raw),
					fmt.Sprintf("variable %q has DefinitionFrom set", variable.Name)),
				)
			}
		}
	}

	if topology.Workers != nil {
		for _, md := range topology.Workers.MachineDeployments {
			if md.Variables != nil {
				for _, variable := range md.Variables.Overrides {
					if variable.DefinitionFrom != "" { //nolint:staticcheck // Intentionally using the deprecated field here to check that it is not set.
						allErrs = append(allErrs, field.Invalid(
							fldPath.Child("workers", "machineDeployments").Key(md.Name).Child("variables", "overrides").Key(variable.Name),
							string(variable.Value.Raw),
							fmt.Sprintf("variable %q has DefinitionFrom set", variable.Name)),
						)
					}
				}
			}
		}
		for _, mp := range topology.Workers.MachinePools {
			if mp.Variables != nil {
				for _, variable := range mp.Variables.Overrides {
					if variable.DefinitionFrom != "" { //nolint:staticcheck // Intentionally using the deprecated field here to check that it is not set.
						allErrs = append(allErrs, field.Invalid(
							fldPath.Child("workers", "machinePools").Key(mp.Name).Child("variables", "overrides").Key(variable.Name),
							string(variable.Value.Raw),
							fmt.Sprintf("variable %q has DefinitionFrom set", variable.Name)),
						)
					}
				}
			}
		}
	}

	return allErrs
}

// validateAutoscalerAnnotationsForCluster iterates the MachineDeploymentsTopology objects under Workers and ensures the replicas
// field and min/max annotations for autoscaler are not set at the same time. Optionally it also checks if a given ClusterClass has
// the annotations that may apply to this Cluster.
func validateAutoscalerAnnotationsForCluster(cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	if cluster.Spec.Topology == nil || cluster.Spec.Topology.Workers == nil {
		return allErrs
	}

	fldPath := field.NewPath("spec", "topology")
	for _, mdt := range cluster.Spec.Topology.Workers.MachineDeployments {
		if mdt.Replicas == nil {
			continue
		}
		for k := range mdt.Metadata.Annotations {
			if k == clusterv1.AutoscalerMinSizeAnnotation || k == clusterv1.AutoscalerMaxSizeAnnotation {
				allErrs = append(
					allErrs,
					field.Invalid(
						fldPath.Child("workers", "machineDeployments").Key(mdt.Name).Child("replicas"),
						mdt.Replicas,
						fmt.Sprintf("cannot be set for cluster %q in namespace %q if the same MachineDeploymentTopology has autoscaler annotations",
							cluster.Name, cluster.Namespace),
					),
				)
				break
			}
		}

		// Find a matching MachineDeploymentClass for this MachineDeploymentTopology and make sure it does not have
		// the autoscaler annotations in its Template. Skip this step entirely if clusterClass is nil.
		if clusterClass == nil {
			continue
		}
		for _, mdc := range clusterClass.Spec.Workers.MachineDeployments {
			if mdc.Class != mdt.Class {
				continue
			}
			for k := range mdc.Template.Metadata.Annotations {
				if k == clusterv1.AutoscalerMinSizeAnnotation || k == clusterv1.AutoscalerMaxSizeAnnotation {
					allErrs = append(
						allErrs,
						field.Invalid(
							fldPath.Child("workers", "machineDeployments").Key(mdt.Name).Child("replicas"),
							mdt.Replicas,
							fmt.Sprintf("cannot be set for cluster %q in namespace %q if the source class %q of this MachineDeploymentTopology has autoscaler annotations",
								cluster.Name, cluster.Namespace, mdt.Class),
						),
					)
					break
				}
			}
		}
	}
	return allErrs
}
