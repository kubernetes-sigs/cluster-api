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

package alpha

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// getMachineDeployment retrieves the MachineDeployment object corresponding to the name and namespace specified.
func getMachineDeployment(ctx context.Context, proxy cluster.Proxy, name, namespace string) (*clusterv1.MachineDeployment, error) {
	mdObj := &clusterv1.MachineDeployment{}
	c, err := proxy.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	mdObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.Get(ctx, mdObjKey, mdObj); err != nil {
		return nil, errors.Wrapf(err, "failed to get MachineDeployment %s/%s",
			mdObjKey.Namespace, mdObjKey.Name)
	}
	return mdObj, nil
}

// setRolloutAfterOnMachineDeployment sets MachineDeployment.spec.rolloutAfter.
func setRolloutAfterOnMachineDeployment(ctx context.Context, proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"rollout":{"after":"%v"}}}`, time.Now().Format(time.RFC3339))))
	return patchMachineDeployment(ctx, proxy, name, namespace, patch)
}

// patchMachineDeployment applies a patch to a machinedeployment.
func patchMachineDeployment(ctx context.Context, proxy cluster.Proxy, name, namespace string, patch client.Patch) error {
	cFrom, err := proxy.NewClient(ctx)
	if err != nil {
		return err
	}
	mdObj := &clusterv1.MachineDeployment{}
	mdObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := cFrom.Get(ctx, mdObjKey, mdObj); err != nil {
		return errors.Wrapf(err, "failed to get MachineDeployment %s/%s", mdObj.GetNamespace(), mdObj.GetName())
	}

	if err := cFrom.Patch(ctx, mdObj, patch); err != nil {
		return errors.Wrapf(err, "failed while patching MachineDeployment %s/%s", mdObj.GetNamespace(), mdObj.GetName())
	}
	return nil
}
