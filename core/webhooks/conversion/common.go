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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

var apiVersionGetter = func(_ context.Context, _ schema.GroupKind) (string, error) {
	return "", errors.New("apiVersionGetter not set")
}

// SetAPIVersionGetter sets an APIVersionGetter that is required during conversion to retrieve
// the APIVersion when converting from v1beta2 to v1beta1.
func SetAPIVersionGetter(f func(ctx context.Context, gk schema.GroupKind) (string, error)) {
	apiVersionGetter = f
}

func convertMachineSpecToContractVersionedObjectReference(src *clusterv1beta1.MachineSpec, dst *clusterv1.MachineSpec) error {
	infraRef, err := convertToContractVersionedObjectReference(&src.InfrastructureRef)
	if err != nil {
		return err
	}
	dst.InfrastructureRef = infraRef

	if src.Bootstrap.ConfigRef != nil {
		bootstrapRef, err := convertToContractVersionedObjectReference(src.Bootstrap.ConfigRef)
		if err != nil {
			return err
		}
		dst.Bootstrap.ConfigRef = bootstrapRef
	}

	return nil
}

func convertMachineSpecToObjectReference(ctx context.Context, src *clusterv1.MachineSpec, dst *clusterv1beta1.MachineSpec, namespace string) error {
	if src.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(ctx, src.InfrastructureRef, namespace)
		if err != nil {
			return err
		}
		dst.InfrastructureRef = *infraRef
	}

	if src.Bootstrap.ConfigRef.IsDefined() {
		bootstrapRef, err := convertToObjectReference(ctx, src.Bootstrap.ConfigRef, namespace)
		if err != nil {
			return err
		}
		dst.Bootstrap.ConfigRef = bootstrapRef
	}

	return nil
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

func dropEmptyStringsMachineSpec(spec *clusterv1beta1.MachineSpec) {
	dropEmptyString(&spec.Version)
	dropEmptyString(&spec.ProviderID)
	dropEmptyString(&spec.FailureDomain)
}

func dropEmptyString(s **string) {
	if *s != nil && **s == "" {
		*s = nil
	}
}
