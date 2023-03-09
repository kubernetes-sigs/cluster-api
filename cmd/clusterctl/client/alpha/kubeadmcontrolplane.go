/*
Copyright 2022 The Kubernetes Authors.

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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
)

// getKubeadmControlPlane retrieves the KubeadmControlPlane object corresponding to the name and namespace specified.
func getKubeadmControlPlane(proxy cluster.Proxy, name, namespace string) (*controlplanev1.KubeadmControlPlane, error) {
	kcpObj := &controlplanev1.KubeadmControlPlane{}
	c, err := proxy.NewClient()
	if err != nil {
		return nil, err
	}
	kcpObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.Get(ctx, kcpObjKey, kcpObj); err != nil {
		return nil, errors.Wrapf(err, "failed to get KubeadmControlPlane %s/%s",
			kcpObjKey.Namespace, kcpObjKey.Name)
	}
	return kcpObj, nil
}

// setRolloutAfterOnKCP sets KubeadmControlPlane.spec.rolloutAfter.
func setRolloutAfterOnKCP(proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"rolloutAfter":"%v"}}`, time.Now().Format(time.RFC3339))))
	return patchKubeadmControlPlane(proxy, name, namespace, patch)
}

// patchKubeadmControlPlane applies a patch to a KubeadmControlPlane.
func patchKubeadmControlPlane(proxy cluster.Proxy, name, namespace string, patch client.Patch) error {
	cFrom, err := proxy.NewClient()
	if err != nil {
		return err
	}
	kcpObj := &controlplanev1.KubeadmControlPlane{}
	kcpObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := cFrom.Get(ctx, kcpObjKey, kcpObj); err != nil {
		return errors.Wrapf(err, "failed to get KubeadmControlPlane %s/%s", kcpObj.GetNamespace(), kcpObj.GetName())
	}

	if err := cFrom.Patch(ctx, kcpObj, patch); err != nil {
		return errors.Wrapf(err, "failed while patching KubeadmControlPlane %s/%s", kcpObj.GetNamespace(), kcpObj.GetName())
	}
	return nil
}
