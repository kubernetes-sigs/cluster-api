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
	"reflect"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/coredns/corefile-migration/migration"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	topologynames "sigs.k8s.io/cluster-api/internal/topology/names"
	"sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/cluster-api/util/version"
)

const defaultCACertificatesExpiryDays = 3650

func (webhook *KubeadmControlPlane) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&controlplanev1.KubeadmControlPlane{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-controlplane-cluster-x-k8s-io-v1beta2-kubeadmcontrolplane,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,versions=v1beta2,name=default.kubeadmcontrolplane.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/validate-controlplane-cluster-x-k8s-io-v1beta2-kubeadmcontrolplane,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,versions=v1beta2,name=validation.kubeadmcontrolplane.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// KubeadmControlPlane implements a validation and defaulting webhook for KubeadmControlPlane.
type KubeadmControlPlane struct{}

var _ webhook.CustomValidator = &KubeadmControlPlane{}
var _ webhook.CustomDefaulter = &KubeadmControlPlane{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *KubeadmControlPlane) Default(_ context.Context, obj runtime.Object) error {
	k, ok := obj.(*controlplanev1.KubeadmControlPlane)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlane but got a %T", obj))
	}

	defaultKubeadmControlPlaneSpec(&k.Spec)

	return nil
}

func defaultKubeadmControlPlaneSpec(s *controlplanev1.KubeadmControlPlaneSpec) {
	if s.Replicas == nil {
		replicas := int32(1)
		s.Replicas = &replicas
	}

	if !strings.HasPrefix(s.Version, "v") {
		s.Version = "v" + s.Version
	}

	// Enforce RollingUpdate strategy and default MaxSurge if not set.
	s.Rollout.Strategy.Type = controlplanev1.RollingUpdateStrategyType
	s.Rollout.Strategy.RollingUpdate.MaxSurge = intstr.ValueOrDefault(s.Rollout.Strategy.RollingUpdate.MaxSurge, intstr.FromInt32(1))
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmControlPlane) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	k, ok := obj.(*controlplanev1.KubeadmControlPlane)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlane but got a %T", obj))
	}

	spec := k.Spec
	allErrs := validateKubeadmControlPlaneSpec(spec, field.NewPath("spec"))
	allErrs = append(allErrs, validateClusterConfiguration(nil, spec.KubeadmConfigSpec.ClusterConfiguration, field.NewPath("spec", "kubeadmConfigSpec", "clusterConfiguration"))...)
	allErrs = append(allErrs, spec.KubeadmConfigSpec.Validate(field.NewPath("spec", "kubeadmConfigSpec"))...)
	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("KubeadmControlPlane").GroupKind(), k.Name, allErrs)
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
	bootCommands         = "bootCommands"
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
	featureGates         = "featureGates"
	timeouts             = "timeouts"
)

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmControlPlane) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	// add a * to indicate everything beneath is ok.
	// For example, {"spec", "*"} will allow any path under "spec" to change.
	// For example, {"spec"} will allow "spec" to also be unset.
	allowedPaths := [][]string{
		// metadata
		{"metadata", "*"},
		// spec.kubeadmConfigSpec.clusterConfiguration
		{spec, kubeadmConfigSpec, clusterConfiguration, "etcd"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "etcd", "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "etcd", "local"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "etcd", "local", "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "etcd", "external", "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "dns"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "dns", "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "imageRepository"},
		{spec, kubeadmConfigSpec, clusterConfiguration, featureGates},
		{spec, kubeadmConfigSpec, clusterConfiguration, featureGates, "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, apiServer},
		{spec, kubeadmConfigSpec, clusterConfiguration, apiServer, "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, controllerManager},
		{spec, kubeadmConfigSpec, clusterConfiguration, controllerManager, "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, scheduler},
		{spec, kubeadmConfigSpec, clusterConfiguration, scheduler, "*"},
		{spec, kubeadmConfigSpec, clusterConfiguration, "certificateValidityPeriodDays"},
		// spec.kubeadmConfigSpec.initConfiguration
		{spec, kubeadmConfigSpec, initConfiguration, nodeRegistration},
		{spec, kubeadmConfigSpec, initConfiguration, nodeRegistration, "*"},
		{spec, kubeadmConfigSpec, initConfiguration, patches, directory},
		{spec, kubeadmConfigSpec, initConfiguration, patches},
		{spec, kubeadmConfigSpec, initConfiguration, skipPhases},
		{spec, kubeadmConfigSpec, initConfiguration, "bootstrapTokens"},
		{spec, kubeadmConfigSpec, initConfiguration, "localAPIEndpoint"},
		{spec, kubeadmConfigSpec, initConfiguration, "localAPIEndpoint", "*"},
		{spec, kubeadmConfigSpec, initConfiguration, timeouts},
		{spec, kubeadmConfigSpec, initConfiguration, timeouts, "*"},
		// spec.kubeadmConfigSpec.joinConfiguration
		{spec, kubeadmConfigSpec, joinConfiguration, nodeRegistration},
		{spec, kubeadmConfigSpec, joinConfiguration, nodeRegistration, "*"},
		{spec, kubeadmConfigSpec, joinConfiguration, patches, directory},
		{spec, kubeadmConfigSpec, joinConfiguration, patches},
		{spec, kubeadmConfigSpec, joinConfiguration, skipPhases},
		{spec, kubeadmConfigSpec, joinConfiguration, "caCertPath"},
		{spec, kubeadmConfigSpec, joinConfiguration, "controlPlane"},
		{spec, kubeadmConfigSpec, joinConfiguration, "controlPlane", "*"},
		{spec, kubeadmConfigSpec, joinConfiguration, "discovery"},
		{spec, kubeadmConfigSpec, joinConfiguration, "discovery", "*"},
		{spec, kubeadmConfigSpec, joinConfiguration, timeouts},
		{spec, kubeadmConfigSpec, joinConfiguration, timeouts, "*"},
		// spec.kubeadmConfigSpec
		{spec, kubeadmConfigSpec, bootCommands},
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
		// spec.machineTemplate
		{spec, "machineTemplate", "metadata"},
		{spec, "machineTemplate", "metadata", "*"},
		{spec, "machineTemplate", "spec"},
		{spec, "machineTemplate", "spec", "*"},
		// spec
		{spec, "replicas"},
		{spec, "version"},
		{spec, "remediation"},
		{spec, "remediation", "*"},
		{spec, "machineNaming"},
		{spec, "machineNaming", "*"},
		{spec, "rollout"},
		{spec, "rollout", "*"},
	}

	oldK, ok := oldObj.(*controlplanev1.KubeadmControlPlane)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlane but got a %T", oldObj))
	}

	newK, ok := newObj.(*controlplanev1.KubeadmControlPlane)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlane but got a %T", newObj))
	}

	allErrs := validateKubeadmControlPlaneSpec(newK.Spec, field.NewPath("spec"))

	originalJSON, err := json.Marshal(oldK)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	modifiedJSON, err := json.Marshal(newK)
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

	allErrs = append(allErrs, webhook.validateVersion(oldK, newK)...)
	allErrs = append(allErrs, validateClusterConfiguration(oldK.Spec.KubeadmConfigSpec.ClusterConfiguration, newK.Spec.KubeadmConfigSpec.ClusterConfiguration, field.NewPath("spec", "kubeadmConfigSpec", "clusterConfiguration"))...)
	allErrs = append(allErrs, webhook.validateCoreDNSVersion(oldK, newK)...)
	allErrs = append(allErrs, newK.Spec.KubeadmConfigSpec.Validate(field.NewPath("spec", "kubeadmConfigSpec"))...)

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("KubeadmControlPlane").GroupKind(), newK.Name, allErrs)
	}

	return nil, nil
}

func validateKubeadmControlPlaneSpec(s controlplanev1.KubeadmControlPlaneSpec, pathPrefix *field.Path) field.ErrorList {
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
		path := field.NewPath("spec", "kubeadmConfigSpec", "clusterConfiguration")
		if s.KubeadmConfigSpec.ClusterConfiguration.CertificateValidityPeriodDays > s.KubeadmConfigSpec.ClusterConfiguration.CACertificateValidityPeriodDays {
			allErrs = append(
				allErrs,
				field.Invalid(
					path.Child("certificateValidityPeriodDays"),
					s.KubeadmConfigSpec.ClusterConfiguration.CertificateValidityPeriodDays,
					fmt.Sprintf("must be less than or equal to caCertificateValidityPeriodDays %v", s.KubeadmConfigSpec.ClusterConfiguration.CACertificateValidityPeriodDays),
				),
			)
		}
		if s.KubeadmConfigSpec.ClusterConfiguration.CACertificateValidityPeriodDays == 0 && s.KubeadmConfigSpec.ClusterConfiguration.CertificateValidityPeriodDays > defaultCACertificatesExpiryDays {
			allErrs = append(
				allErrs,
				field.Invalid(
					path.Child("certificateValidityPeriodDays"),
					s.KubeadmConfigSpec.ClusterConfiguration.CertificateValidityPeriodDays,
					fmt.Sprintf("must be less than or equal to default value of caCertificateValidityPeriodDays %v", defaultCACertificatesExpiryDays),
				),
			)
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

	if s.MachineTemplate.Spec.InfrastructureRef.APIGroup == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "apiGroup"),
				s.MachineTemplate.Spec.InfrastructureRef.APIGroup,
				"cannot be empty",
			),
		)
	}
	if s.MachineTemplate.Spec.InfrastructureRef.Kind == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "kind"),
				s.MachineTemplate.Spec.InfrastructureRef.Kind,
				"cannot be empty",
			),
		)
	}
	if s.MachineTemplate.Spec.InfrastructureRef.Name == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "name"),
				s.MachineTemplate.Spec.InfrastructureRef.Name,
				"cannot be empty",
			),
		)
	}

	// Validate the metadata of the MachineTemplate
	allErrs = append(allErrs, s.MachineTemplate.ObjectMeta.Validate(pathPrefix.Child("machineTemplate", "metadata"))...)

	if !version.KubeSemver.MatchString(s.Version) {
		allErrs = append(allErrs, field.Invalid(pathPrefix.Child("version"), s.Version, "must be a valid semantic version"))
	}

	allErrs = append(allErrs, validateRolloutStrategy(s.KubeadmConfigSpec.ClusterConfiguration, s.Rollout, s.Replicas, pathPrefix.Child("rollout"))...)
	allErrs = append(allErrs, validateNaming(s.MachineNaming, pathPrefix.Child("machineNaming"))...)
	return allErrs
}

func validateRolloutStrategy(clusterConfiguration *bootstrapv1.ClusterConfiguration, rolloutSpec controlplanev1.KubeadmControlPlaneRolloutSpec, replicas *int32, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	rolloutStrategy := rolloutSpec.Strategy
	strategyPathPrefix := pathPrefix.Child("strategy")

	if reflect.DeepEqual(rolloutStrategy, controlplanev1.KubeadmControlPlaneRolloutStrategy{}) {
		return nil
	}

	if rolloutStrategy.Type != controlplanev1.RollingUpdateStrategyType {
		allErrs = append(
			allErrs,
			field.Required(
				strategyPathPrefix.Child("type"),
				"only RollingUpdate is supported",
			),
		)
	}

	if rolloutStrategy.RollingUpdate.MaxSurge != nil {
		ios1 := intstr.FromInt32(1)
		ios0 := intstr.FromInt32(0)
		if rolloutStrategy.RollingUpdate.MaxSurge.IntValue() == ios0.IntValue() && (replicas != nil && *replicas < int32(3)) {
			allErrs = append(
				allErrs,
				field.Required(
					strategyPathPrefix.Child("rollingUpdate"),
					"when KubeadmControlPlane is configured to scale-in, replica count needs to be at least 3",
				),
			)
		}
		if rolloutStrategy.RollingUpdate.MaxSurge.IntValue() != ios1.IntValue() && rolloutStrategy.RollingUpdate.MaxSurge.IntValue() != ios0.IntValue() {
			allErrs = append(
				allErrs,
				field.Required(
					strategyPathPrefix.Child("rollingUpdate", "maxSurge"),
					"value must be 1 or 0",
				),
			)
		}
	}

	if clusterConfiguration == nil {
		return allErrs
	}

	if clusterConfiguration.CertificateValidityPeriodDays != 0 && rolloutSpec.Before.CertificatesExpiryDays != 0 {
		if rolloutSpec.Before.CertificatesExpiryDays < clusterConfiguration.CertificateValidityPeriodDays {
			allErrs = append(allErrs, field.Invalid(pathPrefix.Child("before", "certificatesExpiryDays"), rolloutSpec.Before.CertificatesExpiryDays, fmt.Sprintf("must be greater than or equal to certificateValidityPeriodDays %v", clusterConfiguration.CertificateValidityPeriodDays)))
		}
	}
	return allErrs
}

func validateNaming(machineNaming controlplanev1.MachineNamingSpec, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if machineNaming.Template != "" {
		if !strings.Contains(machineNaming.Template, "{{ .random }}") {
			allErrs = append(allErrs,
				field.Invalid(
					pathPrefix.Child("template"),
					machineNaming.Template,
					"invalid template, {{ .random }} is missing",
				))
		}
		name, err := topologynames.KCPMachineNameGenerator(machineNaming.Template, "cluster", "kubeadmcontrolplane").GenerateName()
		if err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					pathPrefix.Child("template"),
					machineNaming.Template,
					fmt.Sprintf("invalid template: %v", err),
				))
		} else {
			for _, err := range validation.IsDNS1123Subdomain(name) {
				allErrs = append(allErrs,
					field.Invalid(
						pathPrefix.Child("template"),
						machineNaming.Template,
						fmt.Sprintf("invalid template, generated names would not be valid Kubernetes object names: %v", err),
					))
			}
		}
	}

	return allErrs
}

func validateClusterConfiguration(oldClusterConfiguration, newClusterConfiguration *bootstrapv1.ClusterConfiguration, pathPrefix *field.Path) field.ErrorList {
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
		if _, err := version.ParseTolerantImageTag(newClusterConfiguration.DNS.ImageTag); err != nil {
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
		if (newClusterConfiguration.Etcd.External != nil && oldClusterConfiguration.Etcd.External == nil) || (newClusterConfiguration.Etcd.External == nil && oldClusterConfiguration.Etcd.External != nil) {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("etcd", "external"),
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

func (webhook *KubeadmControlPlane) validateCoreDNSVersion(oldK, newK *controlplanev1.KubeadmControlPlane) (allErrs field.ErrorList) {
	if newK.Spec.KubeadmConfigSpec.ClusterConfiguration == nil || oldK.Spec.KubeadmConfigSpec.ClusterConfiguration == nil {
		return allErrs
	}
	// return if either current or target versions is empty
	if newK.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag == "" || oldK.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag == "" {
		return allErrs
	}
	targetDNS := &newK.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS

	fromVersion, err := version.ParseTolerantImageTag(oldK.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "kubeadmConfigSpec", "clusterConfiguration", "dns", "imageTag"),
				oldK.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag,
				fmt.Sprintf("failed to parse current CoreDNS version: %v", err),
			),
		)
		return allErrs
	}

	toVersion, err := version.ParseTolerantImageTag(targetDNS.ImageTag)
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
	if version.Compare(toVersion, fromVersion, version.WithoutPreReleases()) == 0 {
		return allErrs
	}

	// Skip validating if the skip CoreDNS annotation is set. If set, KCP doesn't use the migration library.
	if _, ok := newK.Annotations[controlplanev1.SkipCoreDNSAnnotation]; ok {
		return allErrs
	}

	if err := migration.ValidUpMigration(version.MajorMinorPatch(fromVersion).String(), version.MajorMinorPatch(toVersion).String()); err != nil {
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

func (webhook *KubeadmControlPlane) validateVersion(oldK, newK *controlplanev1.KubeadmControlPlane) (allErrs field.ErrorList) {
	previousVersion := oldK.Spec.Version
	fromVersion, err := semver.ParseTolerant(previousVersion)
	if err != nil {
		allErrs = append(allErrs,
			field.InternalError(
				field.NewPath("spec", "version"),
				errors.Wrapf(err, "failed to parse current kubeadmcontrolplane version: %s", previousVersion),
			),
		)
		return allErrs
	}

	toVersion, err := semver.ParseTolerant(newK.Spec.Version)
	if err != nil {
		allErrs = append(allErrs,
			field.InternalError(
				field.NewPath("spec", "version"),
				errors.Wrapf(err, "failed to parse updated kubeadmcontrolplane version: %s", newK.Spec.Version),
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
	if version.Compare(toVersion, ceilVersion, version.WithoutPreReleases()) >= 0 {
		allErrs = append(allErrs,
			field.Forbidden(
				field.NewPath("spec", "version"),
				fmt.Sprintf("cannot update Kubernetes version from %s to %s", previousVersion, newK.Spec.Version),
			),
		)
	}

	return allErrs
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmControlPlane) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
