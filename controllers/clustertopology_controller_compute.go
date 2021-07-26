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

package controllers

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// clusterTopologyClass holds all the objects required for computing the desired state of a managed Cluster topology.
type clusterTopologyClass struct {
	clusterClass                  *clusterv1.ClusterClass                   //nolint:structcheck
	infrastructureClusterTemplate *unstructured.Unstructured                //nolint:structcheck
	controlPlane                  controlPlaneTopologyClass                 //nolint:structcheck
	machineDeployments            map[string]machineDeploymentTopologyClass //nolint:structcheck
}

// controlPlaneTopologyClass holds the templates required for computing the desired state of a managed control plane.
type controlPlaneTopologyClass struct {
	template                      *unstructured.Unstructured //nolint:structcheck
	infrastructureMachineTemplate *unstructured.Unstructured //nolint:structcheck
}

// machineDeploymentTopologyClass holds the templates required for computing the desired state of a managed deployment.
type machineDeploymentTopologyClass struct {
	bootstrapTemplate             *unstructured.Unstructured //nolint:structcheck
	infrastructureMachineTemplate *unstructured.Unstructured //nolint:structcheck
}

// clusterTopologyState holds all the objects representing the state of a managed Cluster topology.
// NOTE: please note that we are going to deal with two different type state, the current state as read from the API server,
// and the desired state resulting from processing the clusterTopologyClass.
type clusterTopologyState struct {
	cluster               *clusterv1.Cluster               //nolint:structcheck
	infrastructureCluster *unstructured.Unstructured       //nolint:structcheck
	controlPlane          controlPlaneTopologyState        //nolint:structcheck
	machineDeployments    []machineDeploymentTopologyState //nolint:structcheck
}

// controlPlaneTopologyState all the objects representing the state of a managed control plane.
type controlPlaneTopologyState struct {
	object                        *unstructured.Unstructured //nolint:structcheck
	infrastructureMachineTemplate *unstructured.Unstructured //nolint:structcheck
}

// machineDeploymentTopologyState all the objects representing the state of a managed deployment.
type machineDeploymentTopologyState struct {
	object                        *clusterv1.MachineDeployment //nolint:structcheck
	bootstrapTemplate             *unstructured.Unstructured   //nolint:structcheck
	infrastructureMachineTemplate *unstructured.Unstructured   //nolint:structcheck
}

// Gets the ClusterClass and the referenced templates to be used for a managed Cluster topology.
func (r *ClusterTopologyReconciler) getClass(ctx context.Context, cluster *clusterv1.Cluster) (*clusterTopologyClass, error) {
	// TODO: add get class logic; also remove nolint exception from clusterTopologyClass and machineDeploymentTopologyClass
	return nil, nil
}

// Gets the current state of the Cluster topology.
func (r *ClusterTopologyReconciler) getCurrentState(ctx context.Context, cluster *clusterv1.Cluster) (*clusterTopologyState, error) {
	// TODO: add get class logic; also remove nolint exception from clusterTopologyState and machineDeploymentTopologyState
	return nil, nil
}

// Computes the desired state of the Cluster topology.
func (r *ClusterTopologyReconciler) computeDesiredState(ctx context.Context, input *clusterTopologyClass, current *clusterTopologyState) (*clusterTopologyState, error) {
	// TODO: add compute logic
	return nil, nil
}

// getNestedRef returns the ref value of a nested field.
func getNestedRef(obj *unstructured.Unstructured, fields ...string) (*corev1.ObjectReference, error) {
	ref := &corev1.ObjectReference{}
	if v, ok, err := unstructured.NestedString(obj.UnstructuredContent(), append(fields, "apiVersion")...); ok && err == nil {
		ref.APIVersion = v
	} else {
		return nil, errors.Errorf("failed to get reference apiVersion")
	}
	if v, ok, err := unstructured.NestedString(obj.UnstructuredContent(), append(fields, "kind")...); ok && err == nil {
		ref.Kind = v
	} else {
		return nil, errors.Errorf("failed to get reference Kind")
	}
	if v, ok, err := unstructured.NestedString(obj.UnstructuredContent(), append(fields, "name")...); ok && err == nil {
		ref.Name = v
	} else {
		return nil, errors.Errorf("failed to get reference name")
	}
	if v, ok, err := unstructured.NestedString(obj.UnstructuredContent(), append(fields, "namespace")...); ok && err == nil {
		ref.Namespace = v
	} else {
		return nil, errors.Errorf("failed to get reference namespace")
	}
	return ref, nil
}

// setNestedRef sets the value of a nested field to a reference to the refObj provided.
func setNestedRef(obj, refObj *unstructured.Unstructured, fields ...string) error {
	ref := map[string]interface{}{
		"kind":       refObj.GetKind(),
		"namespace":  refObj.GetNamespace(),
		"name":       refObj.GetName(),
		"apiVersion": refObj.GetAPIVersion(),
	}
	return unstructured.SetNestedField(obj.UnstructuredContent(), ref, fields...)
}

func objToRef(obj client.Object) *corev1.ObjectReference {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return &corev1.ObjectReference{
		Kind:       gvk.Kind,
		APIVersion: gvk.GroupVersion().String(),
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
	}
}
