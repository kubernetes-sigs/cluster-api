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
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

const (
	namespaceKind                      = "Namespace"
	validatingWebhookConfigurationKind = "ValidatingWebhookConfiguration"
	mutatingWebhookConfigurationKind   = "MutatingWebhookConfiguration"
	customResourceDefinitionKind       = "CustomResourceDefinition"
	providerGroupKind                  = "Provider.clusterctl.cluster.x-k8s.io"
)

// DeleteOptions holds options for ComponentsClient.Delete func.
type DeleteOptions struct {
	Provider         clusterctlv1.Provider
	IncludeNamespace bool
	IncludeCRDs      bool
	SkipInventory    bool
}

// ComponentsClient has methods to work with provider components in the cluster.
type ComponentsClient interface {
	// Create creates the provider components in the management cluster.
	Create(ctx context.Context, objs []unstructured.Unstructured) error

	// Delete deletes the provider components from the management cluster.
	// The operation is designed to prevent accidental deletion of user created objects, so
	// it is required to explicitly opt-in for the deletion of the namespace where the provider components are hosted
	// and for the deletion of the provider's CRDs.
	Delete(ctx context.Context, options DeleteOptions) error

	// DeleteWebhookNamespace deletes the core provider webhook namespace (eg. capi-webhook-system).
	// This is required when upgrading to v1alpha4 where webhooks are included in the controller itself.
	DeleteWebhookNamespace(ctx context.Context) error

	// ValidateNoObjectsExist checks if custom resources of the custom resource definitions exist and returns an error if so.
	ValidateNoObjectsExist(ctx context.Context, provider clusterctlv1.Provider) error
}

// providerComponents implements ComponentsClient.
type providerComponents struct {
	proxy Proxy
}

func (p *providerComponents) Create(ctx context.Context, objs []unstructured.Unstructured) error {
	createComponentObjectBackoff := newWriteBackoff()
	for i := range objs {
		obj := objs[i]

		// Create the Kubernetes object.
		// Nb. The operation is wrapped in a retry loop to make Create more resilient to unexpected conditions.
		if err := retryWithExponentialBackoff(ctx, createComponentObjectBackoff, func(ctx context.Context) error {
			return p.createObj(ctx, obj)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (p *providerComponents) createObj(ctx context.Context, obj unstructured.Unstructured) error {
	log := logf.Log
	c, err := p.proxy.NewClient(ctx)
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

		// if it does not exists, create the component
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

func (p *providerComponents) Delete(ctx context.Context, options DeleteOptions) error {
	log := logf.Log
	log.Info("Deleting", "Provider", klog.KObj(&options.Provider), "providerVersion", options.Provider.Version)

	// Fetch all the components belonging to a provider.
	// We want that the delete operation is able to clean-up everything.
	labels := map[string]string{
		clusterctlv1.ClusterctlLabel: "",
		clusterv1.ProviderNameLabel:  options.Provider.ManifestLabel(),
	}

	namespaces := []string{options.Provider.Namespace}
	resources, err := p.proxy.ListResources(ctx, labels, namespaces...)
	if err != nil {
		return err
	}

	// Filter the resources according to the delete options
	resourcesToDelete := []unstructured.Unstructured{}
	namespacesToDelete := sets.Set[string]{}
	instanceNamespacePrefix := fmt.Sprintf("%s-", options.Provider.Namespace)
	for _, obj := range resources {
		// If the CRDs should NOT be deleted, skip it;
		// NB. Skipping CRDs deletion ensures that also the objects of Kind defined in the CRDs Kind are not deleted.
		isCRD := obj.GroupVersionKind().Kind == customResourceDefinitionKind
		if !options.IncludeCRDs && isCRD {
			continue
		}
		// If the resource is a namespace
		isNamespace := obj.GroupVersionKind().Kind == namespaceKind
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

		// If the resource is part of the inventory for clusterctl don't delete it at this point as losing this information makes the
		// upgrade function non-reentrant. Instead keep the inventory objects around until the upgrade is finished and working and
		// delete them at the end of the upgrade flow.
		isInventory := obj.GroupVersionKind().GroupKind().String() == providerGroupKind
		if isInventory && options.SkipInventory {
			continue
		}

		// If the resource is a cluster resource, skip it if the resource name does not start with the instance prefix.
		// This is required because there are cluster resources like e.g. ClusterRoles and ClusterRoleBinding, which are instance specific;
		// During the installation, clusterctl adds the instance namespace prefix to such resources (see fixRBAC), and so we can rely
		// on that for deleting only the global resources belonging the instance we are processing.
		// NOTE: namespace and CRD are special case managed above; webhook instead goes hand by hand with the controller they
		// should always be deleted.
		isWebhook := obj.GroupVersionKind().Kind == validatingWebhookConfigurationKind || obj.GroupVersionKind().Kind == mutatingWebhookConfigurationKind

		if util.IsClusterResource(obj.GetKind()) &&
			!isNamespace && !isCRD && !isWebhook &&
			// TODO(oscr) Delete the check below condition when the min version to upgrade from is CAPI v1.3
			// This check is needed due to the (now removed) support for multiple instances of the same provider.
			// For more context read GitHub issue #7318 and/or PR #7339
			!strings.HasPrefix(obj.GetName(), instanceNamespacePrefix) {
			continue
		}
		resourcesToDelete = append(resourcesToDelete, obj)
	}

	// Delete all the provider components.
	cs, err := p.proxy.NewClient(ctx)
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
		deleteBackoff := newWriteBackoff()
		if err := retryWithExponentialBackoff(ctx, deleteBackoff, func(ctx context.Context) error {
			if err := cs.Delete(ctx, &obj); err != nil {
				if apierrors.IsNotFound(err) {
					// Tolerate IsNotFound error that might happen because we are not enforcing a deletion order
					// that considers relation across objects (e.g. Deployments -> ReplicaSets -> Pods)
					return nil
				}
				return err
			}
			return nil
		}); err != nil {
			errList = append(errList, errors.Wrapf(err, "Error deleting object %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName()))
		}
	}

	return kerrors.NewAggregate(errList)
}

func (p *providerComponents) DeleteWebhookNamespace(ctx context.Context) error {
	const webhookNamespaceName = "capi-webhook-system"

	log := logf.Log
	log.V(5).Info("Deleting", "namespace", webhookNamespaceName)

	c, err := p.proxy.NewClient(ctx)
	if err != nil {
		return err
	}

	coreProviderWebhookNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: webhookNamespaceName}}
	if err := c.Delete(ctx, coreProviderWebhookNs); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to delete namespace %s", webhookNamespaceName)
	}

	return nil
}

func (p *providerComponents) ValidateNoObjectsExist(ctx context.Context, provider clusterctlv1.Provider) error {
	log := logf.Log
	log.Info("Checking for CRs", "Provider", klog.KObj(&provider), "providerVersion", provider.Version)

	proxyClient, err := p.proxy.NewClient(ctx)
	if err != nil {
		return err
	}

	// Fetch all the components belonging to a provider.
	// We want that the delete operation is able to clean-up everything.
	labels := map[string]string{
		clusterctlv1.ClusterctlLabel: "",
		clusterv1.ProviderNameLabel:  provider.ManifestLabel(),
	}

	customResources := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := proxyClient.List(ctx, customResources, client.MatchingLabels(labels)); err != nil {
		return err
	}

	// Filter the resources according to the delete options
	crsHavingObjects := []string{}
	for _, crd := range customResources.Items {
		storageVersion, err := storageVersionForCRD(&crd)
		if err != nil {
			return err
		}

		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   crd.Spec.Group,
			Version: storageVersion,
			Kind:    crd.Spec.Names.ListKind,
		})

		if err := proxyClient.List(ctx, list); err != nil {
			return err
		}

		if len(list.Items) > 0 {
			crsHavingObjects = append(crsHavingObjects, crd.Kind)
		}
	}

	if len(crsHavingObjects) > 0 {
		return fmt.Errorf("found existing objects for provider CRDs %q: [%s]. Please delete these objects first before running clusterctl delete with --include-crd", provider.GetName(), strings.Join(crsHavingObjects, ", "))
	}

	return nil
}

// newComponentsClient returns a providerComponents.
func newComponentsClient(proxy Proxy) *providerComponents {
	return &providerComponents{
		proxy: proxy,
	}
}
