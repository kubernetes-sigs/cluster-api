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
	"fmt"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DeleteOptions struct {
	Provider         clusterctlv1.Provider
	IncludeNamespace bool
	IncludeCRDs      bool
}

// ComponentsClient has methods to work with provider components in the cluster.
type ComponentsClient interface {
	// Create creates the provider components in the management cluster.
	Create(objs []unstructured.Unstructured) error

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

func (p *providerComponents) Create(objs []unstructured.Unstructured) error {
	createComponentObjectBackoff := newWriteBackoff()
	for i := range objs {
		obj := objs[i]

		// Create the Kubernetes object.
		// Nb. The operation is wrapped in a retry loop to make Create more resilient to unexpected conditions.
		if err := retryWithExponentialBackoff(createComponentObjectBackoff, func() error {
			return p.createObj(obj)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (p *providerComponents) createObj(obj unstructured.Unstructured) error {
	log := logf.Log
	c, err := p.proxy.NewClient()
	if err != nil {
		return err
	}

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
		log.V(5).Info("Creating", logf.UnstructuredToValues(obj)...)
		if err := c.Create(ctx, &obj); err != nil {
			return errors.Wrapf(err, "failed to create provider object %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
		}
		return nil
	}

	// otherwise update the component
	// NB. we are using client.Merge PatchOption so the new objects gets compared with the current one server side
	log.V(5).Info("Patching", logf.UnstructuredToValues(obj)...)
	obj.SetResourceVersion(currentR.GetResourceVersion())
	if err := c.Patch(ctx, &obj, client.Merge); err != nil {
		return errors.Wrapf(err, "failed to patch provider object")
	}
	return nil
}

func (p *providerComponents) Delete(options DeleteOptions) error {
	log := logf.Log
	log.Info("Deleting", "Provider", options.Provider.Name, "Version", options.Provider.Version, "TargetNamespace", options.Provider.Namespace)

	// Fetch all the components belonging to a provider.
	// We want that the delete operation is able to clean-up everything in a the most common use case that is
	// single-tenant management clusters. However, the downside of this is that this operation might be destructive
	// in multi-tenant scenario, because a single operation could delete both instance specific and shared CRDs/web-hook components.
	// This is considered acceptable because we are considering the multi-tenant scenario an advanced use case, and the assumption
	// is that user in this case understand the potential impacts of this operation.
	// TODO: in future we can eventually block delete --IncludeCRDs in case more than one instance of a provider exists
	labels := map[string]string{
		clusterctlv1.ClusterctlLabelName: "",
		clusterv1.ProviderLabelName:      options.Provider.ManifestLabel(),
	}

	namespaces := []string{options.Provider.Namespace}
	if options.IncludeCRDs {
		namespaces = append(namespaces, repository.WebhookNamespaceName)
	}

	resources, err := p.proxy.ListResources(labels, namespaces...)
	if err != nil {
		return err
	}

	// Filter the resources according to the delete options
	resourcesToDelete := []unstructured.Unstructured{}
	namespacesToDelete := sets.NewString()
	instanceNamespacePrefix := fmt.Sprintf("%s-", options.Provider.Namespace)
	for _, obj := range resources {
		// If the CRDs (and by extensions, all the shared resources) should NOT be deleted, skip it;
		// NB. Skipping CRDs deletion ensures that also the objects of Kind defined in the CRDs Kind are not deleted.
		isSharedResource := util.IsSharedResource(obj)
		if !options.IncludeCRDs && isSharedResource {
			continue
		}

		// If the resource is a namespace
		isNamespace := obj.GroupVersionKind().Kind == "Namespace"
		if isNamespace {
			// Skip all the namespaces not related to the provider instance being processed.
			if obj.GetName() != options.Provider.Namespace {
				continue
			}
			// If the  Namespace should NOT be deleted, skip it, otherwise keep track of the namespaces we are deleting;
			// NB. Skipping Namespaces deletion ensures that also the objects hosted in the namespace but without the "clusterctl.cluster.x-k8s.io" and the "cluster.x-k8s.io/provider" label are not deleted.
			if !options.IncludeNamespace {
				continue
			}
			namespacesToDelete.Insert(obj.GetName())
		}

		// If not a shared resource or not a namespace
		if !isSharedResource && !isNamespace {
			// If the resource is a cluster resource, skip it if the resource name does not start with the instance prefix.
			// This is required because there are cluster resources like e.g. ClusterRoles and ClusterRoleBinding, which are instance specific;
			// During the installation, clusterctl adds the instance namespace prefix to such resources (see fixRBAC), and so we can rely
			// on that for deleting only the global resources belonging the the instance we are processing.
			if util.IsClusterResource(obj.GetKind()) {
				if !strings.HasPrefix(obj.GetName(), instanceNamespacePrefix) {
					continue
				}
			}
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
		log.V(5).Info("Deleting", logf.UnstructuredToValues(obj)...)
		if err := cs.Delete(ctx, &obj); err != nil {
			if apierrors.IsNotFound(err) {
				// Tolerate IsNotFound error that might happen because we are not enforcing a deletion order
				// that considers relation across objects (e.g. Deployments -> ReplicaSets -> Pods)
				continue
			}
			errList = append(errList, errors.Wrapf(err, "Error deleting object %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName()))
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
