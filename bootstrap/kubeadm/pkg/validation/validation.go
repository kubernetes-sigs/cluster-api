/*
Copyright 2026 The Kubernetes Authors.

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

// Package validation contains validation code that is used in CABPK and KCP.
package validation

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
)

var (
	cannotUseWithIgnition                            = fmt.Sprintf("not supported when spec.format is set to: %q", bootstrapv1.Ignition)
	conflictingFileSourceMsg                         = "only one of content or contentFrom may be specified for a single file"
	conflictingUserSourceMsg                         = "only one of passwd or passwdFrom may be specified for a single user"
	kubeadmBootstrapFormatIgnitionFeatureDisabledMsg = "can be set only if the KubeadmBootstrapFormatIgnition feature gate is enabled"
	missingSecretNameMsg                             = "secret file source must specify non-empty secret name"
	missingSecretKeyMsg                              = "secret file source must specify non-empty secret key"
	pathConflictMsg                                  = "path property must be unique among all files"
)

// Validate ensures the KubeadmConfigSpec is valid.
func Validate(c *bootstrapv1.KubeadmConfigSpec, isKCP bool, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validateFiles(c, pathPrefix)...)
	allErrs = append(allErrs, validateUsers(c, pathPrefix)...)
	allErrs = append(allErrs, validateIgnition(c, pathPrefix)...)
	allErrs = append(allErrs, validateDiskSetup(c, pathPrefix)...)

	// Validate JoinConfiguration.
	if c.JoinConfiguration.IsDefined() {
		kfg := c.JoinConfiguration.Discovery.File.KubeConfig
		userPath := pathPrefix.Child("joinConfiguration", "discovery", "file", "kubeconfig", "user")
		// Note: MinProperties=1 on User ensures that at least one of AuthProvider or Exec is set
		if kfg.User.AuthProvider.IsDefined() && kfg.User.Exec.IsDefined() {
			allErrs = append(allErrs,
				field.Invalid(
					userPath,
					kfg.User,
					"only one of authProvider or exec must be defined",
				),
			)
		}
	}

	// Only ensure ControlPlaneComponentHealthCheckSeconds fields are equal for KubeadmControlPlane and KubeadmControlPlaneTemplate.
	// In KubeadmConfig objects usually only one of InitConfiguration or JoinConfiguration is defined as a Machine uses
	// either kubeadm init or kubeadm join, but not both.
	if isKCP {
		// Validate timeouts
		// Note: When v1beta1 will be removed, we can drop this limitation.
		tInit := "unset"
		if c.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds != nil {
			tInit = fmt.Sprintf("%d", *c.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds)
		}
		tJoin := "unset"
		if c.JoinConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds != nil {
			tJoin = fmt.Sprintf("%d", *c.JoinConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds)
		}
		if tInit != tJoin {
			allErrs = append(allErrs,
				field.Invalid(
					pathPrefix.Child("initConfiguration", "timeouts", "controlPlaneComponentHealthCheckSeconds"),
					tInit,
					fmt.Sprintf("controlPlaneComponentHealthCheckSeconds must be set to the same value both in initConfiguration.timeouts (%s) and in joinConfiguration.timeouts (%s)", tInit, tJoin),
				),
				field.Invalid(
					pathPrefix.Child("joinConfiguration", "timeouts", "controlPlaneComponentHealthCheckSeconds"),
					tJoin,
					fmt.Sprintf("controlPlaneComponentHealthCheckSeconds must be set to the same value both in initConfiguration.timeouts (%s) and in joinConfiguration.timeouts (%s)", tInit, tJoin),
				),
			)
		}
	}

	return allErrs
}

func validateFiles(c *bootstrapv1.KubeadmConfigSpec, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	knownPaths := map[string]struct{}{}

	for i := range c.Files {
		file := c.Files[i]
		if file.Content != "" && file.ContentFrom.IsDefined() {
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
		if file.ContentFrom.IsDefined() {
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

func validateUsers(c *bootstrapv1.KubeadmConfigSpec, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for i := range c.Users {
		user := c.Users[i]
		if user.Passwd != "" && user.PasswdFrom.IsDefined() {
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
		if user.PasswdFrom.IsDefined() {
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

func validateIgnition(c *bootstrapv1.KubeadmConfigSpec, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !feature.Gates.Enabled(feature.KubeadmBootstrapFormatIgnition) {
		if c.Format == bootstrapv1.Ignition {
			allErrs = append(allErrs, field.Forbidden(
				pathPrefix.Child("format"), kubeadmBootstrapFormatIgnitionFeatureDisabledMsg))
		}

		if c.Ignition.IsDefined() {
			allErrs = append(allErrs, field.Forbidden(
				pathPrefix.Child("ignition"), kubeadmBootstrapFormatIgnitionFeatureDisabledMsg))
		}

		return allErrs
	}

	if c.Format != bootstrapv1.Ignition {
		if c.Ignition.IsDefined() {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("format"),
					c.Format,
					fmt.Sprintf("must be set to %q if spec.ignition is set", bootstrapv1.Ignition),
				),
			)
		}

		return allErrs
	}

	for i, user := range c.Users {
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

	for i, file := range c.Files {
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

	if c.BootCommands != nil {
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("bootCommands"),
				cannotUseWithIgnition,
			),
		)
	}

	for i, partition := range c.DiskSetup.Partitions {
		if partition.TableType != "" && partition.TableType != "gpt" {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("diskSetup", "partitions").Index(i).Child("tableType"),
					partition.TableType,
					fmt.Sprintf(
						"only partition type %q is supported when spec.format is set to %q",
						"gpt",
						bootstrapv1.Ignition,
					),
				),
			)
		}
	}

	for i, fs := range c.DiskSetup.Filesystems {
		if fs.ReplaceFS != "" {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("diskSetup", "filesystems").Index(i).Child("replaceFS"),
					cannotUseWithIgnition,
				),
			)
		}

		if fs.Partition != "" {
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

func validateDiskSetup(c *bootstrapv1.KubeadmConfigSpec, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for i, partition := range c.DiskSetup.Partitions {
		if len(partition.DiskLayout) > 0 {
			var totalPercentage int32
			for _, layout := range partition.DiskLayout {
				totalPercentage += layout.Percentage
			}

			if totalPercentage > 100 {
				allErrs = append(
					allErrs,
					field.Invalid(
						pathPrefix.Child("diskSetup", "partitions").Index(i).Child("diskLayout"),
						totalPercentage,
						"the sum of all partition percentages must not be greater than 100",
					),
				)
			}
		}
	}

	return allErrs
}
