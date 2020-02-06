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
	"time"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	embeddedCustomResourceDefinitionPath = "cmd/clusterctl/config/manifest/clusterctl-api.yaml"

	waitInventoryCRDInterval = 250 * time.Millisecond
	waitInventoryCRDTimeout  = 1 * time.Minute
)

// InventoryClient exposes methods to interface with a cluster's provider inventory.
type InventoryClient interface {
	// EnsureCustomResourceDefinitions installs the CRD required for creating inventory items, if necessary.
	// Nb. In order to provide a simpler out-of-the box experience, the inventory CRD
	// is embedded in the clusterctl binary.
	EnsureCustomResourceDefinitions() error

	// Validate checks the inventory in order to determine if adding a new provider instance to the cluster.
	// can lead to an non functional cluster.
	Validate(clusterctlv1.Provider) error

	// Create an inventory item for a provider instance installed in the cluster.
	Create(clusterctlv1.Provider) error

	// List returns the inventory items for all the provider instances installed in the cluster.
	List() (*clusterctlv1.ProviderList, error)

	// GetDefaultProviderName returns the default provider for a given ProviderType.
	// In case there is only a single provider for a given type, e.g. only the AWS infrastructure Provider, it returns
	// this as the default provider; In case there are more provider of the same type, there is no default provider.
	GetDefaultProviderName(providerType clusterctlv1.ProviderType) (string, error)

	// GetDefaultProviderVersion returns the default version for a given provider.
	// In case there is only a single version installed for a given provider, e.g. only the v0.4.1 version for the AWS provider, it returns
	// this as the default version; In case there are more version installed for the same provider, there is no default provider version.
	GetDefaultProviderVersion(provider string) (string, error)

	// GetDefaultProviderNamespace returns the default namespace for a given provider.
	// In case there is only a single instance for a given provider, e.g. only the AWS provider in the capa-system namespace, it returns
	// this as the default namespace; In case there are more instances for the same provider installed in different namespaces, there is no default provider namespace.
	GetDefaultProviderNamespace(provider string) (string, error)

	// GetManagementGroups returns the list of management groups defined in the management cluster.
	GetManagementGroups() (ManagementGroupList, error)
}

// inventoryClient implements InventoryClient.
type inventoryClient struct {
	proxy               Proxy
	pollImmediateWaiter PollImmediateWaiter
}

// ensure inventoryClient implements InventoryClient.
var _ InventoryClient = &inventoryClient{}

// newInventoryClient returns a inventoryClient.
func newInventoryClient(proxy Proxy, pollImmediateWaiter PollImmediateWaiter) *inventoryClient {
	return &inventoryClient{
		proxy:               proxy,
		pollImmediateWaiter: pollImmediateWaiter,
	}
}

func (p *inventoryClient) EnsureCustomResourceDefinitions() error {
	log := logf.Log

	c, err := p.proxy.NewClient()
	if err != nil {
		return err
	}

	// Check the CRDs already exists, if yes, exit immediately.
	l := &clusterctlv1.ProviderList{}
	if err = c.List(ctx, l); err == nil {
		return nil
	}
	if !apimeta.IsNoMatchError(err) {
		return errors.Wrap(err, "failed to check if the clusterctl inventory CRD exists")
	}

	log.V(1).Info("Installing the clusterctl inventory CRD")

	// Get the CRDs manifest from the embedded assets.
	yaml, err := config.Asset(embeddedCustomResourceDefinitionPath)
	if err != nil {
		return err
	}

	// Transform the yaml in a list of objects.
	objs, err := util.ToUnstructured(yaml)
	if err != nil {
		return errors.Wrap(err, "failed to parse yaml for clusterctl inventory CRDs")
	}

	// Install the CRDs.
	for i := range objs {
		o := objs[i]
		log.V(5).Info("Creating", logf.UnstructuredToValues(o)...)

		labels := o.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[clusterctlv1.ClusterctlCoreLabelName] = "inventory"
		o.SetLabels(labels)

		if err := c.Create(ctx, &o); err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return errors.Wrapf(err, "failed to create clusterctl inventory CRDs component: %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
		}

		// If the object is a CRDs, waits for it being Established.
		if apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition").GroupKind() == o.GroupVersionKind().GroupKind() {
			crdKey, err := client.ObjectKeyFromObject(&o)
			if err != nil {
				return nil
			}

			if err := p.pollImmediateWaiter(waitInventoryCRDInterval, waitInventoryCRDTimeout, func() (bool, error) {
				crd := &apiextensionsv1.CustomResourceDefinition{}
				if err := c.Get(ctx, crdKey, crd); err != nil {
					return false, err
				}

				for _, c := range crd.Status.Conditions {
					if c.Type == apiextensionsv1.Established && c.Status == apiextensionsv1.ConditionTrue {
						return true, nil
					}
				}
				return false, nil
			}); err != nil {
				return errors.Wrapf(err, "failed to scale deployment")
			}
		}
	}

	return nil
}

func (p *inventoryClient) Validate(m clusterctlv1.Provider) error {
	providerList, err := p.List()
	if err != nil {
		return err
	}

	providers := providerList.FilterByName(m.Name)

	if len(providers) == 0 {
		return nil
	}

	// Target Namespace check
	// Installing two instances of the same provider in the same namespace won't be supported
	for _, i := range providers {
		if i.Namespace == m.Namespace {
			return errors.Errorf("there is already an instance of the %q provider installed in the %q namespace", m.Name, m.Namespace)
		}
	}

	// Watching Namespace check:
	// If we are going to install an instance of a provider watching objects in namespaces already controlled by other providers
	// then there will be providers fighting for objects...
	for _, i := range providers {
		if i.HasWatchingOverlapWith(m) {
			return errors.Errorf("the new instance of the %q provider is going to watch for objects in the namespace %q that is already controlled by other providers", m.Name, m.WatchedNamespace)
		}
	}

	return nil
}

func (p *inventoryClient) Create(m clusterctlv1.Provider) error {
	cl, err := p.proxy.NewClient()
	if err != nil {
		return err
	}

	currentProvider := &clusterctlv1.Provider{}
	key := client.ObjectKey{
		Namespace: m.Namespace,
		Name:      m.Name,
	}
	if err := cl.Get(ctx, key, currentProvider); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get current provider object")
		}
		currentProvider = nil
	}

	c := m.DeepCopyObject()
	if currentProvider == nil {
		if err := cl.Create(ctx, c); err != nil {
			return errors.Wrapf(err, "failed to create provider object")
		}
	} else {
		m.ResourceVersion = currentProvider.ResourceVersion
		if err := cl.Update(ctx, c); err != nil {
			return errors.Wrapf(err, "failed to update provider object")
		}
	}

	return nil
}

func (p *inventoryClient) List() (*clusterctlv1.ProviderList, error) {
	cl, err := p.proxy.NewClient()
	if err != nil {
		return nil, err
	}

	providerList := &clusterctlv1.ProviderList{}
	if err := cl.List(ctx, providerList); err != nil {
		return nil, errors.Wrap(err, "failed get providers")
	}
	return providerList, nil
}

func (p *inventoryClient) GetDefaultProviderName(providerType clusterctlv1.ProviderType) (string, error) {
	providerList, err := p.List()
	if err != nil {
		return "", err
	}

	// Group the providers by name, because we consider more instance of the same provider not relevant for the answer.
	names := sets.NewString()
	for _, p := range providerList.FilterByType(providerType) {
		names.Insert(p.Name)
	}

	// If there is only one provider, this is the default
	if names.Len() == 1 {
		return names.List()[0], nil
	}

	// There is no provider or more than one provider of this type; in both cases, a default provider name cannot be decided.
	return "", nil
}

func (p *inventoryClient) GetDefaultProviderVersion(provider string) (string, error) {
	providerList, err := p.List()
	if err != nil {
		return "", err
	}

	// Group the provider instances by version.
	versions := sets.NewString()
	for _, p := range providerList.FilterByName(provider) {
		versions.Insert(p.Version)
	}

	if versions.Len() == 1 {
		return versions.List()[0], nil
	}

	// There is no version installed or more than one version installed for this provider; in both cases, a default version for this provider cannot be decided.
	return "", nil
}

func (p *inventoryClient) GetDefaultProviderNamespace(provider string) (string, error) {
	providerList, err := p.List()
	if err != nil {
		return "", err
	}

	// Group the providers by namespace
	namespaces := sets.NewString()
	for _, p := range providerList.FilterByName(provider) {
		namespaces.Insert(p.Namespace)
	}

	if namespaces.Len() == 1 {
		return namespaces.List()[0], nil
	}

	// There is no provider or more than one namespace for this provider; in both cases, a default provider namespace cannot be decided.
	return "", nil
}
