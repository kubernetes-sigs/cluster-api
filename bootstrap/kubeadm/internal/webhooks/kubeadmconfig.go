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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
)

var (
	cannotUseWithIgnition                            = fmt.Sprintf("not supported when spec.format is set to %q", bootstrapv1.Ignition)
	conflictingFileSourceMsg                         = "only one of content or contentFrom may be specified for a single file"
	conflictingUserSourceMsg                         = "only one of passwd or passwdFrom may be specified for a single user"
	kubeadmBootstrapFormatIgnitionFeatureDisabledMsg = "can be set only if the KubeadmBootstrapFormatIgnition feature gate is enabled"
	missingSecretNameMsg                             = "secret file source must specify non-empty secret name"
	missingSecretKeyMsg                              = "secret file source must specify non-empty secret key"
	pathConflictMsg                                  = "path property must be unique among all files"
)

func (webhook *KubeadmConfig) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&bootstrapv1.KubeadmConfig{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfig,mutating=true,failurePolicy=fail,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs,versions=v1beta1,name=default.kubeadmconfig.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/validate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfig,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs,versions=v1beta1,name=validation.kubeadmconfig.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// KubeadmConfig implements a validating and defaulting webhook for KubeadmConfig.
type KubeadmConfig struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &KubeadmConfig{}
var _ webhook.CustomValidator = &KubeadmConfig{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *KubeadmConfig) Default(_ context.Context, obj runtime.Object) error {
	config, ok := obj.(*bootstrapv1.KubeadmConfig)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfig but got a %T", obj))
	}
	bootstrapv1.DefaultKubeadmConfigSpec(&config.Spec)

	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *KubeadmConfig) ValidateCreate(_ context.Context, obj runtime.Object) error {
	config, ok := obj.(*bootstrapv1.KubeadmConfig)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfig but got a %T", obj))
	}
	return webhook.validate(config)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *KubeadmConfig) ValidateUpdate(_ context.Context, _, newObj runtime.Object) error {
	config, ok := newObj.(*bootstrapv1.KubeadmConfig)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfig but got a %T", newObj))
	}
	return webhook.validate(config)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *KubeadmConfig) ValidateDelete(_ context.Context, _ runtime.Object) error {
	return nil
}

func (webhook *KubeadmConfig) validate(config *bootstrapv1.KubeadmConfig) error {
	allErrs := ValidateKubeadmConfigSpec(&config.Spec, field.NewPath("spec"))

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(bootstrapv1.GroupVersion.WithKind("KubeadmConfig").GroupKind(), config.Name, allErrs)
}

// ValidateKubeadmConfigSpec ensures the KubeadmConfigSpec is valid.
func ValidateKubeadmConfigSpec(spec *bootstrapv1.KubeadmConfigSpec, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validateFiles(spec, pathPrefix)...)
	allErrs = append(allErrs, validateUsers(spec, pathPrefix)...)
	allErrs = append(allErrs, validateIgnition(spec, pathPrefix)...)

	return allErrs
}

func validateFiles(spec *bootstrapv1.KubeadmConfigSpec, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	knownPaths := map[string]struct{}{}

	for i := range spec.Files {
		file := spec.Files[i]
		if file.Content != "" && file.ContentFrom != nil {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("files").Index(i),
					file,
					conflictingFileSourceMsg,
				),
			)
		}
		// n.b.: if we ever add types besides Secret as a ContentFrom
		// Source, we must add webhook validation here for one of the
		// sources being non-nil.
		if file.ContentFrom != nil {
			if file.ContentFrom.Secret.Name == "" {
				allErrs = append(
					allErrs,
					field.Required(
						pathPrefix.Child("files").Index(i).Child("contentFrom", "secret", "name"),
						missingSecretNameMsg,
					),
				)
			}
			if file.ContentFrom.Secret.Key == "" {
				allErrs = append(
					allErrs,
					field.Required(
						pathPrefix.Child("files").Index(i).Child("contentFrom", "secret", "key"),
						missingSecretKeyMsg,
					),
				)
			}
		}
		_, conflict := knownPaths[file.Path]
		if conflict {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("files").Index(i).Child("path"),
					file,
					pathConflictMsg,
				),
			)
		}
		knownPaths[file.Path] = struct{}{}
	}

	return allErrs
}

func validateUsers(spec *bootstrapv1.KubeadmConfigSpec, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for i := range spec.Users {
		user := spec.Users[i]
		if user.Passwd != nil && user.PasswdFrom != nil {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("users").Index(i),
					user,
					conflictingUserSourceMsg,
				),
			)
		}
		// n.b.: if we ever add types besides Secret as a PasswdFrom
		// Source, we must add webhook validation here for one of the
		// sources being non-nil.
		if user.PasswdFrom != nil {
			if user.PasswdFrom.Secret.Name == "" {
				allErrs = append(
					allErrs,
					field.Required(
						pathPrefix.Child("users").Index(i).Child("passwdFrom", "secret", "name"),
						missingSecretNameMsg,
					),
				)
			}
			if user.PasswdFrom.Secret.Key == "" {
				allErrs = append(
					allErrs,
					field.Required(
						pathPrefix.Child("users").Index(i).Child("passwdFrom", "secret", "key"),
						missingSecretKeyMsg,
					),
				)
			}
		}
	}

	return allErrs
}

func validateIgnition(spec *bootstrapv1.KubeadmConfigSpec, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !feature.Gates.Enabled(feature.KubeadmBootstrapFormatIgnition) {
		if spec.Format == bootstrapv1.Ignition {
			allErrs = append(allErrs, field.Forbidden(
				pathPrefix.Child("format"), kubeadmBootstrapFormatIgnitionFeatureDisabledMsg))
		}

		if spec.Ignition != nil {
			allErrs = append(allErrs, field.Forbidden(
				pathPrefix.Child("ignition"), kubeadmBootstrapFormatIgnitionFeatureDisabledMsg))
		}

		return allErrs
	}

	if spec.Format != bootstrapv1.Ignition {
		if spec.Ignition != nil {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("format"),
					spec.Format,
					fmt.Sprintf("must be set to %q if spec.ignition is set", bootstrapv1.Ignition),
				),
			)
		}

		return allErrs
	}

	for i, user := range spec.Users {
		if user.Inactive != nil && *user.Inactive {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("users").Index(i).Child("inactive"),
					cannotUseWithIgnition,
				),
			)
		}
	}

	if spec.UseExperimentalRetryJoin {
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("useExperimentalRetryJoin"),
				cannotUseWithIgnition,
			),
		)
	}

	for i, file := range spec.Files {
		if file.Encoding == bootstrapv1.Gzip || file.Encoding == bootstrapv1.GzipBase64 {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("files").Index(i).Child("encoding"),
					cannotUseWithIgnition,
				),
			)
		}
	}

	if spec.DiskSetup == nil {
		return allErrs
	}

	for i, partition := range spec.DiskSetup.Partitions {
		if partition.TableType != nil && *partition.TableType != "gpt" {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("diskSetup", "partitions").Index(i).Child("tableType"),
					*partition.TableType,
					fmt.Sprintf(
						"only partition type %q is supported when spec.format is set to %q",
						"gpt",
						bootstrapv1.Ignition,
					),
				),
			)
		}
	}

	for i, fs := range spec.DiskSetup.Filesystems {
		if fs.ReplaceFS != nil {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("diskSetup", "filesystems").Index(i).Child("replaceFS"),
					cannotUseWithIgnition,
				),
			)
		}

		if fs.Partition != nil {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("diskSetup", "filesystems").Index(i).Child("partition"),
					cannotUseWithIgnition,
				),
			)
		}
	}

	return allErrs
}
