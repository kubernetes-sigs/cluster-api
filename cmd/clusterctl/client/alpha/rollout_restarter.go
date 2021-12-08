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
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// ObjectRestarter will issue a restart on the specified cluster-api resource.
func (r *rollout) ObjectRestarter(proxy cluster.Proxy, ref corev1.ObjectReference) error {
	switch ref.Kind {
	case MachineDeployment:
		deployment, err := getMachineDeployment(proxy, ref.Name, ref.Namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", ref.Kind, ref.Name)
		}
		if deployment.Spec.Paused {
			return errors.Errorf("can't restart paused machinedeployment (run rollout resume first): %v/%v\n", ref.Kind, ref.Name)
		}
		if err := setRestartedAtAnnotation(proxy, ref.Name, ref.Namespace); err != nil {
			return err
		}
	default:
		return errors.Errorf("Invalid resource type %v. Valid values: %v", ref.Kind, validResourceTypes)
	}
	return nil
}

// setRestartedAtAnnotation sets the restartedAt annotation in the MachineDeployment's spec.template.objectmeta.
func setRestartedAtAnnotation(proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"cluster.x-k8s.io/restartedAt\":\"%v\"}}}}}", time.Now().Format(time.RFC3339))))
	return patchMachineDeployemt(proxy, name, namespace, patch)
}
