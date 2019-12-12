/*
Copyright 2019 The Kubernetes Authors.

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

package util

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigOwner provides a data interface for different config owner types.
type ConfigOwner struct {
	*unstructured.Unstructured
}

// IsInfrastructureReady extracts infrastructure status from the config owner.
func (co ConfigOwner) IsInfrastructureReady() bool {
	infrastructureReady, _, err := unstructured.NestedBool(co.Object, "status", "infrastructureReady")
	if err != nil {
		return false
	}
	return infrastructureReady
}

// ClusterName extracts spec.clusterName from the config owner.
func (co ConfigOwner) ClusterName() string {
	clusterName, _, err := unstructured.NestedString(co.Object, "spec", "clusterName")
	if err != nil {
		return ""
	}
	return clusterName
}

// DataSecretName extracts spec.bootstrap.dataSecretName from the config owner.
func (co ConfigOwner) DataSecretName() *string {
	dataSecretName, exist, err := unstructured.NestedString(co.Object, "spec", "bootstrap", "dataSecretName")
	if err != nil || !exist {
		return nil
	}
	return &dataSecretName
}

// IsControlPlaneMachine checks if an unstructured object is Machine with the control plane role.
func (co ConfigOwner) IsControlPlaneMachine() bool {
	if co.GetKind() != "Machine" {
		return false
	}
	labels := co.GetLabels()
	if labels == nil {
		return false
	}
	_, ok := labels[clusterv1.MachineControlPlaneLabelName]
	return ok
}

// GeConfigOwner returns the Unstructured object owning the current resource.
func GetConfigOwner(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*ConfigOwner, error) {
	for _, ref := range obj.OwnerReferences {
		if ref.Kind == "Machine" && ref.APIVersion == clusterv1.GroupVersion.String() {
			return GetOwnerByRef(ctx, c, &corev1.ObjectReference{
				APIVersion: ref.APIVersion,
				Kind:       ref.Kind,
				Namespace:  obj.Namespace,
				Name:       ref.Name,
			})
		}
	}
	return nil, nil
}

// GetOwnerByRef finds and returns the owner by looking at the object reference.
func GetOwnerByRef(ctx context.Context, c client.Client, ref *corev1.ObjectReference) (*ConfigOwner, error) {
	obj, err := external.Get(ctx, c, ref, ref.Namespace)
	if err != nil {
		return nil, err
	}
	return &ConfigOwner{obj}, nil
}
