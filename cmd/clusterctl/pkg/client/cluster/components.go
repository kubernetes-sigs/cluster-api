/*
Copyright 2019 The Kubernetes Authors.

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

package cluster

import (
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ComponentsClient has methods to work with provider components in the cluster.
type ComponentsClient interface {
	Create(components repository.Components) error

	//TODO: add more in a follow up PR
}

// providerComponents implements ComponentsClient.
type providerComponents struct {
	proxy Proxy
}

// Create provider components defined in the yaml file.
func (p *providerComponents) Create(components repository.Components) error {

	c, err := p.proxy.NewClient()
	if err != nil {
		return err
	}

	// sort provider components for creation according to relation across objects (e.g. Namespace before everything namespaced)
	resources := sortResourcesForCreate(components.Objs())

	// creates (or updates) provider components
	for _, r := range resources {

		// check if the component already exists, and eventually update it
		currentR := &unstructured.Unstructured{}
		currentR.SetGroupVersionKind(r.GroupVersionKind())

		key := client.ObjectKey{
			Namespace: r.GetNamespace(),
			Name:      r.GetName(),
		}
		if err = c.Get(ctx, key, currentR); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to get current provider object")
			}

			//if it does not exists, create the component
			klog.V(3).Infof("Creating: %s, %s/%s", r.GroupVersionKind(), r.GetNamespace(), r.GetName())
			if err = c.Create(ctx, &r); err != nil { //nolint
				return errors.Wrapf(err, "failed to create provider object")
			}

			continue
		}

		// otherwise update the component
		klog.V(3).Infof("Updating: %s, %s/%s", r.GroupVersionKind(), r.GetNamespace(), r.GetName())

		// if upgrading an existing component, then use the current resourceVersion for the optimistic lock
		r.SetResourceVersion(currentR.GetResourceVersion())
		if err = c.Update(ctx, &r); err != nil { //nolint
			return errors.Wrapf(err, "failed to update provider object")
		}
	}

	return nil
}

// newComponentsClient returns a providerComponents.
func newComponentsClient(proxy Proxy) *providerComponents {
	return &providerComponents{
		proxy: proxy,
	}
}

// - Namespaces go first because all namespaced resources depend on them.
// - Custom Resource Definitions come before Custom Resource so that they can be
//   restored with their corresponding CRD.
// - Storage Classes are needed to create PVs and PVCs correctly.
// - PVs go before PVCs because PVCs depend on them.
// - PVCs go before pods or controllers so they can be mounted as volumes.
// - Secrets and config maps go before pods or controllers so they can be mounted as volumes.
// - Service accounts go before pods or controllers so pods can use them.
// - Limit ranges go before pods or controllers so pods can use them.
// - Pods go before ReplicaSets
// - ReplicaSets go before Deployments
// - Endpoints go before Services
var defaultCreatePriorities = []string{
	"Namespace",
	"CustomResourceDefinition",
	"StorageClass",
	"PersistentVolume",
	"PersistentVolumeClaim",
	"Secret",
	"ConfigMap",
	"ServiceAccount",
	"LimitRange",
	"Pods",
	"ReplicaSet",
	"Endpoints",
}

func sortResourcesForCreate(resources []unstructured.Unstructured) []unstructured.Unstructured {
	var ret []unstructured.Unstructured

	// First get resources by priority
	for _, p := range defaultCreatePriorities {
		for _, o := range resources {
			if o.GetKind() == p {
				ret = append(ret, o)
			}
		}
	}

	// Then get all the other resources
	for _, o := range resources {
		found := false
		for _, r := range ret {
			if o.GroupVersionKind() == r.GroupVersionKind() && o.GetNamespace() == r.GetNamespace() && o.GetName() == r.GetName() {
				found = true
				break
			}
		}
		if !found {
			ret = append(ret, o)
		}
	}

	return ret
}
