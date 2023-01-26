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
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/util/annotations"
)

// ObjectPauser will issue a pause on the specified cluster-api resource.
func (r *rollout) ObjectPauser(proxy cluster.Proxy, ref corev1.ObjectReference) error {
	switch ref.Kind {
	case MachineDeployment:
		deployment, err := getMachineDeployment(proxy, ref.Name, ref.Namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", ref.Kind, ref.Name)
		}
		if deployment.Spec.Paused {
			return errors.Errorf("MachineDeployment is already paused: %v/%v\n", ref.Kind, ref.Name) //nolint:revive // MachineDeployment is intentionally capitalized.
		}
		if err := pauseMachineDeployment(proxy, ref.Name, ref.Namespace); err != nil {
			return err
		}
	case KubeadmControlPlane:
		kcp, err := getKubeadmControlPlane(proxy, ref.Name, ref.Namespace)
		if err != nil || kcp == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", ref.Kind, ref.Name)
		}
		if annotations.HasPaused(kcp.GetObjectMeta()) {
			return errors.Errorf("KubeadmControlPlane is already paused: %v/%v\n", ref.Kind, ref.Name) //nolint:revive // KubeadmControlPlane is intentionally capitalized.
		}
		if err := pauseKubeadmControlPlane(proxy, ref.Name, ref.Namespace); err != nil {
			return err
		}
	default:
		return errors.Errorf("Invalid resource type %q, valid values are %v", ref.Kind, validResourceTypes)
	}
	return nil
}

// pauseMachineDeployment sets Paused to true in the MachineDeployment's spec.
func pauseMachineDeployment(proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%t}}", true)))
	return patchMachineDeployment(proxy, name, namespace, patch)
}

// pauseKubeadmControlPlane sets paused annotation to true.
func pauseKubeadmControlPlane(proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{%q: \"%t\"}}}", clusterv1.PausedAnnotation, true)))
	return patchKubeadmControlPlane(proxy, name, namespace, patch)
}
