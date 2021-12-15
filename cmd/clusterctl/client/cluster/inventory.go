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
	"time"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/config"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

const (
	waitInventoryCRDInterval = 250 * time.Millisecond
	waitInventoryCRDTimeout  = 1 * time.Minute
)

// CheckCAPIContractOption is some configuration that modifies options for CheckCAPIContract.
type CheckCAPIContractOption interface {
	// Apply applies this configuration to the given CheckCAPIContractOptions.
	Apply(*CheckCAPIContractOptions)
}

// CheckCAPIContractOptions contains options for CheckCAPIContract.
type CheckCAPIContractOptions struct {
	// AllowCAPINotInstalled instructs CheckCAPIContract to tolerate management clusters without Cluster API installed yet.
	AllowCAPINotInstalled bool

	// AllowCAPIContracts instructs CheckCAPIContract to tolerate management clusters with Cluster API with the given contract.
	AllowCAPIContracts []string

	// AllowCAPIAnyContract instructs CheckCAPIContract to tolerate management clusters with Cluster API installed with any contract.
	AllowCAPIAnyContract bool
}

// AllowCAPINotInstalled instructs CheckCAPIContract to tolerate management clusters without Cluster API installed yet.
// NOTE: This allows clusterctl init to run on empty management clusters.
type AllowCAPINotInstalled struct{}

// Apply applies this configuration to the given CheckCAPIContractOptions.
func (t AllowCAPINotInstalled) Apply(in *CheckCAPIContractOptions) {
	in.AllowCAPINotInstalled = true
}

// AllowCAPIAnyContract instructs CheckCAPIContract to tolerate management clusters with Cluster API with any contract.
// NOTE: This allows clusterctl generate cluster with managed topologies to work properly by performing checks to see if CAPI is installed.
type AllowCAPIAnyContract struct{}

// Apply applies this configuration to the given CheckCAPIContractOptions.
func (t AllowCAPIAnyContract) Apply(in *CheckCAPIContractOptions) {
	in.AllowCAPIAnyContract = true
}

// AllowCAPIContract instructs CheckCAPIContract to tolerate management clusters with Cluster API with the given contract.
// NOTE: This allows clusterctl upgrade to work on management clusters with old contract.
type AllowCAPIContract struct {
	Contract string
}

// Apply applies this configuration to the given CheckCAPIContractOptions.
func (t AllowCAPIContract) Apply(in *CheckCAPIContractOptions) {
	in.AllowCAPIContracts = append(in.AllowCAPIContracts, t.Contract)
}

// InventoryClient exposes methods to interface with a cluster's provider inventory.
type InventoryClient interface {
	// EnsureCustomResourceDefinitions installs the CRD required for creating inventory items, if necessary.
	// Nb. In order to provide a simpler out-of-the box experience, the inventory CRD
	// is embedded in the clusterctl binary.
	EnsureCustomResourceDefinitions() error

	// Create an inventory item for a provider instance installed in the cluster.
	Create(clusterctlv1.Provider) error

	// List returns the inventory items for all the provider instances installed in the cluster.
	List() (*clusterctlv1.ProviderList, error)

	// GetDefaultProviderName returns the default provider for a given ProviderType.
	// In case there is only a single provider for a given type, e.g. only the AWS infrastructure Provider, it returns
	// this as the default provider; In case there are more provider of the same type, there is no default provider.
	GetDefaultProviderName(providerType clusterctlv1.ProviderType) (string, error)

	// GetProviderVersion returns the version for a given provider.
	GetProviderVersion(provider string, providerType clusterctlv1.ProviderType) (string, error)

	// GetProviderNamespace returns the namespace for a given provider.
	GetProviderNamespace(provider string, providerType clusterctlv1.ProviderType) (string, error)

	// CheckCAPIContract checks the Cluster API version installed in the management cluster, and fails if this version
	// does not match the current one supported by clusterctl.
	CheckCAPIContract(...CheckCAPIContractOption) error

	// CheckCAPIInstalled checks if Cluster API is installed on the management cluster.
	CheckCAPIInstalled() (bool, error)

	// CheckSingleProviderInstance ensures that only one instance of a provider is running, returns error otherwise.
	CheckSingleProviderInstance() error
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

	if err := p.proxy.ValidateKubernetesVersion(); err != nil {
		return err
	}

	// Being this the first connection of many clusterctl operations, we want to fail fast if there is no
	// connectivity to the cluster, so we try to get a client as a first thing.
	// NB. NewClient has an internal retry loop that should mitigate temporary connection glitch; here we are
	// trying to detect persistent connection problems (>10s) before entering in longer retry loops while executing
	// clusterctl operations.
	_, err := p.proxy.NewClient()
	if err != nil {
		return err
	}

	// Check the CRDs already exists, if yes, exit immediately.
	// Nb. The operation is wrapped in a retry loop to make EnsureCustomResourceDefinitions more resilient to unexpected conditions.
	var crdIsIstalled bool
	listInventoryBackoff := newReadBackoff()
	if err := retryWithExponentialBackoff(listInventoryBackoff, func() error {
		var err error
		crdIsIstalled, err = checkInventoryCRDs(p.proxy)
		return err
	}); err != nil {
		return err
	}
	if crdIsIstalled {
		return nil
	}

	log.V(1).Info("Installing the clusterctl inventory CRD")

	// Transform the yaml in a list of objects.
	objs, err := utilyaml.ToUnstructured(config.ClusterctlAPIManifest)
	if err != nil {
		return errors.Wrap(err, "failed to parse yaml for clusterctl inventory CRDs")
	}

	// Install the CRDs.
	createInventoryObjectBackoff := newWriteBackoff()
	for i := range objs {
		o := objs[i]
		log.V(5).Info("Creating", logf.UnstructuredToValues(o)...)

		// Create the Kubernetes object.
		// Nb. The operation is wrapped in a retry loop to make EnsureCustomResourceDefinitions more resilient to unexpected conditions.
		if err := retryWithExponentialBackoff(createInventoryObjectBackoff, func() error {
			return p.createObj(o)
		}); err != nil {
			return err
		}

		// If the object is a CRDs, waits for it being Established.
		if apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition").GroupKind() == o.GroupVersionKind().GroupKind() {
			crdKey := client.ObjectKeyFromObject(&o)
			if err := p.pollImmediateWaiter(waitInventoryCRDInterval, waitInventoryCRDTimeout, func() (bool, error) {
				c, err := p.proxy.NewClient()
				if err != nil {
					return false, err
				}

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

// checkInventoryCRDs checks if the inventory CRDs are installed in the cluster.
func checkInventoryCRDs(proxy Proxy) (bool, error) {
	c, err := proxy.NewClient()
	if err != nil {
		return false, err
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := c.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("providers.%s", clusterctlv1.GroupVersion.Group)}, crd); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to check if the clusterctl inventory CRD exists")
	}

	for _, version := range crd.Spec.Versions {
		if version.Name == clusterctlv1.GroupVersion.Version {
			return true, nil
		}
	}
	return true, errors.Errorf("clusterctl inventory CRD does not defines the %s version", clusterctlv1.GroupVersion.Version)
}

func (p *inventoryClient) createObj(o unstructured.Unstructured) error {
	c, err := p.proxy.NewClient()
	if err != nil {
		return err
	}

	labels := o.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[clusterctlv1.ClusterctlCoreLabelName] = clusterctlv1.ClusterctlCoreLabelInventoryValue
	o.SetLabels(labels)

	if err := c.Create(ctx, &o); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to create clusterctl inventory CRDs component: %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
	}
	return nil
}

func (p *inventoryClient) Create(m clusterctlv1.Provider) error {
	// Create the Kubernetes object.
	createInventoryObjectBackoff := newWriteBackoff()
	return retryWithExponentialBackoff(createInventoryObjectBackoff, func() error {
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

			// if it does not exists, create the provider object
			if err := cl.Create(ctx, &m); err != nil {
				return errors.Wrapf(err, "failed to create provider object")
			}
			return nil
		}

		// otherwise patch the provider object
		// NB. we are using client.Merge PatchOption so the new objects gets compared with the current one server side
		m.SetResourceVersion(currentProvider.GetResourceVersion())
		if err := cl.Patch(ctx, &m, client.Merge); err != nil {
			return errors.Wrapf(err, "failed to patch provider object")
		}

		return nil
	})
}

func (p *inventoryClient) List() (*clusterctlv1.ProviderList, error) {
	providerList := &clusterctlv1.ProviderList{}

	listProvidersBackoff := newReadBackoff()
	if err := retryWithExponentialBackoff(listProvidersBackoff, func() error {
		return listProviders(p.proxy, providerList)
	}); err != nil {
		return nil, err
	}

	return providerList, nil
}

// listProviders retrieves the list of provider inventory objects.
func listProviders(proxy Proxy, providerList *clusterctlv1.ProviderList) error {
	cl, err := proxy.NewClient()
	if err != nil {
		return err
	}

	if err := cl.List(ctx, providerList); err != nil {
		return errors.Wrap(err, "failed get providers")
	}
	return nil
}

func (p *inventoryClient) GetDefaultProviderName(providerType clusterctlv1.ProviderType) (string, error) {
	providerList, err := p.List()
	if err != nil {
		return "", err
	}

	// Group the providers by name, because we consider more instance of the same provider not relevant for the answer.
	names := sets.NewString()
	for _, p := range providerList.FilterByType(providerType) {
		names.Insert(p.ProviderName)
	}

	// If there is only one provider, this is the default
	if names.Len() == 1 {
		return names.List()[0], nil
	}

	// There is no provider or more than one provider of this type; in both cases, a default provider name cannot be decided.
	return "", nil
}

func (p *inventoryClient) GetProviderVersion(provider string, providerType clusterctlv1.ProviderType) (string, error) {
	providerList, err := p.List()
	if err != nil {
		return "", err
	}

	// Group the provider instances by version.
	versions := sets.NewString()
	for _, p := range providerList.FilterByProviderNameAndType(provider, providerType) {
		versions.Insert(p.Version)
	}

	if versions.Len() == 1 {
		return versions.List()[0], nil
	}

	// The default version for this provider cannot be decided.
	return "", nil
}

func (p *inventoryClient) GetProviderNamespace(provider string, providerType clusterctlv1.ProviderType) (string, error) {
	providerList, err := p.List()
	if err != nil {
		return "", err
	}

	// Group the providers by namespace
	namespaces := sets.NewString()
	for _, p := range providerList.FilterByProviderNameAndType(provider, providerType) {
		namespaces.Insert(p.Namespace)
	}

	if namespaces.Len() == 1 {
		return namespaces.List()[0], nil
	}

	// The default provider namespace cannot be decided.
	return "", nil
}

func (p *inventoryClient) CheckCAPIContract(options ...CheckCAPIContractOption) error {
	opt := &CheckCAPIContractOptions{}
	for _, o := range options {
		o.Apply(opt)
	}

	c, err := p.proxy.NewClient()
	if err != nil {
		return err
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := c.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("clusters.%s", clusterv1.GroupVersion.Group)}, crd); err != nil {
		if opt.AllowCAPINotInstalled && apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to check Cluster API version")
	}

	if opt.AllowCAPIAnyContract {
		return nil
	}

	for _, version := range crd.Spec.Versions {
		if version.Storage {
			if version.Name == clusterv1.GroupVersion.Version {
				return nil
			}
			for _, allowedContract := range opt.AllowCAPIContracts {
				if version.Name == allowedContract {
					return nil
				}
			}
			return errors.Errorf("this version of clusterctl could be used only with %q management clusters, %q detected", clusterv1.GroupVersion.Version, version.Name)
		}
	}
	return errors.Errorf("failed to check Cluster API version")
}

func (p *inventoryClient) CheckCAPIInstalled() (bool, error) {
	if err := p.CheckCAPIContract(AllowCAPIAnyContract{}); err != nil {
		if apierrors.IsNotFound(err) {
			// The expected CRDs are not installed on the management. This would mean that Cluster API is not installed on the cluster.
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (p *inventoryClient) CheckSingleProviderInstance() error {
	providers, err := p.List()
	if err != nil {
		return err
	}

	providerGroups := make(map[string][]string)
	for _, p := range providers.Items {
		namespacedName := types.NamespacedName{Namespace: p.Namespace, Name: p.Name}.String()
		if providers, ok := providerGroups[p.ManifestLabel()]; ok {
			providerGroups[p.ManifestLabel()] = append(providers, namespacedName)
		} else {
			providerGroups[p.ManifestLabel()] = []string{namespacedName}
		}
	}

	var errs []error
	for provider, providerInstances := range providerGroups {
		if len(providerInstances) > 1 {
			errs = append(errs, errors.Errorf("multiple instance of provider type %q found: %v", provider, providerInstances))
		}
	}

	if len(errs) > 0 {
		return errors.Wrap(kerrors.NewAggregate(errs), "detected multiple instances of the same provider, "+
			"but clusterctl does not support this use case. See https://cluster-api.sigs.k8s.io/developer/architecture/controllers/support-multiple-instances.html for more details")
	}

	return nil
}
