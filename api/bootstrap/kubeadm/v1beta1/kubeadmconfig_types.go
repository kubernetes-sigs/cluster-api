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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
)

// Format specifies the output format of the bootstrap data
// +kubebuilder:validation:Enum=cloud-config;ignition
type Format string

const (
	// CloudConfig make the bootstrap data to be of cloud-config format.
	CloudConfig Format = "cloud-config"

	// Ignition make the bootstrap data to be of Ignition format.
	Ignition Format = "ignition"
)

var (
	cannotUseWithIgnition                            = fmt.Sprintf("not supported when spec.format is set to: %q", Ignition)
	conflictingFileSourceMsg                         = "only one of content or contentFrom may be specified for a single file"
	conflictingUserSourceMsg                         = "only one of passwd or passwdFrom may be specified for a single user"
	kubeadmBootstrapFormatIgnitionFeatureDisabledMsg = "can be set only if the KubeadmBootstrapFormatIgnition feature gate is enabled"
	missingSecretNameMsg                             = "secret file source must specify non-empty secret name"
	missingSecretKeyMsg                              = "secret file source must specify non-empty secret key"
	pathConflictMsg                                  = "path property must be unique among all files"
)

// KubeadmConfigSpec defines the desired state of KubeadmConfig.
// Either ClusterConfiguration and InitConfiguration should be defined or the JoinConfiguration should be defined.
type KubeadmConfigSpec struct {
	// clusterConfiguration along with InitConfiguration are the configurations necessary for the init command
	// +optional
	ClusterConfiguration *ClusterConfiguration `json:"clusterConfiguration,omitempty"`

	// initConfiguration along with ClusterConfiguration are the configurations necessary for the init command
	// +optional
	InitConfiguration *InitConfiguration `json:"initConfiguration,omitempty"`

	// joinConfiguration is the kubeadm configuration for the join command
	// +optional
	JoinConfiguration *JoinConfiguration `json:"joinConfiguration,omitempty"`

	// files specifies extra files to be passed to user_data upon creation.
	// +optional
	// +kubebuilder:validation:MaxItems=200
	Files []File `json:"files,omitempty"`

	// diskSetup specifies options for the creation of partition tables and file systems on devices.
	// +optional
	DiskSetup *DiskSetup `json:"diskSetup,omitempty"`

	// mounts specifies a list of mount points to be setup.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	Mounts []MountPoints `json:"mounts,omitempty"`

	// bootCommands specifies extra commands to run very early in the boot process via the cloud-init bootcmd
	// module. bootcmd will run on every boot, 'cloud-init-per' command can be used to make bootcmd run exactly
	// once. This is typically run in the cloud-init.service systemd unit. This has no effect in Ignition.
	// +optional
	// +kubebuilder:validation:MaxItems=1000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=10240
	BootCommands []string `json:"bootCommands,omitempty"`

	// preKubeadmCommands specifies extra commands to run before kubeadm runs.
	// With cloud-init, this is prepended to the runcmd module configuration, and is typically executed in
	// the cloud-final.service systemd unit. In Ignition, this is prepended to /etc/kubeadm.sh.
	// +optional
	// +kubebuilder:validation:MaxItems=1000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=10240
	PreKubeadmCommands []string `json:"preKubeadmCommands,omitempty"`

	// postKubeadmCommands specifies extra commands to run after kubeadm runs.
	// With cloud-init, this is appended to the runcmd module configuration, and is typically executed in
	// the cloud-final.service systemd unit. In Ignition, this is appended to /etc/kubeadm.sh.
	// +optional
	// +kubebuilder:validation:MaxItems=1000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=10240
	PostKubeadmCommands []string `json:"postKubeadmCommands,omitempty"`

	// users specifies extra users to add
	// +optional
	// +kubebuilder:validation:MaxItems=100
	Users []User `json:"users,omitempty"`

	// ntp specifies NTP configuration
	// +optional
	NTP *NTP `json:"ntp,omitempty"`

	// format specifies the output format of the bootstrap data
	// +optional
	Format Format `json:"format,omitempty"`

	// verbosity is the number for the kubeadm log level verbosity.
	// It overrides the `--v` flag in kubeadm commands.
	// +optional
	Verbosity *int32 `json:"verbosity,omitempty"`

	// useExperimentalRetryJoin replaces a basic kubeadm command with a shell
	// script with retries for joins.
	//
	// This is meant to be an experimental temporary workaround on some environments
	// where joins fail due to timing (and other issues). The long term goal is to add retries to
	// kubeadm proper and use that functionality.
	//
	// This will add about 40KB to userdata
	//
	// For more information, refer to https://github.com/kubernetes-sigs/cluster-api/pull/2763#discussion_r397306055.
	// +optional
	//
	// Deprecated: This experimental fix is no longer needed and this field will be removed in a future release.
	// When removing also remove from staticcheck exclude-rules for SA1019 in golangci.yml
	UseExperimentalRetryJoin bool `json:"useExperimentalRetryJoin,omitempty"`

	// ignition contains Ignition specific configuration.
	// +optional
	Ignition *IgnitionSpec `json:"ignition,omitempty"`
}

// Default defaults a KubeadmConfigSpec.
func (c *KubeadmConfigSpec) Default() {
	if c.Format == "" {
		c.Format = CloudConfig
	}
	if c.InitConfiguration != nil && c.InitConfiguration.NodeRegistration.ImagePullPolicy == "" {
		c.InitConfiguration.NodeRegistration.ImagePullPolicy = "IfNotPresent"
	}
	if c.JoinConfiguration != nil && c.JoinConfiguration.NodeRegistration.ImagePullPolicy == "" {
		c.JoinConfiguration.NodeRegistration.ImagePullPolicy = "IfNotPresent"
	}
	if c.JoinConfiguration != nil && c.JoinConfiguration.Discovery.File != nil {
		if kfg := c.JoinConfiguration.Discovery.File.KubeConfig; kfg != nil {
			if kfg.User.Exec != nil {
				if kfg.User.Exec.APIVersion == "" {
					kfg.User.Exec.APIVersion = "client.authentication.k8s.io/v1"
				}
			}
		}
	}
}

// Validate ensures the KubeadmConfigSpec is valid.
func (c *KubeadmConfigSpec) Validate(pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, c.validateFiles(pathPrefix)...)
	allErrs = append(allErrs, c.validateUsers(pathPrefix)...)
	allErrs = append(allErrs, c.validateIgnition(pathPrefix)...)

	// Validate JoinConfiguration.
	if c.JoinConfiguration != nil {
		if c.JoinConfiguration.Discovery.File != nil {
			if kfg := c.JoinConfiguration.Discovery.File.KubeConfig; kfg != nil {
				userPath := pathPrefix.Child("joinConfiguration", "discovery", "file", "kubeconfig", "user")
				if kfg.User.AuthProvider == nil && kfg.User.Exec == nil {
					allErrs = append(allErrs,
						field.Invalid(
							userPath,
							kfg.User,
							"at least one of authProvider or exec must be defined",
						),
					)
				}
				if kfg.User.AuthProvider != nil && kfg.User.Exec != nil {
					allErrs = append(allErrs,
						field.Invalid(
							userPath,
							kfg.User,
							"either authProvider or exec must be defined",
						),
					)
				}
			}
		}
	}

	return allErrs
}

func (c *KubeadmConfigSpec) validateFiles(pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	knownPaths := map[string]struct{}{}

	for i := range c.Files {
		file := c.Files[i]
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

func (c *KubeadmConfigSpec) validateUsers(pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for i := range c.Users {
		user := c.Users[i]
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

func (c *KubeadmConfigSpec) validateIgnition(pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !feature.Gates.Enabled(feature.KubeadmBootstrapFormatIgnition) {
		if c.Format == Ignition {
			allErrs = append(allErrs, field.Forbidden(
				pathPrefix.Child("format"), kubeadmBootstrapFormatIgnitionFeatureDisabledMsg))
		}

		if c.Ignition != nil {
			allErrs = append(allErrs, field.Forbidden(
				pathPrefix.Child("ignition"), kubeadmBootstrapFormatIgnitionFeatureDisabledMsg))
		}

		return allErrs
	}

	if c.Format != Ignition {
		if c.Ignition != nil {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("format"),
					c.Format,
					fmt.Sprintf("must be set to %q if spec.ignition is set", Ignition),
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

	if c.UseExperimentalRetryJoin {
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("useExperimentalRetryJoin"),
				cannotUseWithIgnition,
			),
		)
	}

	for i, file := range c.Files {
		if file.Encoding == Gzip || file.Encoding == GzipBase64 {
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

	if c.DiskSetup == nil {
		return allErrs
	}

	for i, partition := range c.DiskSetup.Partitions {
		if partition.TableType != nil && *partition.TableType != "gpt" {
			allErrs = append(
				allErrs,
				field.Invalid(
					pathPrefix.Child("diskSetup", "partitions").Index(i).Child("tableType"),
					*partition.TableType,
					fmt.Sprintf(
						"only partition type %q is supported when spec.format is set to %q",
						"gpt",
						Ignition,
					),
				),
			)
		}
	}

	for i, fs := range c.DiskSetup.Filesystems {
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

// IgnitionSpec contains Ignition specific configuration.
type IgnitionSpec struct {
	// containerLinuxConfig contains CLC specific configuration.
	// +optional
	ContainerLinuxConfig *ContainerLinuxConfig `json:"containerLinuxConfig,omitempty"`
}

// ContainerLinuxConfig contains CLC-specific configuration.
//
// We use a structured type here to allow adding additional fields, for example 'version'.
type ContainerLinuxConfig struct {
	// additionalConfig contains additional configuration to be merged with the Ignition
	// configuration generated by the bootstrapper controller. More info: https://coreos.github.io/ignition/operator-notes/#config-merging
	//
	// The data format is documented here: https://kinvolk.io/docs/flatcar-container-linux/latest/provisioning/cl-config/
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32768
	AdditionalConfig string `json:"additionalConfig,omitempty"`

	// strict controls if AdditionalConfig should be strictly parsed. If so, warnings are treated as errors.
	// +optional
	Strict bool `json:"strict,omitempty"`
}

// KubeadmConfigStatus defines the observed state of KubeadmConfig.
type KubeadmConfigStatus struct {
	// ready indicates the BootstrapData field is ready to be consumed
	// +optional
	Ready bool `json:"ready"`

	// dataSecretName is the name of the secret that stores the bootstrap data script.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	DataSecretName *string `json:"dataSecretName,omitempty"`

	// failureReason will be set on non-retryable errors
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureReason string `json:"failureReason,omitempty"`

	// failureMessage will be set on non-retryable errors
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	FailureMessage string `json:"failureMessage,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions defines current service state of the KubeadmConfig.
	// +optional
	Conditions clusterv1beta1.Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in KubeadmConfig's status with the V1Beta2 version.
	// +optional
	V1Beta2 *KubeadmConfigV1Beta2Status `json:"v1beta2,omitempty"`
}

// KubeadmConfigV1Beta2Status groups all the fields that will be added or modified in KubeadmConfig with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type KubeadmConfigV1Beta2Status struct {
	// conditions represents the observations of a KubeadmConfig's current state.
	// Known condition types are Ready, DataSecretAvailable, CertificatesAvailable.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeadmconfigs,scope=Namespaced,categories=cluster-api
// +kubebuilder:deprecatedversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of KubeadmConfig"

// KubeadmConfig is the Schema for the kubeadmconfigs API.
type KubeadmConfig struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of KubeadmConfig.
	// +optional
	Spec KubeadmConfigSpec `json:"spec,omitempty"`
	// status is the observed state of KubeadmConfig.
	// +optional
	Status KubeadmConfigStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *KubeadmConfig) GetConditions() clusterv1beta1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *KubeadmConfig) SetConditions(conditions clusterv1beta1.Conditions) {
	c.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (c *KubeadmConfig) GetV1Beta2Conditions() []metav1.Condition {
	if c.Status.V1Beta2 == nil {
		return nil
	}
	return c.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (c *KubeadmConfig) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if c.Status.V1Beta2 == nil {
		c.Status.V1Beta2 = &KubeadmConfigV1Beta2Status{}
	}
	c.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// KubeadmConfigList contains a list of KubeadmConfig.
type KubeadmConfigList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of KubeadmConfigs.
	Items []KubeadmConfig `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &KubeadmConfig{}, &KubeadmConfigList{})
}

// Encoding specifies the cloud-init file encoding.
// +kubebuilder:validation:Enum=base64;gzip;gzip+base64
type Encoding string

const (
	// Base64 implies the contents of the file are encoded as base64.
	Base64 Encoding = "base64"
	// Gzip implies the contents of the file are encoded with gzip.
	Gzip Encoding = "gzip"
	// GzipBase64 implies the contents of the file are first base64 encoded and then gzip encoded.
	GzipBase64 Encoding = "gzip+base64"
)

// File defines the input for generating write_files in cloud-init.
type File struct {
	// path specifies the full path on disk where to store the file.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Path string `json:"path"`

	// owner specifies the ownership of the file, e.g. "root:root".
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Owner string `json:"owner,omitempty"`

	// permissions specifies the permissions to assign to the file, e.g. "0640".
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=16
	Permissions string `json:"permissions,omitempty"`

	// encoding specifies the encoding of the file contents.
	// +optional
	Encoding Encoding `json:"encoding,omitempty"`

	// append specifies whether to append Content to existing file if Path exists.
	// +optional
	Append bool `json:"append,omitempty"`

	// content is the actual content of the file.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	Content string `json:"content,omitempty"`

	// contentFrom is a referenced source of content to populate the file.
	// +optional
	ContentFrom *FileSource `json:"contentFrom,omitempty"`
}

// FileSource is a union of all possible external source types for file data.
// Only one field may be populated in any given instance. Developers adding new
// sources of data for target systems should add them here.
type FileSource struct {
	// secret represents a secret that should populate this file.
	// +required
	Secret SecretFileSource `json:"secret"`
}

// SecretFileSource adapts a Secret into a FileSource.
//
// The contents of the target Secret's Data field will be presented
// as files using the keys in the Data field as the file names.
type SecretFileSource struct {
	// name of the secret in the KubeadmBootstrapConfig's namespace to use.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// key is the key in the secret's data map for this value.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Key string `json:"key"`
}

// PasswdSource is a union of all possible external source types for passwd data.
// Only one field may be populated in any given instance. Developers adding new
// sources of data for target systems should add them here.
type PasswdSource struct {
	// secret represents a secret that should populate this password.
	// +required
	Secret SecretPasswdSource `json:"secret"`
}

// SecretPasswdSource adapts a Secret into a PasswdSource.
//
// The contents of the target Secret's Data field will be presented
// as passwd using the keys in the Data field as the file names.
type SecretPasswdSource struct {
	// name of the secret in the KubeadmBootstrapConfig's namespace to use.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// key is the key in the secret's data map for this value.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Key string `json:"key"`
}

// User defines the input for a generated user in cloud-init.
type User struct {
	// name specifies the user name
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`

	// gecos specifies the gecos to use for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Gecos *string `json:"gecos,omitempty"`

	// groups specifies the additional groups for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Groups *string `json:"groups,omitempty"`

	// homeDir specifies the home directory to use for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	HomeDir *string `json:"homeDir,omitempty"`

	// inactive specifies whether to mark the user as inactive
	// +optional
	Inactive *bool `json:"inactive,omitempty"`

	// shell specifies the user's shell
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Shell *string `json:"shell,omitempty"`

	// passwd specifies a hashed password for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Passwd *string `json:"passwd,omitempty"`

	// passwdFrom is a referenced source of passwd to populate the passwd.
	// +optional
	PasswdFrom *PasswdSource `json:"passwdFrom,omitempty"`

	// primaryGroup specifies the primary group for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	PrimaryGroup *string `json:"primaryGroup,omitempty"`

	// lockPassword specifies if password login should be disabled
	// +optional
	LockPassword *bool `json:"lockPassword,omitempty"`

	// sudo specifies a sudo role for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Sudo *string `json:"sudo,omitempty"`

	// sshAuthorizedKeys specifies a list of ssh authorized keys for the user
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=2048
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`
}

// NTP defines input for generated ntp in cloud-init.
type NTP struct {
	// servers specifies which NTP servers to use
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=512
	Servers []string `json:"servers,omitempty"`

	// enabled specifies whether NTP should be enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// DiskSetup defines input for generated disk_setup and fs_setup in cloud-init.
type DiskSetup struct {
	// partitions specifies the list of the partitions to setup.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	Partitions []Partition `json:"partitions,omitempty"`

	// filesystems specifies the list of file systems to setup.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	Filesystems []Filesystem `json:"filesystems,omitempty"`
}

// Partition defines how to create and layout a partition.
type Partition struct {
	// device is the name of the device.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Device string `json:"device"`
	// layout specifies the device layout.
	// If it is true, a single partition will be created for the entire device.
	// When layout is false, it means don't partition or ignore existing partitioning.
	// +required
	Layout bool `json:"layout"`
	// overwrite describes whether to skip checks and create the partition if a partition or filesystem is found on the device.
	// Use with caution. Default is 'false'.
	// +optional
	Overwrite *bool `json:"overwrite,omitempty"`
	// tableType specifies the tupe of partition table. The following are supported:
	// 'mbr': default and setups a MS-DOS partition table
	// 'gpt': setups a GPT partition table
	// +optional
	// +kubebuilder:validation:Enum=mbr;gpt
	TableType *string `json:"tableType,omitempty"`
}

// Filesystem defines the file systems to be created.
type Filesystem struct {
	// device specifies the device name
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Device string `json:"device"`

	// filesystem specifies the file system type.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Filesystem string `json:"filesystem"`

	// label specifies the file system label to be used. If set to None, no label is used.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Label string `json:"label,omitempty"`

	// partition specifies the partition to use. The valid options are: "auto|any", "auto", "any", "none", and <NUM>, where NUM is the actual partition number.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Partition *string `json:"partition,omitempty"`

	// overwrite defines whether or not to overwrite any existing filesystem.
	// If true, any pre-existing file system will be destroyed. Use with Caution.
	// +optional
	Overwrite *bool `json:"overwrite,omitempty"`

	// replaceFS is a special directive, used for Microsoft Azure that instructs cloud-init to replace a file system of <FS_TYPE>.
	// NOTE: unless you define a label, this requires the use of the 'any' partition directive.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ReplaceFS *string `json:"replaceFS,omitempty"`

	// extraOpts defined extra options to add to the command for creating the file system.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	ExtraOpts []string `json:"extraOpts,omitempty"`
}

// MountPoints defines input for generated mounts in cloud-init.
// +kubebuilder:validation:items:MinLength=1
// +kubebuilder:validation:items:MaxLength=512
type MountPoints []string
