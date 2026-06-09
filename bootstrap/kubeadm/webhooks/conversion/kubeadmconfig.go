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

package conversion

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	bootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// KubeadmConfig is a HubSpokeConverter for the KubeadmConfig API type.
var KubeadmConfig = conversion.NewHubSpokeConverter(&bootstrapv1.KubeadmConfig{},
	conversion.NewSpokeConverter(&bootstrapv1beta1.KubeadmConfig{}, ConvertKubeadmConfigHubToV1Beta1, ConvertKubeadmConfigV1Beta1ToHub),
)

// ConvertKubeadmConfigV1Beta1ToHub converts a v1beta1 KubeadmConfig to a hub KubeadmConfig.
func ConvertKubeadmConfigV1Beta1ToHub(ctx context.Context, src *bootstrapv1beta1.KubeadmConfig, dst *bootstrapv1.KubeadmConfig) error {
	if err := bootstrapv1beta1.Convert_v1beta1_KubeadmConfig_To_v1beta2_KubeadmConfig(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &bootstrapv1.KubeadmConfig{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := bootstrapv1.KubeadmConfigInitializationStatus{}
	restoredBootstrapDataSecretCreated := restored.Status.Initialization.DataSecretCreated
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Ready, ok, restoredBootstrapDataSecretCreated, &initialization.DataSecretCreated)
	if !reflect.DeepEqual(initialization, bootstrapv1.KubeadmConfigInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}
	if err := RestoreBoolIntentKubeadmConfigSpec(&src.Spec, &dst.Spec, ok, &restored.Spec); err != nil {
		return err
	}

	// Recover other values
	if ok {
		RestoreKubeadmConfigSpec(&restored.Spec, &dst.Spec)
	}

	// Override restored data with timeouts values already existing in v1beta1 but in other structs.
	ConvertKubeadmConfigSpecV1Beta1ToHub(ctx, &src.Spec, &dst.Spec)
	return nil
}

// RestoreKubeadmConfigSpec restores a KubeadmConfigSpec.
func RestoreKubeadmConfigSpec(restored *bootstrapv1.KubeadmConfigSpec, dst *bootstrapv1.KubeadmConfigSpec) {
	// Restore fields added in v1beta2
	// Note: Because timeout fields partially exist already in v1beta1 we are using the conversion annotation
	// instead of backporting the entire timeout fields to v1beta1 and then having some duplicate timeout fields.
	if restored.InitConfiguration.IsDefined() && !reflect.DeepEqual(restored.InitConfiguration.Timeouts, bootstrapv1.Timeouts{}) {
		dst.InitConfiguration.Timeouts = restored.InitConfiguration.Timeouts
	}
	if restored.JoinConfiguration.IsDefined() && !reflect.DeepEqual(restored.JoinConfiguration.Timeouts, bootstrapv1.Timeouts{}) {
		dst.JoinConfiguration.Timeouts = restored.JoinConfiguration.Timeouts
	}
}

// RestoreBoolIntentKubeadmConfigSpec restores bool intent of a KubeadmConfigSpec.
func RestoreBoolIntentKubeadmConfigSpec(src *bootstrapv1beta1.KubeadmConfigSpec, dst *bootstrapv1.KubeadmConfigSpec, hasRestored bool, restored *bootstrapv1.KubeadmConfigSpec) error {
	if dst.JoinConfiguration.Discovery.BootstrapToken.IsDefined() {
		restoredUnsafeSkipCAVerification := restored.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification
		clusterv1.Convert_bool_To_Pointer_bool(src.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification, hasRestored, restoredUnsafeSkipCAVerification, &dst.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification)
	}

	if dst.JoinConfiguration.Discovery.File.IsDefined() && dst.JoinConfiguration.Discovery.File.KubeConfig.IsDefined() {
		if dst.JoinConfiguration.Discovery.File.KubeConfig.Cluster.IsDefined() {
			restoredInsecureSkipTLSVerify := restored.JoinConfiguration.Discovery.File.KubeConfig.Cluster.InsecureSkipTLSVerify
			clusterv1.Convert_bool_To_Pointer_bool(src.JoinConfiguration.Discovery.File.KubeConfig.Cluster.InsecureSkipTLSVerify, hasRestored, restoredInsecureSkipTLSVerify, &dst.JoinConfiguration.Discovery.File.KubeConfig.Cluster.InsecureSkipTLSVerify)
		}
		if dst.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.IsDefined() {
			restoredExecProvideClusterInfo := restored.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.ProvideClusterInfo
			clusterv1.Convert_bool_To_Pointer_bool(src.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.ProvideClusterInfo, hasRestored, restoredExecProvideClusterInfo, &dst.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.ProvideClusterInfo)
		}
	}

	for i, volume := range dst.ClusterConfiguration.APIServer.ExtraVolumes {
		var srcVolume *bootstrapv1beta1.HostPathMount
		if src.ClusterConfiguration != nil {
			for _, v := range src.ClusterConfiguration.APIServer.ExtraVolumes {
				if v.HostPath == volume.HostPath {
					srcVolume = &v
					break
				}
			}
		}
		if srcVolume == nil {
			return fmt.Errorf("apiServer extraVolume with hostPath %q not found in source data", volume.HostPath)
		}
		var restoredVolumeReadOnly *bool
		for _, v := range restored.ClusterConfiguration.APIServer.ExtraVolumes {
			if v.HostPath == volume.HostPath {
				restoredVolumeReadOnly = v.ReadOnly
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcVolume.ReadOnly, hasRestored, restoredVolumeReadOnly, &volume.ReadOnly)
		dst.ClusterConfiguration.APIServer.ExtraVolumes[i] = volume
	}
	for i, volume := range dst.ClusterConfiguration.ControllerManager.ExtraVolumes {
		var srcVolume *bootstrapv1beta1.HostPathMount
		if src.ClusterConfiguration != nil {
			for _, v := range src.ClusterConfiguration.ControllerManager.ExtraVolumes {
				if v.HostPath == volume.HostPath {
					srcVolume = &v
					break
				}
			}
		}
		if srcVolume == nil {
			return fmt.Errorf("controllerManager extraVolume with hostPath %q not found in source data", volume.HostPath)
		}
		var restoredVolumeReadOnly *bool
		for _, v := range restored.ClusterConfiguration.ControllerManager.ExtraVolumes {
			if v.HostPath == volume.HostPath {
				restoredVolumeReadOnly = v.ReadOnly
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcVolume.ReadOnly, hasRestored, restoredVolumeReadOnly, &volume.ReadOnly)
		dst.ClusterConfiguration.ControllerManager.ExtraVolumes[i] = volume
	}
	for i, volume := range dst.ClusterConfiguration.Scheduler.ExtraVolumes {
		var srcVolume *bootstrapv1beta1.HostPathMount
		if src.ClusterConfiguration != nil {
			for _, v := range src.ClusterConfiguration.Scheduler.ExtraVolumes {
				if v.HostPath == volume.HostPath {
					srcVolume = &v
					break
				}
			}
		}
		if srcVolume == nil {
			return fmt.Errorf("scheduler extraVolume with hostPath %q not found in source data", volume.HostPath)
		}
		var restoredVolumeReadOnly *bool
		for _, v := range restored.ClusterConfiguration.Scheduler.ExtraVolumes {
			if v.HostPath == volume.HostPath {
				restoredVolumeReadOnly = v.ReadOnly
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcVolume.ReadOnly, hasRestored, restoredVolumeReadOnly, &volume.ReadOnly)
		dst.ClusterConfiguration.Scheduler.ExtraVolumes[i] = volume
	}

	for i, file := range dst.Files {
		var srcFile *bootstrapv1beta1.File
		for _, f := range src.Files {
			if f.Path == file.Path {
				srcFile = &f
				break
			}
		}
		if srcFile == nil {
			return fmt.Errorf("file with path %q not found in source data", file.Path)
		}
		var restoredFileAppend *bool
		for _, f := range restored.Files {
			if f.Path == file.Path {
				restoredFileAppend = f.Append
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcFile.Append, hasRestored, restoredFileAppend, &file.Append)
		dst.Files[i] = file
	}

	if dst.Ignition.IsDefined() && dst.Ignition.ContainerLinuxConfig.IsDefined() {
		restoredIgnitionStrict := restored.Ignition.ContainerLinuxConfig.Strict
		clusterv1.Convert_bool_To_Pointer_bool(src.Ignition.ContainerLinuxConfig.Strict, hasRestored, restoredIgnitionStrict, &dst.Ignition.ContainerLinuxConfig.Strict)
	}
	return nil
}

// ConvertKubeadmConfigSpecV1Beta1ToHub converts a v1beta1 KubeadmConfigSpec to a hub KubeadmConfigSpec.
func ConvertKubeadmConfigSpecV1Beta1ToHub(_ context.Context, src *bootstrapv1beta1.KubeadmConfigSpec, dst *bootstrapv1.KubeadmConfigSpec) {
	// Override with timeouts values already existing in v1beta1.
	var initControlPlaneComponentHealthCheckSeconds *int32
	if src.ClusterConfiguration != nil && src.ClusterConfiguration.APIServer.TimeoutForControlPlane != nil {
		dst.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = clusterv1.ConvertToSeconds(src.ClusterConfiguration.APIServer.TimeoutForControlPlane)
		initControlPlaneComponentHealthCheckSeconds = dst.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds
	}
	if (src.JoinConfiguration != nil && src.JoinConfiguration.Discovery.Timeout != nil) || initControlPlaneComponentHealthCheckSeconds != nil {
		dst.JoinConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = initControlPlaneComponentHealthCheckSeconds
		if src.JoinConfiguration != nil && src.JoinConfiguration.Discovery.Timeout != nil {
			dst.JoinConfiguration.Timeouts.TLSBootstrapSeconds = clusterv1.ConvertToSeconds(src.JoinConfiguration.Discovery.Timeout)
		}
	}
}

// ConvertKubeadmConfigHubToV1Beta1 converts a hub KubeadmConfig to a v1beta1 KubeadmConfig.
func ConvertKubeadmConfigHubToV1Beta1(ctx context.Context, src *bootstrapv1.KubeadmConfig, dst *bootstrapv1beta1.KubeadmConfig) error {
	if err := bootstrapv1beta1.Convert_v1beta2_KubeadmConfig_To_v1beta1_KubeadmConfig(src, dst, nil); err != nil {
		return err
	}

	// Convert timeouts moved from one struct to another.
	ConvertKubeadmConfigSpecHubToV1Beta1(ctx, &src.Spec, &dst.Spec)

	dropEmptyStringsKubeadmConfigSpec(&dst.Spec)
	dropEmptyStringsKubeadmConfigStatus(&dst.Status)

	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalDataUnsafeNoCopy(src, dst)
}

// ConvertKubeadmConfigSpecHubToV1Beta1 converts a hub KubeadmConfigSpec to a v1beta1 KubeadmConfigSpec.
func ConvertKubeadmConfigSpecHubToV1Beta1(_ context.Context, src *bootstrapv1.KubeadmConfigSpec, dst *bootstrapv1beta1.KubeadmConfigSpec) {
	// Convert timeouts moved from one struct to another.
	if src.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds != nil {
		if dst.ClusterConfiguration == nil {
			dst.ClusterConfiguration = &bootstrapv1beta1.ClusterConfiguration{}
		}
		dst.ClusterConfiguration.APIServer.TimeoutForControlPlane = clusterv1.ConvertFromSeconds(src.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds)
	}
	if reflect.DeepEqual(dst.InitConfiguration, &bootstrapv1beta1.InitConfiguration{}) {
		dst.InitConfiguration = nil
	}
	if src.JoinConfiguration.Timeouts.TLSBootstrapSeconds != nil {
		if dst.JoinConfiguration == nil {
			dst.JoinConfiguration = &bootstrapv1beta1.JoinConfiguration{}
		}
		dst.JoinConfiguration.Discovery.Timeout = clusterv1.ConvertFromSeconds(src.JoinConfiguration.Timeouts.TLSBootstrapSeconds)
	}
	if reflect.DeepEqual(dst.JoinConfiguration, &bootstrapv1beta1.JoinConfiguration{}) {
		dst.JoinConfiguration = nil
	}
}

func dropEmptyStringsKubeadmConfigSpec(dst *bootstrapv1beta1.KubeadmConfigSpec) {
	for i, u := range dst.Users {
		dropEmptyString(&u.Gecos)
		dropEmptyString(&u.Groups)
		dropEmptyString(&u.HomeDir)
		dropEmptyString(&u.Shell)
		dropEmptyString(&u.Passwd)
		dropEmptyString(&u.PrimaryGroup)
		dropEmptyString(&u.Sudo)
		dst.Users[i] = u
	}

	if dst.DiskSetup != nil {
		for i, p := range dst.DiskSetup.Partitions {
			dropEmptyString(&p.TableType)
			dst.DiskSetup.Partitions[i] = p
		}
		for i, f := range dst.DiskSetup.Filesystems {
			dropEmptyString(&f.Partition)
			dropEmptyString(&f.ReplaceFS)
			dst.DiskSetup.Filesystems[i] = f
		}
	}
}

func dropEmptyStringsKubeadmConfigStatus(dst *bootstrapv1beta1.KubeadmConfigStatus) {
	dropEmptyString(&dst.DataSecretName)
}

func dropEmptyString(s **string) {
	if *s != nil && **s == "" {
		*s = nil
	}
}
