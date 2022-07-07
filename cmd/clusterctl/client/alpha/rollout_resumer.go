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
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/util/annotations"
)

// ObjectResumer will issue a resume on the specified cluster-api resource.
func (r *rollout) ObjectResumer(proxy cluster.Proxy, ref corev1.ObjectReference) error {
	switch ref.Kind {
	case MachineDeployment:
		deployment, err := getMachineDeployment(proxy, ref.Name, ref.Namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", ref.Kind, ref.Name)
		}
		if !deployment.Spec.Paused {
			return errors.Errorf("MachineDeployment is not currently paused: %v/%v\n", ref.Kind, ref.Name) //nolint:revive // MachineDeployment is intentionally capitalized.
		}
		if err := resumeMachineDeployment(proxy, ref.Name, ref.Namespace); err != nil {
			return err
		}
	case KubeadmControlPlane:
		kcp, err := getKubeadmControlPlane(proxy, ref.Name, ref.Namespace)
		if err != nil || kcp == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", ref.Kind, ref.Name)
		}
		if !annotations.HasPaused(kcp.GetObjectMeta()) {
			return errors.Errorf("KubeadmControlPlane is not currently paused: %v/%v\n", ref.Kind, ref.Name) //nolint:revive // KubeadmControlPlane is intentionally capitalized.
		}
		if err := resumeKubeadmControlPlane(proxy, ref.Name, ref.Namespace); err != nil {
			return err
		}
	default:
		return errors.Errorf("invalid resource type %q, valid values are %v", ref.Kind, validResourceTypes)
	}
	return nil
}

// resumeMachineDeployment sets Paused to true in the MachineDeployment's spec.
func resumeMachineDeployment(proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%t}}", false)))

	return patchMachineDeployment(proxy, name, namespace, patch)
}

// resumeKubeadmControlPlane removes paused annotation.
func resumeKubeadmControlPlane(proxy cluster.Proxy, name, namespace string) error {
	// In the paused annotation we must replace slashes to ~1, see https://datatracker.ietf.org/doc/html/rfc6901#section-3.
	pausedAnnotation := strings.Replace(clusterv1.PausedAnnotation, "/", "~1", -1)
	patch := client.RawPatch(types.JSONPatchType, []byte(fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/metadata/annotations/%s\"}]", pausedAnnotation)))

	return patchKubeadmControlPlane(proxy, name, namespace, patch)
}
