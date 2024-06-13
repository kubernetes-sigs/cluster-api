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
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// ObjectRestarter will issue a restart on the specified cluster-api resource.
func (r *rollout) ObjectRestarter(ctx context.Context, proxy cluster.Proxy, ref corev1.ObjectReference) error {
	switch ref.Kind {
	case MachineDeployment:
		deployment, err := getMachineDeployment(ctx, proxy, ref.Name, ref.Namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to fetch resource %v/%v", ref.Kind, ref.Name)
		}
		if deployment.Spec.Paused {
			return errors.Errorf("can't restart paused MachineDeployment (run rollout resume first): %v/%v", ref.Kind, ref.Name)
		}
		if deployment.Spec.RolloutAfter != nil && deployment.Spec.RolloutAfter.After(time.Now()) {
			return errors.Errorf("can't update MachineDeployment (remove 'spec.rolloutAfter' first): %v/%v", ref.Kind, ref.Name)
		}
		if err := setRolloutAfterOnMachineDeployment(ctx, proxy, ref.Name, ref.Namespace); err != nil {
			return err
		}
	default:
		_, err := resourceHasRolloutAfter(proxy, ref)
		if err != nil {
			return errors.Errorf("Invalid resource type %v. Resource must implement rolloutAfter in it's spec", ref.Kind)
		}
		obj, err := getUnstructuredControlPlane(ctx, proxy, ref)
		if err != nil || obj == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", ref.Kind, ref.Name)
		}

		annotations := obj.GetAnnotations()
		if paused, ok := annotations["cluster.x-k8s.io/paused"]; ok && paused == "true" {
			return errors.Errorf("can't perform operations on paused resource (remove annotation 'cluster.x-k8s.io/paused' first): %v/%v", obj.GetKind(), obj.GetName())
		}

		if err := checkControlPlaneRolloutAfter(obj); err != nil {
			return errors.Errorf("err: %s, can't update ControlPlane (remove 'spec.rolloutAfter' first): %v/%v", err.Error(), ref.Kind, ref.Name)
		}

		if err := setRolloutAfterOnControlPlane(ctx, proxy, ref); err != nil {
			return err
		}
	}
	return nil
}
