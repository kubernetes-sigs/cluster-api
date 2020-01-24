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
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DeleteOptions struct {
	Provider             clusterctlv1.Provider
	ForceDeleteNamespace bool
	ForceDeleteCRD       bool
}

// ComponentsClient has methods to work with provider components in the cluster.
type ComponentsClient interface {
	// Create creates the provider components in the management cluster.
	Create(components repository.Components) error

	// Delete deletes the provider components from the management cluster.
	// The operation is designed to prevent accidental deletion of user created objects, so
	// it is required to explicitly opt-in for the deletion of the namespace where the provider components are hosted
	// and for the deletion of the provider's CRDs.
	Delete(options DeleteOptions) error
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
	for i := range resources {
		obj := resources[i]

		// check if the component already exists, and eventually update it
		currentR := &unstructured.Unstructured{}
		currentR.SetGroupVersionKind(obj.GroupVersionKind())

		key := client.ObjectKey{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}
		if err := c.Get(ctx, key, currentR); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to get current provider object")
			}

			//if it does not exists, create the component
			klog.V(3).Infof("Creating: %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
			if err := c.Create(ctx, &obj); err != nil {
				return errors.Wrapf(err, "failed to create provider object %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
			}

			continue
		}

		// otherwise update the component
		klog.V(3).Infof("Updating: %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())

		// if upgrading an existing component, then use the current resourceVersion for the optimistic lock
		obj.SetResourceVersion(currentR.GetResourceVersion())
		if err := c.Update(ctx, &obj); err != nil {
			return errors.Wrapf(err, "failed to update provider object %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
		}
	}

	return nil
}

func (p *providerComponents) Delete(options DeleteOptions) error {
	// Fetch all the components belonging to a provider.
	labels := map[string]string{
		clusterctlv1.ClusterctlProviderLabelName: options.Provider.Name,
	}
	resources, err := p.proxy.ListResources(options.Provider.Namespace, labels)
	if err != nil {
		return err
	}

	resourcesToDelete := []unstructured.Unstructured{}
	namespacesToDelete := sets.NewString()
	for _, obj := range resources {
		// If the CRDs should NOT be deleted, skip it;
		// NB. Skipping CRDs deletion ensures that also the objects of Kind defined in the CRDs Kind are not deleted.
		if obj.GroupVersionKind().Kind == "CustomResourceDefinition" && !options.ForceDeleteCRD {
			continue
		}

		// If the Namespace should NOT be deleted, skip it, otherwise keep track of the namespaces we are deleting;
		// NB. Skipping Namespaces deletion ensures that also the objects hosted in the namespace but without the "clusterctl.cluster.x-k8s.io/provider" label are not deleted.
		if obj.GroupVersionKind().Kind == "Namespace" {
			if !options.ForceDeleteNamespace {
				continue
			}
			namespacesToDelete.Insert(obj.GetName())
		}

		resourcesToDelete = append(resourcesToDelete, obj)
	}

	// Delete all the provider components.
	cs, err := p.proxy.NewClient()
	if err != nil {
		return err
	}

	errList := []error{}
	for i := range resourcesToDelete {
		obj := resourcesToDelete[i]

		// if the objects is in a namespace that is going to be deleted, skip deletion
		// because everything that is contained in the namespace will be deleted by the Namespace controller
		if namespacesToDelete.Has(obj.GetNamespace()) {
			continue
		}

		// Otherwise delete the object
		if err := cs.Delete(ctx, &obj); err != nil {
			if apierrors.IsNotFound(err) {
				// Tolerate IsNotFound error that might happen because we are not enforcing a deletion order
				// that considers relation across objects (e.g. Deployments -> ReplicaSets -> Pods)
				continue
			}
			errList = append(errList, errors.Wrapf(err, "Error deleting object %s, %s/%s",
				obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName()))
		}
	}

	return kerrors.NewAggregate(errList)
}

// newComponentsClient returns a providerComponents.
func newComponentsClient(proxy Proxy) *providerComponents {
	return &providerComponents{
		proxy: proxy,
	}
}

var defaultCreatePriorities = []string{
	// Namespaces go first because all namespaced resources depend on them.
	"Namespace",
	// Custom Resource Definitions come before Custom Resource so that they can be
	// restored with their corresponding CRD.
	"CustomResourceDefinition",
	// Storage Classes are needed to create PVs and PVCs correctly.
	"StorageClass",
	// PVs go before PVCs because PVCs depend on them.
	"PersistentVolume",
	// PVCs go before pods or controllers so they can be mounted as volumes.
	"PersistentVolumeClaim",
	// Secrets and ConfigMaps go before pods or controllers so they can be mounted as volumes.
	"Secret",
	"ConfigMap",
	// Service accounts go before pods or controllers so pods can use them.
	"ServiceAccount",
	// Limit ranges go before pods or controllers so pods can use them.
	"LimitRange",
	// Pods go before ReplicaSets
	"Pods",
	// ReplicaSets go before Deployments
	"ReplicaSet",
	// Endpoints go before Services
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
