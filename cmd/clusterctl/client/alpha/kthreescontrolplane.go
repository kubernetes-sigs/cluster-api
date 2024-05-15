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
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kthreescontrolplanev1 "github.com/k3s-io/cluster-api-k3s/controlplane/api/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// getKThreesControlPlane retrieves the KThreesControlPlane object corresponding to the name and namespace specified.
func getKThreesControlPlane(ctx context.Context, proxy cluster.Proxy, name, namespace string) (*kthreescontrolplanev1.KThreesControlPlane, error) {
	kcpObj := &kthreescontrolplanev1.KThreesControlPlane{}
	c, err := proxy.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	kcpObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.Get(ctx, kcpObjKey, kcpObj); err != nil {
		return nil, errors.Wrapf(err, "failed to get KThreesControlPlane %s/%s",
			kcpObjKey.Namespace, kcpObjKey.Name)
	}
	return kcpObj, nil
}

// setUpgradeAfterOnKThreesControlPlane sets KThreesControlPlane.spec.rolloutAfter.
func setUpgradeAfterOnKThreesControlPlane(ctx context.Context, proxy cluster.Proxy, name, namespace string) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"upgradeAfter":"%v"}}`, time.Now().Format(time.RFC3339))))
	return patchKThreesControlPlane(ctx, proxy, name, namespace, patch)
}

// patchKThreesControlPlane applies a patch to a KThreesControlPlane.
func patchKThreesControlPlane(ctx context.Context, proxy cluster.Proxy, name, namespace string, patch client.Patch) error {
	cFrom, err := proxy.NewClient(ctx)
	if err != nil {
		return err
	}
	kcpObj := &kthreescontrolplanev1.KThreesControlPlane{}
	kcpObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := cFrom.Get(ctx, kcpObjKey, kcpObj); err != nil {
		return errors.Wrapf(err, "failed to get KThreesControlPlane %s/%s", kcpObj.GetNamespace(), kcpObj.GetName())
	}

	if err := cFrom.Patch(ctx, kcpObj, patch); err != nil {
		return errors.Wrapf(err, "failed while patching KThreesControlPlane %s/%s", kcpObj.GetNamespace(), kcpObj.GetName())
	}
	return nil
}
