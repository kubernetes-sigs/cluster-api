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
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const embeddedCustomResourceDefinitionPath = "cmd/clusterctl/config/manifest/clusterctl-api.yaml"

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
	List() ([]clusterctlv1.Provider, error)

	// GetDefaultProviderName returns the default provider for a given ProviderType.
	// In case there is only a single provider for a given type, e.g. only the AWS infrastructure Provider, it returns
	// this as the default provider; In case there are more provider of the same type, there is no default provider.
	GetDefaultProviderName(providerType clusterctlv1.ProviderType) (string, error)
}

// inventoryClient implements InventoryClient.
type inventoryClient struct {
	proxy Proxy
}

// ensure inventoryClient implements InventoryClient.
var _ InventoryClient = &inventoryClient{}

// newInventoryClient returns a inventoryClient.
func newInventoryClient(proxy Proxy) *inventoryClient {
	return &inventoryClient{
		proxy: proxy,
	}
}

func (p *inventoryClient) EnsureCustomResourceDefinitions() error {
	c, err := p.proxy.NewClient()
	if err != nil {
		return err
	}

	// Check the CRDs already exists, if yes, exit immediately.
	l := &clusterctlv1.ProviderList{}
	if err := c.List(ctx, l); err != nil {
		if !apimeta.IsNoMatchError(err) {
			return errors.Wrap(err, "failed to check if the clusterctl inventory CRD exists")
		}
	}

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
	for _, o := range objs {
		klog.V(3).Infof("Creating: %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
		if err := c.Create(ctx, o.DeepCopy()); err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return errors.Wrapf(err, "failed to create clusterctl inventory CRDs component: %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
		}
	}

	return nil
}

func (p *inventoryClient) Validate(m clusterctlv1.Provider) error {
	instances, err := p.list(listOptions{
		Name: m.Name,
	})
	if err != nil {
		return err
	}

	if len(instances) == 0 {
		return nil
	}

	// Target Namespace check
	// Installing two instances of the same provider in the same namespace won't be supported
	for _, i := range instances {
		if i.Namespace == m.Namespace {
			return errors.Errorf("There is already an instance of the %q provider installed in the %q namespace", m.Name, m.Namespace)
		}
	}

	// Version check:
	// If we are going to install an instance of a provider with version X, and there are already other instances of the same provider with
	// different versions there is the risk of creating problems to all the version different than X because we are going to override
	// all the existing non-namespaced objects (e.g. CRDs) with the ones from version X
	sameVersion := false
	for _, i := range instances {
		if i.Version == m.Version {
			sameVersion = true
		}
	}
	if !sameVersion {
		return errors.Errorf("The new instance of the %q provider has a version different than other instances of the same provider", m.Name)
	}

	// Watching Namespace check:
	// If we are going to install an instance of a provider watching objects in namespaces already controlled by other providers
	// then there will be providers fighting for objects...
	if m.WatchedNamespace == "" {
		return errors.Errorf("The new instance of the %q provider is going to watch for objects in namespaces already controlled by other providers", m.Name)
	}

	sameNamespace := false
	for _, i := range instances {
		if i.WatchedNamespace == "" || m.WatchedNamespace == i.WatchedNamespace {
			sameNamespace = true
		}
	}
	if sameNamespace {
		return errors.Errorf("The new instance of the %q provider is going to watch for objects in the namespace %q that is already controlled by other providers", m.Name, m.WatchedNamespace)
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

func (p *inventoryClient) List() ([]clusterctlv1.Provider, error) {
	return p.list(listOptions{})
}

type listOptions struct {
	Namespace string
	Name      string
	Type      clusterctlv1.ProviderType
}

func (p *inventoryClient) list(options listOptions) ([]clusterctlv1.Provider, error) {
	cl, err := p.proxy.NewClient()
	if err != nil {
		return nil, err
	}

	l := &clusterctlv1.ProviderList{}
	if err := cl.List(ctx, l); err != nil {
		return nil, errors.Wrap(err, "failed get providers")
	}

	ret := []clusterctlv1.Provider{}
	for _, i := range l.Items {
		if options.Name != "" && i.Name != options.Name {
			continue
		}
		if options.Namespace != "" && i.Namespace != options.Namespace {
			continue
		}
		if options.Type != "" && i.Type != string(options.Type) {
			continue
		}
		ret = append(ret, i)
	}
	return ret, nil
}

func (p *inventoryClient) GetDefaultProviderName(providerType clusterctlv1.ProviderType) (string, error) {
	l, err := p.list(listOptions{
		Type: providerType,
	})
	if err != nil {
		return "", err
	}

	// Group the providers by name, because we consider more instance of the same provider not relevant for the answer.
	names := sets.NewString()
	for _, p := range l {
		names.Insert(p.Name)
	}

	// If there is only one provider, this is the default
	if names.Len() == 1 {
		return names.List()[0], nil
	}

	// There is no provider or more than one provider of this type; in both cases, a default provider name cannot be decided.
	return "", nil
}
