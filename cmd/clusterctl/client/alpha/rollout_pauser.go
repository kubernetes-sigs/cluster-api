/*
Copyright 2020 The Kubernetes Authors.

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

package alpha

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// ObjectPauser will issue a pause on the specified cluster-api resource.
func (r *rollout) ObjectPauser(ctx context.Context, proxy cluster.Proxy, ref corev1.ObjectReference) error {
	switch ref.Kind {
	case MachineDeployment:
		deployment, err := getMachineDeployment(ctx, proxy, ref.Name, ref.Namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", ref.Kind, ref.Name)
		}
		if deployment.Spec.Paused {
			return errors.Errorf("MachineDeployment is already paused: %v/%v\n", ref.Kind, ref.Name) //nolint:revive // MachineDeployment is intentionally capitalized.
		}
		if err := pauseMachineDeployment(ctx, proxy, ref.Name, ref.Namespace); err != nil {
			return err
		}
	default:
		obj, err := getUnstructuredControlPlane(ctx, proxy, ref)
		if err != nil || obj == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", ref.Kind, ref.Name)
		}

		annotations := obj.GetAnnotations()
		if paused, ok := annotations["cluster.x-k8s.io/paused"]; ok && paused == "true" {
			return errors.Errorf("can't perform operations on paused resource (remove annotation 'cluster.x-k8s.io/paused' first): %v/%v", obj.GetKind(), obj.GetName())
		}
		if err := pauseControlPlane(ctx, proxy, ref); err != nil {
			return err
		}
	}
	return nil
}

// pauseMachineDeployment sets Paused to true in the MachineDeployment's spec.
func pauseMachineDeployment(ctx context.Context, proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%t}}", true)))
	return patchMachineDeployment(ctx, proxy, name, namespace, patch)
}

// pauseControlPlane sets paused annotation to true.
func pauseControlPlane(ctx context.Context, proxy cluster.Proxy, ref corev1.ObjectReference) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{%q: \"%t\"}}}", clusterv1.PausedAnnotation, true)))
	return patchControlPlane(ctx, proxy, ref, patch)
}
