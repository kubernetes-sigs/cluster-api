/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import (
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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
// +kubebuilder:validation:MinProperties=1
type KubeadmConfigSpec struct {
	// clusterConfiguration along with InitConfiguration are the configurations necessary for the init command
	// +optional
	ClusterConfiguration ClusterConfiguration `json:"clusterConfiguration,omitempty,omitzero"`

	// initConfiguration along with ClusterConfiguration are the configurations necessary for the init command
	// +optional
	InitConfiguration InitConfiguration `json:"initConfiguration,omitempty,omitzero"`

	// joinConfiguration is the kubeadm configuration for the join command
	// +optional
	JoinConfiguration JoinConfiguration `json:"joinConfiguration,omitempty,omitzero"`

	// files specifies extra files to be passed to user_data upon creation.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=200
	Files []File `json:"files,omitempty"`

	// diskSetup specifies options for the creation of partition tables and file systems on devices.
	// +optional
	DiskSetup DiskSetup `json:"diskSetup,omitempty,omitzero"`

	// mounts specifies a list of mount points to be setup.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Mounts []MountPoints `json:"mounts,omitempty"`

	// bootCommands specifies extra commands to run very early in the boot process via the cloud-init bootcmd
	// module. bootcmd will run on every boot, 'cloud-init-per' command can be used to make bootcmd run exactly
	// once. This is typically run in the cloud-init.service systemd unit. This has no effect in Ignition.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=10240
	BootCommands []string `json:"bootCommands,omitempty"`

	// preKubeadmCommands specifies extra commands to run before kubeadm runs.
	// With cloud-init, this is prepended to the runcmd module configuration, and is typically executed in
	// the cloud-final.service systemd unit. In Ignition, this is prepended to /etc/kubeadm.sh.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=10240
	PreKubeadmCommands []string `json:"preKubeadmCommands,omitempty"`

	// postKubeadmCommands specifies extra commands to run after kubeadm runs.
	// With cloud-init, this is appended to the runcmd module configuration, and is typically executed in
	// the cloud-final.service systemd unit. In Ignition, this is appended to /etc/kubeadm.sh.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=10240
	PostKubeadmCommands []string `json:"postKubeadmCommands,omitempty"`

	// users specifies extra users to add
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Users []User `json:"users,omitempty"`

	// ntp specifies NTP configuration
	// +optional
	NTP NTP `json:"ntp,omitempty,omitzero"`

	// format specifies the output format of the bootstrap data.
	// Defaults to cloud-config if not set.
	// +optional
	Format Format `json:"format,omitempty"`

	// verbosity is the number for the kubeadm log level verbosity.
	// It overrides the `--v` flag in kubeadm commands.
	// +optional
	Verbosity *int32 `json:"verbosity,omitempty"`

	// ignition contains Ignition specific configuration.
	// +optional
	Ignition IgnitionSpec `json:"ignition,omitempty,omitzero"`
}

// Validate ensures the KubeadmConfigSpec is valid.
func (c *KubeadmConfigSpec) Validate(isKCP bool, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, c.validateFiles(pathPrefix)...)
	allErrs = append(allErrs, c.validateUsers(pathPrefix)...)
	allErrs = append(allErrs, c.validateIgnition(pathPrefix)...)

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

func (c *KubeadmConfigSpec) validateFiles(pathPrefix *field.Path) field.ErrorList {
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

func (c *KubeadmConfigSpec) validateUsers(pathPrefix *field.Path) field.ErrorList {
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

func (c *KubeadmConfigSpec) validateIgnition(pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !feature.Gates.Enabled(feature.KubeadmBootstrapFormatIgnition) {
		if c.Format == Ignition {
			allErrs = append(allErrs, field.Forbidden(
				pathPrefix.Child("format"), kubeadmBootstrapFormatIgnitionFeatureDisabledMsg))
		}

		if c.Ignition.IsDefined() {
			allErrs = append(allErrs, field.Forbidden(
				pathPrefix.Child("ignition"), kubeadmBootstrapFormatIgnitionFeatureDisabledMsg))
		}

		return allErrs
	}

	if c.Format != Ignition {
		if c.Ignition.IsDefined() {
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
						Ignition,
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

// IgnitionSpec contains Ignition specific configuration.
// +kubebuilder:validation:MinProperties=1
type IgnitionSpec struct {
	// containerLinuxConfig contains CLC specific configuration.
	// +optional
	ContainerLinuxConfig ContainerLinuxConfig `json:"containerLinuxConfig,omitempty,omitzero"`
}

// IsDefined returns true if the IgnitionSpec is defined.
func (r *IgnitionSpec) IsDefined() bool {
	return !reflect.DeepEqual(r, &IgnitionSpec{})
}

// ContainerLinuxConfig contains CLC-specific configuration.
//
// We use a structured type here to allow adding additional fields, for example 'version'.
// +kubebuilder:validation:MinProperties=1
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
	Strict *bool `json:"strict,omitempty"`
}

// IsDefined returns true if the ContainerLinuxConfig is defined.
func (r *ContainerLinuxConfig) IsDefined() bool {
	return !reflect.DeepEqual(r, &ContainerLinuxConfig{})
}

// KubeadmConfigStatus defines the observed state of KubeadmConfig.
// +kubebuilder:validation:MinProperties=1
type KubeadmConfigStatus struct {
	// conditions represents the observations of a KubeadmConfig's current state.
	// Known condition types are Ready, DataSecretAvailable, CertificatesAvailable.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// initialization provides observations of the KubeadmConfig initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Machine provisioning.
	// +optional
	Initialization KubeadmConfigInitializationStatus `json:"initialization,omitempty,omitzero"`

	// dataSecretName is the name of the secret that stores the bootstrap data script.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	DataSecretName string `json:"dataSecretName,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *KubeadmConfigDeprecatedStatus `json:"deprecated,omitempty"`
}

// KubeadmConfigInitializationStatus provides observations of the KubeadmConfig initialization process.
// +kubebuilder:validation:MinProperties=1
type KubeadmConfigInitializationStatus struct {
	// dataSecretCreated is true when the Machine's boostrap secret is created.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Machine provisioning.
	// +optional
	DataSecretCreated *bool `json:"dataSecretCreated,omitempty"`
}

// KubeadmConfigDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type KubeadmConfigDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *KubeadmConfigV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// KubeadmConfigV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type KubeadmConfigV1Beta1DeprecatedStatus struct {
	// conditions defines current service state of the KubeadmConfig.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

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
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kubeadmconfigs,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Data secret created",type="string",JSONPath=`.status.initialization.dataSecretCreated`,description="Boostrap secret is created"
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
	Spec KubeadmConfigSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of KubeadmConfig.
	// +optional
	Status KubeadmConfigStatus `json:"status,omitempty,omitzero"`
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (c *KubeadmConfig) GetV1Beta1Conditions() clusterv1.Conditions {
	if c.Status.Deprecated == nil || c.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return c.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (c *KubeadmConfig) SetV1Beta1Conditions(conditions clusterv1.Conditions) {
	if c.Status.Deprecated == nil {
		c.Status.Deprecated = &KubeadmConfigDeprecatedStatus{}
	}
	if c.Status.Deprecated.V1Beta1 == nil {
		c.Status.Deprecated.V1Beta1 = &KubeadmConfigV1Beta1DeprecatedStatus{}
	}
	c.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (c *KubeadmConfig) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (c *KubeadmConfig) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
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
	Path string `json:"path,omitempty"`

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
	Append *bool `json:"append,omitempty"`

	// content is the actual content of the file.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	Content string `json:"content,omitempty"`

	// contentFrom is a referenced source of content to populate the file.
	// +optional
	ContentFrom FileSource `json:"contentFrom,omitempty,omitzero"`
}

// FileSource is a union of all possible external source types for file data.
// Only one field may be populated in any given instance. Developers adding new
// sources of data for target systems should add them here.
type FileSource struct {
	// secret represents a secret that should populate this file.
	// +required
	Secret SecretFileSource `json:"secret,omitempty,omitzero"`
}

// IsDefined returns true if the FileSource is defined.
func (r *FileSource) IsDefined() bool {
	return !reflect.DeepEqual(r, &FileSource{})
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
	Name string `json:"name,omitempty"`

	// key is the key in the secret's data map for this value.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Key string `json:"key,omitempty"`
}

// PasswdSource is a union of all possible external source types for passwd data.
// Only one field may be populated in any given instance. Developers adding new
// sources of data for target systems should add them here.
type PasswdSource struct {
	// secret represents a secret that should populate this password.
	// +required
	Secret SecretPasswdSource `json:"secret,omitempty,omitzero"`
}

// IsDefined returns true if the PasswdSource is defined.
func (r *PasswdSource) IsDefined() bool {
	return !reflect.DeepEqual(r, &PasswdSource{})
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
	Name string `json:"name,omitempty"`

	// key is the key in the secret's data map for this value.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Key string `json:"key,omitempty"`
}

// User defines the input for a generated user in cloud-init.
type User struct {
	// name specifies the user name
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name,omitempty"`

	// gecos specifies the gecos to use for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Gecos string `json:"gecos,omitempty"`

	// groups specifies the additional groups for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Groups string `json:"groups,omitempty"`

	// homeDir specifies the home directory to use for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	HomeDir string `json:"homeDir,omitempty"`

	// inactive specifies whether to mark the user as inactive
	// +optional
	Inactive *bool `json:"inactive,omitempty"`

	// shell specifies the user's shell
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Shell string `json:"shell,omitempty"`

	// passwd specifies a hashed password for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Passwd string `json:"passwd,omitempty"`

	// passwdFrom is a referenced source of passwd to populate the passwd.
	// +optional
	PasswdFrom PasswdSource `json:"passwdFrom,omitempty,omitzero"`

	// primaryGroup specifies the primary group for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	PrimaryGroup string `json:"primaryGroup,omitempty"`

	// lockPassword specifies if password login should be disabled
	// +optional
	LockPassword *bool `json:"lockPassword,omitempty"`

	// sudo specifies a sudo role for the user
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Sudo string `json:"sudo,omitempty"`

	// sshAuthorizedKeys specifies a list of ssh authorized keys for the user
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=2048
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`
}

// NTP defines input for generated ntp in cloud-init.
// +kubebuilder:validation:MinProperties=1
type NTP struct {
	// servers specifies which NTP servers to use
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=512
	Servers []string `json:"servers,omitempty"`

	// enabled specifies whether NTP should be enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// IsDefined returns true if the NTP is defined.
func (r *NTP) IsDefined() bool {
	return !reflect.DeepEqual(r, &NTP{})
}

// DiskSetup defines input for generated disk_setup and fs_setup in cloud-init.
// +kubebuilder:validation:MinProperties=1
type DiskSetup struct {
	// partitions specifies the list of the partitions to setup.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	Partitions []Partition `json:"partitions,omitempty"`

	// filesystems specifies the list of file systems to setup.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	Filesystems []Filesystem `json:"filesystems,omitempty"`
}

// IsDefined returns true if the DiskSetup is defined.
func (r *DiskSetup) IsDefined() bool {
	return !reflect.DeepEqual(r, &DiskSetup{})
}

// Partition defines how to create and layout a partition.
type Partition struct {
	// device is the name of the device.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Device string `json:"device,omitempty"`

	// layout specifies the device layout.
	// If it is true, a single partition will be created for the entire device.
	// When layout is false, it means don't partition or ignore existing partitioning.
	// +required
	Layout *bool `json:"layout,omitempty"`

	// overwrite describes whether to skip checks and create the partition if a partition or filesystem is found on the device.
	// Use with caution. Default is 'false'.
	// +optional
	Overwrite *bool `json:"overwrite,omitempty"`

	// tableType specifies the tupe of partition table. The following are supported:
	// 'mbr': default and setups a MS-DOS partition table
	// 'gpt': setups a GPT partition table
	// +optional
	// +kubebuilder:validation:Enum=mbr;gpt
	TableType string `json:"tableType,omitempty"`
}

// Filesystem defines the file systems to be created.
type Filesystem struct {
	// device specifies the device name
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Device string `json:"device,omitempty"`

	// filesystem specifies the file system type.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Filesystem string `json:"filesystem,omitempty"`

	// label specifies the file system label to be used. If set to None, no label is used.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Label string `json:"label,omitempty"`

	// partition specifies the partition to use. The valid options are: "auto|any", "auto", "any", "none", and <NUM>, where NUM is the actual partition number.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Partition string `json:"partition,omitempty"`

	// overwrite defines whether or not to overwrite any existing filesystem.
	// If true, any pre-existing file system will be destroyed. Use with Caution.
	// +optional
	Overwrite *bool `json:"overwrite,omitempty"`

	// replaceFS is a special directive, used for Microsoft Azure that instructs cloud-init to replace a file system of <FS_TYPE>.
	// NOTE: unless you define a label, this requires the use of the 'any' partition directive.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ReplaceFS string `json:"replaceFS,omitempty"`

	// extraOpts defined extra options to add to the command for creating the file system.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	ExtraOpts []string `json:"extraOpts,omitempty"`
}

// MountPoints defines input for generated mounts in cloud-init.
// +kubebuilder:validation:MinItems=1
// +kubebuilder:validation:MaxItems=100
// +kubebuilder:validation:items:MinLength=1
// +kubebuilder:validation:items:MaxLength=512
type MountPoints []string
