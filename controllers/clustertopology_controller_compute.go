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
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/external"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// clusterTopologyClass holds all the objects required for computing the desired state of a managed Cluster topology.
type clusterTopologyClass struct {
	clusterClass                  *clusterv1.ClusterClass
	infrastructureClusterTemplate *unstructured.Unstructured
	controlPlane                  controlPlaneTopologyClass
	machineDeployments            map[string]machineDeploymentTopologyClass
}

// controlPlaneTopologyClass holds the templates required for computing the desired state of a managed control plane.
type controlPlaneTopologyClass struct {
	template                      *unstructured.Unstructured
	infrastructureMachineTemplate *unstructured.Unstructured
}

// machineDeploymentTopologyClass holds the templates required for computing the desired state of a managed deployment.
type machineDeploymentTopologyClass struct {
	bootstrapTemplate             *unstructured.Unstructured
	infrastructureMachineTemplate *unstructured.Unstructured
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

// getClass gets the ClusterClass and the referenced templates to be used for a managed Cluster topology. It also converts
// and patches all ObjectReferences in ClusterClass and ControlPlane to the latest apiVersion of the current contract.
// NOTE: This function assumes that cluster.Spec.Topology.Class is set.
func (r *ClusterTopologyReconciler) getClass(ctx context.Context, cluster *clusterv1.Cluster) (_ *clusterTopologyClass, reterr error) {
	class := &clusterTopologyClass{
		clusterClass:       &clusterv1.ClusterClass{},
		machineDeployments: map[string]machineDeploymentTopologyClass{},
	}

	// Get ClusterClass.
	key := client.ObjectKey{Name: cluster.Spec.Topology.Class, Namespace: cluster.Namespace}
	if err := r.Client.Get(ctx, key, class.clusterClass); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ClusterClass %q in namespace %q", cluster.Spec.Topology.Class, cluster.Namespace)
	}

	// We use the patchHelper to patch potential changes to the ObjectReferences in ClusterClass.
	patchHelper, err := patch.NewHelper(class.clusterClass, r.Client)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := patchHelper.Patch(ctx, class.clusterClass); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, errors.Wrapf(err, "failed to patch ClusterClass %q in namespace %q", class.clusterClass.Name, class.clusterClass.Namespace)})
		}
	}()

	// Get ClusterClass.spec.infrastructure.
	class.infrastructureClusterTemplate, err = r.getTemplate(ctx, class.clusterClass.Spec.Infrastructure.Ref)
	if err != nil {
		return nil, err
	}

	// Get ClusterClass.spec.controlPlane.
	class.controlPlane.template, err = r.getTemplate(ctx, class.clusterClass.Spec.ControlPlane.Ref)
	if err != nil {
		return nil, err
	}

	// Check if ClusterClass.spec.ControlPlane.MachineInfrastructure is set, as it's optional.
	if class.clusterClass.Spec.ControlPlane.MachineInfrastructure != nil && class.clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref != nil {
		// Get ClusterClass.spec.controlPlane.machineInfrastructure.
		class.controlPlane.infrastructureMachineTemplate, err = r.getTemplate(ctx, class.clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref)
		if err != nil {
			return nil, err
		}
	}

	for _, mdc := range class.clusterClass.Spec.Workers.MachineDeployments {
		mdTopologyClass := machineDeploymentTopologyClass{}

		mdTopologyClass.infrastructureMachineTemplate, err = r.getTemplate(ctx, mdc.Template.Infrastructure.Ref)
		if err != nil {
			return nil, err
		}

		mdTopologyClass.bootstrapTemplate, err = r.getTemplate(ctx, mdc.Template.Bootstrap.Ref)
		if err != nil {
			return nil, err
		}

		class.machineDeployments[mdc.Class] = mdTopologyClass
	}

	return class, nil
}

// getTemplate gets the object referenced in ref.
// If necessary, it updates the ref to the latest apiVersion of the current contract.
func (r *ClusterTopologyReconciler) getTemplate(ctx context.Context, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	if ref == nil {
		return nil, errors.New("reference is not set")
	}
	if err := utilconversion.ConvertReferenceAPIContract(ctx, r.Client, r.restConfig, ref); err != nil {
		return nil, err
	}

	obj, err := external.Get(ctx, r.UnstructuredCachingClient, ref, ref.Namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s %q in namespace %q", ref.Kind, ref.Name, ref.Namespace)
	}
	return obj, nil
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
