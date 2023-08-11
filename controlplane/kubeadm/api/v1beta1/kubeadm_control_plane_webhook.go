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

package v1beta1

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/coredns/corefile-migration/migration"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/util/kubeadm"
	"sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/cluster-api/util/version"
)

func (in *KubeadmControlPlane) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-controlplane-cluster-x-k8s-io-v1beta1-kubeadmcontrolplane,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,versions=v1beta1,name=default.kubeadmcontrolplane.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/validate-controlplane-cluster-x-k8s-io-v1beta1-kubeadmcontrolplane,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,versions=v1beta1,name=validation.kubeadmcontrolplane.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Defaulter = &KubeadmControlPlane{}
var _ webhook.Validator = &KubeadmControlPlane{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (in *KubeadmControlPlane) Default() {
	defaultKubeadmControlPlaneSpec(&in.Spec, in.Namespace)
}

func defaultKubeadmControlPlaneSpec(s *KubeadmControlPlaneSpec, namespace string) {
	if s.Replicas == nil {
		replicas := int32(1)
		s.Replicas = &replicas
	}

	if s.MachineTemplate.InfrastructureRef.Namespace == "" {
		s.MachineTemplate.InfrastructureRef.Namespace = namespace
	}

	if !strings.HasPrefix(s.Version, "v") {
		s.Version = "v" + s.Version
	}

	bootstrapv1.DefaultKubeadmConfigSpec(&s.KubeadmConfigSpec)

	s.RolloutStrategy = defaultRolloutStrategy(s.RolloutStrategy)
}

func defaultRolloutStrategy(rolloutStrategy *RolloutStrategy) *RolloutStrategy {
	ios1 := intstr.FromInt(1)

	if rolloutStrategy == nil {
		rolloutStrategy = &RolloutStrategy{}
	}

	// Enforce RollingUpdate strategy and default MaxSurge if not set.
	if rolloutStrategy != nil {
		if len(rolloutStrategy.Type) == 0 {
			rolloutStrategy.Type = RollingUpdateStrategyType
		}
		if rolloutStrategy.Type == RollingUpdateStrategyType {
			if rolloutStrategy.RollingUpdate == nil {
				rolloutStrategy.RollingUpdate = &RollingUpdate{}
			}
			rolloutStrategy.RollingUpdate.MaxSurge = intstr.ValueOrDefault(rolloutStrategy.RollingUpdate.MaxSurge, ios1)
		}
	}

	return rolloutStrategy
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (in *KubeadmControlPlane) ValidateCreate() (admission.Warnings, error) {
	spec := in.Spec
	allErrs := validateKubeadmControlPlaneSpec(spec, in.Namespace, field.NewPath("spec"))
	allErrs = append(allErrs, validateClusterConfiguration(spec.KubeadmConfigSpec.ClusterConfiguration, nil, field.NewPath("spec", "kubeadmConfigSpec", "clusterConfiguration"))...)
	allErrs = append(allErrs, spec.KubeadmConfigSpec.Validate(field.NewPath("spec", "kubeadmConfigSpec"))...)
	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("KubeadmControlPlane").GroupKind(), in.Name, allErrs)
	}
	return nil, nil
}

const (
	spec                 = "spec"
	kubeadmConfigSpec    = "kubeadmConfigSpec"
	clusterConfiguration = "clusterConfiguration"
	initConfiguration    = "initConfiguration"
	joinConfiguration    = "joinConfiguration"
	nodeRegistration     = "nodeRegistration"
	skipPhases           = "skipPhases"
	patches              = "patches"
	directory            = "directory"
	preKubeadmCommands   = "preKubeadmCommands"
	postKubeadmCommands  = "postKubeadmCommands"
	files                = "files"
	users                = "users"
	apiServer            = "apiServer"
	controllerManager    = "controllerManager"
	scheduler            = "scheduler"
	ntp                  = "ntp"
	ignition             = "ignition"
	diskSetup            = "diskSetup"
)

const minimumCertificatesExpiryDays = 7

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (in *KubeadmControlPlane) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	// add a * to indicate everything beneath is ok.
	// For example, {"spec", "*"} will allow any path under "spec" to change.
	allowedPaths := [][]string{
		{"metadata", "*"},
		{spec, kubeadmConfigSpec, "useExperimentalRetryJoin"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "etcd", "local", "imageRepository"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "etcd", "local", "imageTag"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "etcd", "local", "extraArgs"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "etcd", "local", "extraArgs", "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "dns", "imageRepository"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "dns", "imageTag"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "imageRepository"},
		{spec, kubeadmConfigSpec, clusterConfiguration, apiServer},
		{spec, kubeadmConfigSpec, clusterConfiguration, apiServer, "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, controllerManager},
		{spec, kubeadmConfigSpec, clusterConfiguration, controllerManager, "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, scheduler},
		{spec, kubeadmConfigSpec, clusterConfiguration, scheduler, "*"},
		{spec, kubeadmConfigSpec, initConfiguration, nodeRegistration},
		{spec, kubeadmConfigSpec, initConfiguration, nodeRegistration, "*"},
		{spec, kubeadmConfigSpec, initConfiguration, patches, directory},
		{spec, kubeadmConfigSpec, initConfiguration, skipPhases},
		{spec, kubeadmConfigSpec, joinConfiguration, nodeRegistration},
		{spec, kubeadmConfigSpec, joinConfiguration, nodeRegistration, "*"},
		{spec, kubeadmConfigSpec, joinConfiguration, patches, directory},
		{spec, kubeadmConfigSpec, joinConfiguration, skipPhases},
		{spec, kubeadmConfigSpec, preKubeadmCommands},
		{spec, kubeadmConfigSpec, postKubeadmCommands},
		{spec, kubeadmConfigSpec, files},
		{spec, kubeadmConfigSpec, "verbosity"},
		{spec, kubeadmConfigSpec, users},
		{spec, kubeadmConfigSpec, ntp},
		{spec, kubeadmConfigSpec, ntp, "*"},
		{spec, kubeadmConfigSpec, ignition},
		{spec, kubeadmConfigSpec, ignition, "*"},
		{spec, kubeadmConfigSpec, diskSetup},
		{spec, kubeadmConfigSpec, diskSetup, "*"},
		{spec, kubeadmConfigSpec, "format"},
		{spec, kubeadmConfigSpec, "mounts"},
		{spec, "machineTemplate", "metadata"},
		{spec, "machineTemplate", "metadata", "*"},
		{spec, "machineTemplate", "infrastructureRef", "apiVersion"},
		{spec, "machineTemplate", "infrastructureRef", "name"},
		{spec, "machineTemplate", "infrastructureRef", "kind"},
		{spec, "machineTemplate", "nodeDrainTimeout"},
		{spec, "machineTemplate", "nodeVolumeDetachTimeout"},
		{spec, "machineTemplate", "nodeDeletionTimeout"},
		{spec, "replicas"},
		{spec, "version"},
		{spec, "remediationStrategy"},
		{spec, "remediationStrategy", "*"},
		{spec, "rolloutAfter"},
		{spec, "rolloutBefore"},
		{spec, "rolloutBefore", "*"},
		{spec, "rolloutStrategy"},
		{spec, "rolloutStrategy", "*"},
	}

	allErrs := validateKubeadmControlPlaneSpec(in.Spec, in.Namespace, field.NewPath("spec"))

	prev, ok := old.(*KubeadmControlPlane)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expecting KubeadmControlPlane but got a %T", old))
	}

	originalJSON, err := json.Marshal(prev)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	modifiedJSON, err := json.Marshal(in)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	diff, err := jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	jsonPatch := map[string]interface{}{}
	if err := json.Unmarshal(diff, &jsonPatch); err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	// Build a list of all paths that are trying to change
	diffpaths := paths([]string{}, jsonPatch)
	// Every path in the diff must be valid for the update function to work.
	for _, path := range diffpaths {
		// Ignore paths that are empty
		if len(path) == 0 {
			continue
		}
		if !allowed(allowedPaths, path) {
			if len(path) == 1 {
				allErrs = append(allErrs, field.Forbidden(field.NewPath(path[0]), "cannot be modified"))
				continue
			}
			allErrs = append(allErrs, field.Forbidden(field.NewPath(path[0], path[1:]...), "cannot be modified"))
		}
	}

	allErrs = append(allErrs, in.validateVersion(prev.Spec.Version)...)
	allErrs = append(allErrs, validateClusterConfiguration(in.Spec.KubeadmConfigSpec.ClusterConfiguration, prev.Spec.KubeadmConfigSpec.ClusterConfiguration, field.NewPath("spec", "kubeadmConfigSpec", "clusterConfiguration"))...)
	allErrs = append(allErrs, in.validateCoreDNSVersion(prev)...)
	allErrs = append(allErrs, in.Spec.KubeadmConfigSpec.Validate(field.NewPath("spec", "kubeadmConfigSpec"))...)

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("KubeadmControlPlane").GroupKind(), in.Name, allErrs)
	}

	return nil, nil
}

func validateKubeadmControlPlaneSpec(s KubeadmControlPlaneSpec, namespace string, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if s.Replicas == nil {
		allErrs = append(
			allErrs,
			field.Required(
				pathPrefix.Child("replicas"),
				"is required",
			),
		)
	} else if *s.Replicas <= 0 {
		// The use of the scale subresource should provide a guarantee that negative values
		// should not be accepted for this field, but since we have to validate that Replicas != 0
		// it doesn't hurt to also additionally validate for negative numbers here as well.
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("replicas"),
				"cannot be less than or equal to 0",
			),
		)
	}

	externalEtcd := false
	if s.KubeadmConfigSpec.ClusterConfiguration != nil {
		if s.KubeadmConfigSpec.ClusterConfiguration.Etcd.External != nil {
			externalEtcd = true
		}
	}

	if !externalEtcd {
		if s.Replicas != nil && *s.Replicas%2 == 0 {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("replicas"),
					"cannot be an even number when etcd is stacked",
				),
			)
		}
	}

	if s.MachineTemplate.InfrastructureRef.APIVersion == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "apiVersion"),
				s.MachineTemplate.InfrastructureRef.APIVersion,
				"cannot be empty",
			),
		)
	}
	if s.MachineTemplate.InfrastructureRef.Kind == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "kind"),
				s.MachineTemplate.InfrastructureRef.Kind,
				"cannot be empty",
			),
		)
	}
	if s.MachineTemplate.InfrastructureRef.Name == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "name"),
				s.MachineTemplate.InfrastructureRef.Name,
				"cannot be empty",
			),
		)
	}
	if s.MachineTemplate.InfrastructureRef.Namespace != namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "namespace"),
				s.MachineTemplate.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if !version.KubeSemver.MatchString(s.Version) {
		allErrs = append(allErrs, field.Invalid(pathPrefix.Child("version"), s.Version, "must be a valid semantic version"))
	}

	allErrs = append(allErrs, validateRolloutBefore(s.RolloutBefore, pathPrefix.Child("rolloutBefore"))...)
	allErrs = append(allErrs, validateRolloutStrategy(s.RolloutStrategy, s.Replicas, pathPrefix.Child("rolloutStrategy"))...)

	return allErrs
}

func validateRolloutBefore(rolloutBefore *RolloutBefore, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if rolloutBefore == nil {
		return allErrs
	}

	if rolloutBefore.CertificatesExpiryDays != nil {
		if *rolloutBefore.CertificatesExpiryDays < minimumCertificatesExpiryDays {
			allErrs = append(allErrs, field.Invalid(pathPrefix.Child("certificatesExpiryDays"), *rolloutBefore.CertificatesExpiryDays, fmt.Sprintf("must be greater than or equal to %v", minimumCertificatesExpiryDays)))
		}
	}

	return allErrs
}

func validateRolloutStrategy(rolloutStrategy *RolloutStrategy, replicas *int32, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if rolloutStrategy == nil {
		return allErrs
	}

	if rolloutStrategy.Type != RollingUpdateStrategyType {
		allErrs = append(
			allErrs,
			field.Required(
				pathPrefix.Child("type"),
				"only RollingUpdateStrategyType is supported",
			),
		)
	}

	ios1 := intstr.FromInt(1)
	ios0 := intstr.FromInt(0)

	if rolloutStrategy.RollingUpdate.MaxSurge.IntValue() == ios0.IntValue() && (replicas != nil && *replicas < int32(3)) {
		allErrs = append(
			allErrs,
			field.Required(
				pathPrefix.Child("rollingUpdate"),
				"when KubeadmControlPlane is configured to scale-in, replica count needs to be at least 3",
			),
		)
	}

	if rolloutStrategy.RollingUpdate.MaxSurge.IntValue() != ios1.IntValue() && rolloutStrategy.RollingUpdate.MaxSurge.IntValue() != ios0.IntValue() {
		allErrs = append(
			allErrs,
			field.Required(
				pathPrefix.Child("rollingUpdate", "maxSurge"),
				"value must be 1 or 0",
			),
		)
	}

	return allErrs
}

func validateClusterConfiguration(newClusterConfiguration, oldClusterConfiguration *bootstrapv1.ClusterConfiguration, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if newClusterConfiguration == nil {
		return allErrs
	}

	// TODO: Remove when kubeadm types include OpenAPI validation
	if !container.ImageTagIsValid(newClusterConfiguration.DNS.ImageTag) {
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("dns", "imageTag"),
				fmt.Sprintf("tag %s is invalid", newClusterConfiguration.DNS.ImageTag),
			),
		)
	}

	if newClusterConfiguration.DNS.ImageTag != "" {
		if _, err := version.ParseMajorMinorPatchTolerant(newClusterConfiguration.DNS.ImageTag); err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("dns", "imageTag"),
					newClusterConfiguration.DNS.ImageTag,
					fmt.Sprintf("failed to parse CoreDNS version: %v", err),
				),
			)
		}
	}

	// TODO: Remove when kubeadm types include OpenAPI validation
	if newClusterConfiguration.Etcd.Local != nil && !container.ImageTagIsValid(newClusterConfiguration.Etcd.Local.ImageTag) {
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("etcd", "local", "imageTag"),
				fmt.Sprintf("tag %s is invalid", newClusterConfiguration.Etcd.Local.ImageTag),
			),
		)
	}

	if newClusterConfiguration.Etcd.Local != nil && newClusterConfiguration.Etcd.External != nil {
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("etcd", "local"),
				"cannot have both external and local etcd",
			),
		)
	}

	// update validations
	if oldClusterConfiguration != nil {
		if newClusterConfiguration.Etcd.External != nil && oldClusterConfiguration.Etcd.Local != nil {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("etcd", "external"),
					"cannot change between external and local etcd",
				),
			)
		}

		if newClusterConfiguration.Etcd.Local != nil && oldClusterConfiguration.Etcd.External != nil {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("etcd", "local"),
					"cannot change between external and local etcd",
				),
			)
		}
	}

	return allErrs
}

func allowed(allowList [][]string, path []string) bool {
	for _, allowed := range allowList {
		if pathsMatch(allowed, path) {
			return true
		}
	}
	return false
}

func pathsMatch(allowed, path []string) bool {
	// if either are empty then no match can be made
	if len(allowed) == 0 || len(path) == 0 {
		return false
	}
	i := 0
	for i = range path {
		// reached the end of the allowed path and no match was found
		if i > len(allowed)-1 {
			return false
		}
		if allowed[i] == "*" {
			return true
		}
		if path[i] != allowed[i] {
			return false
		}
	}
	// path has been completely iterated and has not matched the end of the path.
	// e.g. allowed: []string{"a","b","c"}, path: []string{"a"}
	return i >= len(allowed)-1
}

// paths builds a slice of paths that are being modified.
func paths(path []string, diff map[string]interface{}) [][]string {
	allPaths := [][]string{}
	for key, m := range diff {
		nested, ok := m.(map[string]interface{})
		if !ok {
			// We have to use a copy of path, because otherwise the slice we append to
			// allPaths would be overwritten in another iteration.
			tmp := make([]string, len(path))
			copy(tmp, path)
			allPaths = append(allPaths, append(tmp, key))
			continue
		}
		allPaths = append(allPaths, paths(append(path, key), nested)...)
	}
	return allPaths
}

func (in *KubeadmControlPlane) validateCoreDNSVersion(prev *KubeadmControlPlane) (allErrs field.ErrorList) {
	if in.Spec.KubeadmConfigSpec.ClusterConfiguration == nil || prev.Spec.KubeadmConfigSpec.ClusterConfiguration == nil {
		return allErrs
	}
	// return if either current or target versions is empty
	if prev.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag == "" || in.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag == "" {
		return allErrs
	}
	targetDNS := &in.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS

	fromVersion, err := version.ParseMajorMinorPatchTolerant(prev.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "kubeadmConfigSpec", "clusterConfiguration", "dns", "imageTag"),
				prev.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag,
				fmt.Sprintf("failed to parse current CoreDNS version: %v", err),
			),
		)
		return allErrs
	}

	toVersion, err := version.ParseMajorMinorPatchTolerant(targetDNS.ImageTag)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "kubeadmConfigSpec", "clusterConfiguration", "dns", "imageTag"),
				targetDNS.ImageTag,
				fmt.Sprintf("failed to parse target CoreDNS version: %v", err),
			),
		)
		return allErrs
	}
	// If the versions are equal return here without error.
	// This allows an upgrade where the version of CoreDNS in use is not supported by the migration tool.
	if toVersion.Equals(fromVersion) {
		return allErrs
	}
	if err := migration.ValidUpMigration(fromVersion.String(), toVersion.String()); err != nil {
		allErrs = append(
			allErrs,
			field.Forbidden(
				field.NewPath("spec", "kubeadmConfigSpec", "clusterConfiguration", "dns", "imageTag"),
				fmt.Sprintf("cannot migrate CoreDNS up to '%v' from '%v': %v", toVersion, fromVersion, err),
			),
		)
	}

	return allErrs
}

func (in *KubeadmControlPlane) validateVersion(previousVersion string) (allErrs field.ErrorList) {
	fromVersion, err := version.ParseMajorMinorPatch(previousVersion)
	if err != nil {
		allErrs = append(allErrs,
			field.InternalError(
				field.NewPath("spec", "version"),
				errors.Wrapf(err, "failed to parse current kubeadmcontrolplane version: %s", previousVersion),
			),
		)
		return allErrs
	}

	toVersion, err := version.ParseMajorMinorPatch(in.Spec.Version)
	if err != nil {
		allErrs = append(allErrs,
			field.InternalError(
				field.NewPath("spec", "version"),
				errors.Wrapf(err, "failed to parse updated kubeadmcontrolplane version: %s", in.Spec.Version),
			),
		)
		return allErrs
	}

	// Check if we're trying to upgrade to Kubernetes v1.19.0, which is not supported.
	//
	// See https://github.com/kubernetes-sigs/cluster-api/issues/3564
	if fromVersion.NE(toVersion) && toVersion.Equals(semver.MustParse("1.19.0")) {
		allErrs = append(allErrs,
			field.Forbidden(
				field.NewPath("spec", "version"),
				"cannot update Kubernetes version to v1.19.0, for more information see https://github.com/kubernetes-sigs/cluster-api/issues/3564",
			),
		)
		return allErrs
	}

	// Validate that the update is upgrading at most one minor version.
	// Note: Skipping a minor version is not allowed.
	// Note: Checking against this ceilVersion allows upgrading to the next minor
	// version irrespective of the patch version.
	ceilVersion := semver.Version{
		Major: fromVersion.Major,
		Minor: fromVersion.Minor + 2,
		Patch: 0,
	}
	if toVersion.GTE(ceilVersion) {
		allErrs = append(allErrs,
			field.Forbidden(
				field.NewPath("spec", "version"),
				fmt.Sprintf("cannot update Kubernetes version from %s to %s", previousVersion, in.Spec.Version),
			),
		)
	}

	// The Kubernetes ecosystem has been requested to move users to the new registry due to cost issues.
	// This validation enforces the move to the new registry by forcing users to upgrade to kubeadm versions
	// with the new registry.
	// NOTE: This only affects users relying on the community maintained registry.
	// NOTE: Pinning to the upstream registry is not recommended because it could lead to issues
	// given how the migration has been implemented in kubeadm.
	//
	// Block if imageRepository is not set (i.e. the default registry should be used),
	if (in.Spec.KubeadmConfigSpec.ClusterConfiguration == nil ||
		in.Spec.KubeadmConfigSpec.ClusterConfiguration.ImageRepository == "") &&
		// the version changed (i.e. we have an upgrade),
		toVersion.NE(fromVersion) &&
		// the version is >= v1.22.0 and < v1.26.0
		toVersion.GTE(kubeadm.MinKubernetesVersionImageRegistryMigration) &&
		toVersion.LT(kubeadm.NextKubernetesVersionImageRegistryMigration) &&
		// and the default registry of the new Kubernetes/kubeadm version is the old default registry.
		kubeadm.GetDefaultRegistry(toVersion) == kubeadm.OldDefaultImageRepository {
		allErrs = append(allErrs,
			field.Forbidden(
				field.NewPath("spec", "version"),
				"cannot upgrade to a Kubernetes/kubeadm version which is using the old default registry. Please use a newer Kubernetes patch release which is using the new default registry (>= v1.22.17, >= v1.23.15, >= v1.24.9)",
			),
		)
	}

	return allErrs
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (in *KubeadmControlPlane) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}
