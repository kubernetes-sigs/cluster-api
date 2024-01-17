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

// Package util implements kubeadm utility functionality.
package util

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
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

// HasNodeRefs checks if the config owner has nodeRefs. For a Machine this means
// that it has a nodeRef. For a MachinePool it means that it has as many nodeRefs
// as there are replicas.
func (co ConfigOwner) HasNodeRefs() bool {
	if co.IsMachinePool() {
		numExpectedNodes, found, err := unstructured.NestedInt64(co.Object, "spec", "replicas")
		if err != nil {
			return false
		}
		// replicas default to 1 so this is what we should use if nothing is specified
		if !found {
			numExpectedNodes = 1
		}
		nodeRefs, _, err := unstructured.NestedSlice(co.Object, "status", "nodeRefs")
		if err != nil {
			return false
		}
		return len(nodeRefs) == int(numExpectedNodes)
	}
	nodeRef, _, err := unstructured.NestedMap(co.Object, "status", "nodeRef")
	if err != nil {
		return false
	}
	return len(nodeRef) > 0
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
	_, ok := labels[clusterv1.MachineControlPlaneLabel]
	return ok
}

// IsMachinePool checks if an unstructured object is a MachinePool.
func (co ConfigOwner) IsMachinePool() bool {
	return co.GetKind() == "MachinePool"
}

// KubernetesVersion returns the Kuberentes version for the config owner object.
func (co ConfigOwner) KubernetesVersion() string {
	fields := []string{"spec", "version"}
	if co.IsMachinePool() {
		fields = []string{"spec", "template", "spec", "version"}
	}

	version, _, err := unstructured.NestedString(co.Object, fields...)
	if err != nil {
		return ""
	}
	return version
}

// GetConfigOwner returns the Unstructured object owning the current resource
// using the uncached unstructured client. For performance-sensitive uses,
// consider GetTypedConfigOwner.
func GetConfigOwner(ctx context.Context, c client.Client, obj metav1.Object) (*ConfigOwner, error) {
	return getConfigOwner(ctx, c, obj, GetOwnerByRef)
}

// GetTypedConfigOwner returns the Unstructured object owning the current
// resource. The implementation ensures a typed client is used, so the objects are read from the cache.
func GetTypedConfigOwner(ctx context.Context, c client.Client, obj metav1.Object) (*ConfigOwner, error) {
	return getConfigOwner(ctx, c, obj, GetTypedOwnerByRef)
}

func getConfigOwner(ctx context.Context, c client.Client, obj metav1.Object, getFn func(context.Context, client.Client, *corev1.ObjectReference) (*ConfigOwner, error)) (*ConfigOwner, error) {
	allowedGKs := []schema.GroupKind{
		{
			Group: clusterv1.GroupVersion.Group,
			Kind:  "Machine",
		},
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		allowedGKs = append(allowedGKs, schema.GroupKind{
			Group: expv1.GroupVersion.Group,
			Kind:  "MachinePool",
		})
	}

	for _, ref := range obj.GetOwnerReferences() {
		refGV, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse GroupVersion from %q", ref.APIVersion)
		}
		refGVK := refGV.WithKind(ref.Kind)

		for _, gk := range allowedGKs {
			if refGVK.Group == gk.Group && refGVK.Kind == gk.Kind {
				return getFn(ctx, c, &corev1.ObjectReference{
					APIVersion: ref.APIVersion,
					Kind:       ref.Kind,
					Name:       ref.Name,
					Namespace:  obj.GetNamespace(),
				})
			}
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

// GetTypedOwnerByRef finds and returns the owner by looking at the object
// reference. The implementation ensures a typed client is used, so the objects are read from the cache.
func GetTypedOwnerByRef(ctx context.Context, c client.Client, ref *corev1.ObjectReference) (*ConfigOwner, error) {
	objGVK := ref.GroupVersionKind()
	obj, err := c.Scheme().New(objGVK)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to construct object of type %s", ref.GroupVersionKind())
	}
	clientObj, ok := obj.(client.Object)
	if !ok {
		return nil, errors.Errorf("expected owner reference to refer to a client.Object, is actually %T", obj)
	}
	key := types.NamespacedName{
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}
	err = c.Get(ctx, key, clientObj)
	if err != nil {
		return nil, err
	}

	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(clientObj)
	if err != nil {
		return nil, err
	}
	u := unstructured.Unstructured{}
	u.SetUnstructuredContent(content)
	u.SetGroupVersionKind(objGVK)

	return &ConfigOwner{&u}, nil
}
