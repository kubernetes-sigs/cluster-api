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

package converter

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	webhookconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conversion"
)

var apiVersionGetter = func(context.Context, schema.GroupKind) (string, error) {
	return "", errors.New("apiVersionGetter not set")
}

func SetAPIVersionGetter(f func(context.Context, schema.GroupKind) (string, error)) {
	apiVersionGetter = f
}

var Cluster = webhookconversion.NewHubSpokeConverter(&clusterv1.Cluster{},
	webhookconversion.NewSpokeConverter(&clusterv1beta1.Cluster{}, ConvertClusterHubToV1Beta1, ConvertClusterV1Beta1ToHub),
)

func ConvertClusterV1Beta1ToHub(_ context.Context, src *clusterv1beta1.Cluster, dst *clusterv1.Cluster) error {
	if err := clusterv1beta1.Convert_v1beta1_Cluster_To_v1beta2_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef != nil {
		infraRef, err := convertToContractVersionedObjectReference(src.Spec.InfrastructureRef)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef != nil {
		controlPlaneRef, err := convertToContractVersionedObjectReference(src.Spec.ControlPlaneRef)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	restored := &clusterv1.Cluster{}
	ok, err := conversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Paused, ok, restored.Spec.Paused, &dst.Spec.Paused)

	initialization := clusterv1.ClusterInitializationStatus{}
	restoredControlPlaneInitialized := restored.Status.Initialization.ControlPlaneInitialized
	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.ControlPlaneReady, ok, restoredControlPlaneInitialized, &initialization.ControlPlaneInitialized)
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
	if !reflect.DeepEqual(initialization, clusterv1.ClusterInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}
	return nil
}

func ConvertClusterHubToV1Beta1(ctx context.Context, src *clusterv1.Cluster, dst *clusterv1beta1.Cluster) error {
	if err := clusterv1beta1.Convert_v1beta2_Cluster_To_v1beta1_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(ctx, src.Spec.InfrastructureRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef.IsDefined() {
		controlPlaneRef, err := convertToObjectReference(ctx, src.Spec.ControlPlaneRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	if dst.Spec.ClusterNetwork != nil && dst.Spec.ClusterNetwork.APIServerPort != nil &&
		*dst.Spec.ClusterNetwork.APIServerPort == 0 {
		dst.Spec.ClusterNetwork.APIServerPort = nil
	}

	if dst.Spec.Topology != nil {
		if dst.Spec.Topology.ControlPlane.MachineHealthCheck != nil && dst.Spec.Topology.ControlPlane.MachineHealthCheck.RemediationTemplate != nil {
			dst.Spec.Topology.ControlPlane.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
		}
		if dst.Spec.Topology.Workers != nil {
			for _, md := range dst.Spec.Topology.Workers.MachineDeployments {
				if md.MachineHealthCheck != nil && md.MachineHealthCheck.RemediationTemplate != nil {
					md.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
				}
			}
		}
	}

	dropEmptyStringsCluster(dst)

	return conversion.MarshalData(src, dst)
}

func convertToContractVersionedObjectReference(ref *corev1.ObjectReference) (clusterv1.ContractVersionedObjectReference, error) {
	var apiGroup string
	if ref.APIVersion != "" {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return clusterv1.ContractVersionedObjectReference{}, fmt.Errorf("failed to convert object: failed to parse apiVersion: %v", err)
		}
		apiGroup = gv.Group
	}
	return clusterv1.ContractVersionedObjectReference{
		APIGroup: apiGroup,
		Kind:     ref.Kind,
		Name:     ref.Name,
	}, nil
}

func convertToObjectReference(ctx context.Context, ref clusterv1.ContractVersionedObjectReference, namespace string) (*corev1.ObjectReference, error) {
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

func dropEmptyStringsCluster(dst *clusterv1beta1.Cluster) {
	if dst.Spec.Topology != nil {
		if dst.Spec.Topology.Workers != nil {
			for i, md := range dst.Spec.Topology.Workers.MachineDeployments {
				dropEmptyString(&md.FailureDomain)
				dst.Spec.Topology.Workers.MachineDeployments[i] = md
			}
		}
	}
}

func dropEmptyString(s **string) {
	if *s != nil && **s == "" {
		*s = nil
	}
}
