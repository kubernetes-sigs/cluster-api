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

package cluster

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/cluster-api/controllers/external"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// bootstrapTemplateNamePrefix calculates the name prefix for a BootstrapTemplate.
func bootstrapTemplateNamePrefix(clusterName, machineDeploymentTopologyName string) string {
	return fmt.Sprintf("%s-%s-bootstrap-", clusterName, machineDeploymentTopologyName)
}

// infrastructureMachineTemplateNamePrefix calculates the name prefix for a InfrastructureMachineTemplate.
func infrastructureMachineTemplateNamePrefix(clusterName, machineDeploymentTopologyName string) string {
	return fmt.Sprintf("%s-%s-infra-", clusterName, machineDeploymentTopologyName)
}

// infrastructureMachineTemplateNamePrefix calculates the name prefix for a InfrastructureMachineTemplate.
func controlPlaneInfrastructureMachineTemplateNamePrefix(clusterName string) string {
	return fmt.Sprintf("%s-control-plane-", clusterName)
}

// getReference gets the object referenced in ref.
// If necessary, it updates the ref to the latest apiVersion of the current contract.
func (r *Reconciler) getReference(ctx context.Context, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	if ref == nil {
		return nil, errors.New("reference is not set")
	}
	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, r.APIReader, ref); err != nil {
		return nil, err
	}

	obj, err := external.Get(ctx, r.UnstructuredCachingClient, ref, ref.Namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s %q in namespace %q", ref.Kind, ref.Name, ref.Namespace)
	}
	return obj, nil
}

// refToUnstructured returns an unstructured object with details from an ObjectReference.
func refToUnstructured(ref *corev1.ObjectReference) *unstructured.Unstructured {
	uns := &unstructured.Unstructured{}
	uns.SetAPIVersion(ref.APIVersion)
	uns.SetKind(ref.Kind)
	uns.SetNamespace(ref.Namespace)
	uns.SetName(ref.Name)
	return uns
}
