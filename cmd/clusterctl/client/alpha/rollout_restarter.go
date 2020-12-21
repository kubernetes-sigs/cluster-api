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
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectRestarter will issue a restart on the specified cluster-api resource.
func (r *rollout) ObjectRestarter(proxy cluster.Proxy, tuple util.ResourceTuple, namespace string) error {
	switch tuple.Resource {
	case "machinedeployment":
		deployment, err := getMachineDeployment(proxy, tuple.Name, namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to fetch %v/%v", tuple.Resource, tuple.Name)
		}
		if deployment.Spec.Paused {
			return errors.Errorf("can't restart paused machinedeployment (run rollout resume first): %v/%v\n", tuple.Resource, tuple.Name)
		}
		if err := setRestartedAtAnnotation(proxy, tuple.Name, namespace); err != nil {
			return err
		}
	default:
		return errors.Errorf("Invalid resource type %v. Valid values: %v", tuple.Resource, validResourceTypes)
	}
	return nil
}

// getMachineDeployment retrieves the MachineDeployment object corresponding to the name and namespace specified.
func getMachineDeployment(proxy cluster.Proxy, name, namespace string) (*clusterv1.MachineDeployment, error) {
	mdObj := &clusterv1.MachineDeployment{}
	c, err := proxy.NewClient()
	if err != nil {
		return nil, err
	}
	mdObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.Get(context.TODO(), mdObjKey, mdObj); err != nil {
		return nil, errors.Wrapf(err, "error reading %q %s/%s",
			mdObj.GroupVersionKind(), mdObjKey.Namespace, mdObjKey.Name)
	}
	return mdObj, nil
}

// setRestartedAtAnnotation sets the restartedAt annotation in the MachineDeployment's spec.template.objectmeta.
func setRestartedAtAnnotation(proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"cluster.x-k8s.io/restartedAt\":\"%v\"}}}}}", time.Now().Format(time.RFC3339))))
	return patchMachineDeployemt(proxy, name, namespace, patch)
}

// patchMachineDeployemt applies a patch to a machinedeployment
func patchMachineDeployemt(proxy cluster.Proxy, name, namespace string, patch client.Patch) error {
	cFrom, err := proxy.NewClient()
	if err != nil {
		return err
	}
	mdObj := &clusterv1.MachineDeployment{}
	mdObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := cFrom.Get(context.TODO(), mdObjKey, mdObj); err != nil {
		return errors.Wrapf(err, "error reading %s/%s", mdObj.GetNamespace(), mdObj.GetName())
	}

	if err := cFrom.Patch(context.TODO(), mdObj, patch); err != nil {
		return errors.Wrapf(err, "error while patching %s/%s", mdObj.GetNamespace(), mdObj.GetName())
	}
	return nil
}
