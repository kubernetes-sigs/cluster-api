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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	bootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	controlplanev1beta1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	bootstrapconversion "sigs.k8s.io/cluster-api/bootstrap/kubeadm/webhooks/conversion"
	conversionutil "sigs.k8s.io/cluster-api/util/conversion"
)

// KubeadmControlPlane is a HubSpokeConverter for the KubeadmControlPlane API type.
var KubeadmControlPlane = conversion.NewHubSpokeConverter(&controlplanev1.KubeadmControlPlane{},
	conversion.NewSpokeConverter(&controlplanev1beta1.KubeadmControlPlane{}, ConvertKubeadmControlPlaneHubToV1Beta1, ConvertKubeadmControlPlaneV1Beta1ToHub),
)

// ConvertKubeadmControlPlaneV1Beta1ToHub converts a v1beta1 KubeadmControlPlane to a hub KubeadmControlPlane.
func ConvertKubeadmControlPlaneV1Beta1ToHub(ctx context.Context, src *controlplanev1beta1.KubeadmControlPlane, dst *controlplanev1.KubeadmControlPlane) error {
	if err := controlplanev1beta1.Convert_v1beta1_KubeadmControlPlane_To_v1beta2_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	infraRef, err := convertToContractVersionedObjectReference(&src.Spec.MachineTemplate.InfrastructureRef)
	if err != nil {
		return err
	}
	dst.Spec.MachineTemplate.Spec.InfrastructureRef = *infraRef

	// Manually restore data.
	restored := &controlplanev1.KubeadmControlPlane{}
	ok, err := conversionutil.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := controlplanev1.KubeadmControlPlaneInitializationStatus{}
	restoredControlPlaneInitialized := restored.Status.Initialization.ControlPlaneInitialized
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.Initialized, ok, restoredControlPlaneInitialized, &initialization.ControlPlaneInitialized)
	if !reflect.DeepEqual(initialization, controlplanev1.KubeadmControlPlaneInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	if err := bootstrapconversion.RestoreBoolIntentKubeadmConfigSpec(&src.Spec.KubeadmConfigSpec, &dst.Spec.KubeadmConfigSpec, ok, &restored.Spec.KubeadmConfigSpec); err != nil {
		return err
	}

	// Recover other values
	if ok {
		bootstrapconversion.RestoreKubeadmConfigSpec(&restored.Spec.KubeadmConfigSpec, &dst.Spec.KubeadmConfigSpec)
		// EtcdMaintenance (including MinDefragInterval) is a v1beta2-only field; restore from annotation.
		dst.Spec.EtcdMaintenance = restored.Spec.EtcdMaintenance
		// EtcdMemberDefragTimes is a v1beta2-only status field; restore from annotation.
		dst.Status.EtcdMemberDefragTimes = restored.Status.EtcdMemberDefragTimes
	}

	if src.Spec.RemediationStrategy != nil {
		clusterv1.Convert_Duration_To_Pointer_int32(src.Spec.RemediationStrategy.RetryPeriod, ok, restored.Spec.Remediation.RetryPeriodSeconds, &dst.Spec.Remediation.RetryPeriodSeconds)
	}

	// Override restored data with timeouts values already existing in v1beta1 but in other structs.
	bootstrapconversion.ConvertKubeadmConfigSpecV1Beta1ToHub(ctx, &src.Spec.KubeadmConfigSpec, &dst.Spec.KubeadmConfigSpec)
	return nil
}

// ConvertKubeadmControlPlaneHubToV1Beta1 converts a hub KubeadmControlPlane to a v1beta1 KubeadmControlPlane.
func ConvertKubeadmControlPlaneHubToV1Beta1(ctx context.Context, src *controlplanev1.KubeadmControlPlane, dst *controlplanev1beta1.KubeadmControlPlane) error {
	if err := controlplanev1beta1.Convert_v1beta2_KubeadmControlPlane_To_v1beta1_KubeadmControlPlane(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.MachineTemplate.Spec.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(ctx, &src.Spec.MachineTemplate.Spec.InfrastructureRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.MachineTemplate.InfrastructureRef = *infraRef
	}

	// Convert timeouts moved from one struct to another.
	bootstrapconversion.ConvertKubeadmConfigSpecHubToV1Beta1(ctx, &src.Spec.KubeadmConfigSpec, &dst.Spec.KubeadmConfigSpec)

	dropEmptyStringsKubeadmConfigSpec(&dst.Spec.KubeadmConfigSpec)
	dropEmptyStringsKubeadmControlPlaneStatus(&dst.Status)

	// Preserve Hub data on down-conversion except for metadata.
	return conversionutil.MarshalDataUnsafeNoCopy(src, dst)
}

func convertToContractVersionedObjectReference(ref *corev1.ObjectReference) (*clusterv1.ContractVersionedObjectReference, error) {
	var apiGroup string
	if ref.APIVersion != "" {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to convert object: failed to parse apiVersion: %v", err)
		}
		apiGroup = gv.Group
	}
	return &clusterv1.ContractVersionedObjectReference{
		APIGroup: apiGroup,
		Kind:     ref.Kind,
		Name:     ref.Name,
	}, nil
}

func convertToObjectReference(ctx context.Context, ref *clusterv1.ContractVersionedObjectReference, namespace string) (*corev1.ObjectReference, error) {
	apiVersion, err := apiVersionGetter(ctx, schema.GroupKind{
		Group: ref.APIGroup,
		Kind:  ref.Kind,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to convert object: %v", err)
	}
	return &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       ref.Kind,
		Namespace:  namespace,
		Name:       ref.Name,
	}, nil
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

func dropEmptyStringsKubeadmControlPlaneStatus(dst *controlplanev1beta1.KubeadmControlPlaneStatus) {
	dropEmptyString(&dst.Version)
}

func dropEmptyString(s **string) {
	if *s != nil && **s == "" {
		*s = nil
	}
}
