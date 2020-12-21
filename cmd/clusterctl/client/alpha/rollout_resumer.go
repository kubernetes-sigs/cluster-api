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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectResumer will issue a resume on the specified cluster-api resource.
func (r *rollout) ObjectResumer(proxy cluster.Proxy, tuple util.ResourceTuple, namespace string) error {
	switch tuple.Resource {
	case "machinedeployment":
		deployment, err := getMachineDeployment(proxy, tuple.Name, namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", tuple.Resource, tuple.Name)
		}
		if !deployment.Spec.Paused {
			return errors.Errorf("MachineDeployment is not currently paused: %v/%v\n", tuple.Resource, tuple.Name)
		}
		if err := resumeMachineDeployment(proxy, tuple.Name, namespace); err != nil {
			return err
		}
	default:
		return errors.Errorf("Invalid resource type %q, valid values are %v", tuple.Resource, validResourceTypes)
	}
	return nil
}

// resumeMachineDeployment sets Paused to true in the MachineDeployment's spec.
func resumeMachineDeployment(proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%t}}", false)))

	return patchMachineDeployemt(proxy, name, namespace, patch)
}
