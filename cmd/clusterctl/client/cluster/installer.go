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
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/util/contract"
)

// ProviderInstaller defines methods for enforcing consistency rules for provider installation.
type ProviderInstaller interface {
	// Add adds a provider to the install queue.
	// NB. By deferring the installation, the installer service can perform validation of the target state of the management cluster
	// before actually starting the installation of new providers.
	Add(repository.Components)

	// Install performs the installation of the providers ready in the install queue.
	Install(InstallOptions) ([]repository.Components, error)

	// Validate performs steps to validate a management cluster by looking at the current state and the providers in the queue.
	// The following checks are performed in order to ensure a fully operational cluster:
	// - There must be only one instance of the same provider
	// - All the providers in must support the same API Version of Cluster API (contract)
	// - All provider CRDs that are referenced in core Cluster API CRDs must comply with the CRD naming scheme,
	//   otherwise a warning is logged.
	Validate() error

	// Images returns the list of images required for installing the providers ready in the install queue.
	Images() []string
}

// InstallOptions defines the options used to configure installation.
type InstallOptions struct {
	WaitProviders       bool
	WaitProviderTimeout time.Duration
}

// providerInstaller implements ProviderInstaller.
type providerInstaller struct {
	configClient            config.Client
	repositoryClientFactory RepositoryClientFactory
	proxy                   Proxy
	providerComponents      ComponentsClient
	providerInventory       InventoryClient
	installQueue            []repository.Components
}

var _ ProviderInstaller = &providerInstaller{}

func (i *providerInstaller) Add(components repository.Components) {
	i.installQueue = append(i.installQueue, components)

	// Ensure Providers are installed in the following order: Core, Bootstrap, ControlPlane, Infrastructure.
	sort.Slice(i.installQueue, func(a, b int) bool {
		return i.installQueue[a].Type().Order() < i.installQueue[b].Type().Order()
	})
}

func (i *providerInstaller) Install(opts InstallOptions) ([]repository.Components, error) {
	ret := make([]repository.Components, 0, len(i.installQueue))
	for _, components := range i.installQueue {
		if err := installComponentsAndUpdateInventory(components, i.providerComponents, i.providerInventory); err != nil {
			return nil, err
		}

		ret = append(ret, components)
	}

	return ret, waitForProvidersReady(opts, i.installQueue, i.proxy)
}

func installComponentsAndUpdateInventory(components repository.Components, providerComponents ComponentsClient, providerInventory InventoryClient) error {
	log := logf.Log
	log.Info("Installing", "Provider", components.ManifestLabel(), "Version", components.Version(), "TargetNamespace", components.TargetNamespace())

	inventoryObject := components.InventoryObject()

	log.V(1).Info("Creating objects", "Provider", components.ManifestLabel(), "Version", components.Version(), "TargetNamespace", components.TargetNamespace())
	if err := providerComponents.Create(components.Objs()); err != nil {
		return err
	}

	log.V(1).Info("Creating inventory entry", "Provider", components.ManifestLabel(), "Version", components.Version(), "TargetNamespace", components.TargetNamespace())
	return providerInventory.Create(inventoryObject)
}

// waitForProvidersReady waits till the installed components are ready.
func waitForProvidersReady(opts InstallOptions, installQueue []repository.Components, proxy Proxy) error {
	// If we dont have to wait for providers to be installed
	// return early.
	if !opts.WaitProviders {
		return nil
	}

	log := logf.Log
	log.Info("Waiting for providers to be available...")

	return waitManagerDeploymentsReady(opts, installQueue, proxy)
}

// waitManagerDeploymentsReady waits till the installed manager deployments are ready.
func waitManagerDeploymentsReady(opts InstallOptions, installQueue []repository.Components, proxy Proxy) error {
	for _, components := range installQueue {
		for _, obj := range components.Objs() {
			if util.IsDeploymentWithManager(obj) {
				if err := waitDeploymentReady(obj, opts.WaitProviderTimeout, proxy); err != nil {
					return errors.Wrapf(err, "deployment %q is not ready after %s", obj.GetName(), opts.WaitProviderTimeout)
				}
			}
		}
	}
	return nil
}

func waitDeploymentReady(deployment unstructured.Unstructured, timeout time.Duration, proxy Proxy) error {
	return wait.PollUntilContextTimeout(context.TODO(), 100*time.Millisecond, timeout, false, func(ctx context.Context) (bool, error) {
		c, err := proxy.NewClient()
		if err != nil {
			return false, err
		}
		key := client.ObjectKey{
			Namespace: deployment.GetNamespace(),
			Name:      deployment.GetName(),
		}
		dep := &appsv1.Deployment{}
		if err := c.Get(ctx, key, dep); err != nil {
			return false, err
		}
		for _, c := range dep.Status.Conditions {
			if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

func (i *providerInstaller) Validate() error {
	// Get the list of providers currently in the cluster.
	providerList, err := i.providerInventory.List()
	if err != nil {
		return err
	}

	// Starts simulating what will be the resulting management cluster by adding to the list the providers in the installQueue.
	// During this operation following checks are performed:
	// - There must be only one instance of the same provider
	for _, components := range i.installQueue {
		if providerList, err = simulateInstall(providerList, components); err != nil {
			return errors.Wrapf(err, "installing provider %q can lead to a non functioning management cluster", components.ManifestLabel())
		}
	}

	// Gets the API Version of Cluster API (contract) all the providers in the management cluster must support,
	// which is the same of the core provider.
	providerInstanceContracts := map[string]string{}

	coreProviders := providerList.FilterCore()
	if len(coreProviders) != 1 {
		return errors.Errorf("invalid management cluster: there should a core provider, found %d", len(coreProviders))
	}
	coreProvider := coreProviders[0]

	managementClusterContract, err := i.getProviderContract(providerInstanceContracts, coreProvider)
	if err != nil {
		return err
	}

	// Checks if all the providers supports the same API Version of Cluster API (contract).
	for _, components := range i.installQueue {
		provider := components.InventoryObject()

		// Gets the API Version of Cluster API (contract) the provider support and compare it with the management cluster contract.
		providerContract, err := i.getProviderContract(providerInstanceContracts, provider)
		if err != nil {
			return err
		}
		if providerContract != managementClusterContract {
			return errors.Errorf("installing provider %q can lead to a non functioning management cluster: the target version for the provider supports the %s API Version of Cluster API (contract), while the management cluster is using %s", components.ManifestLabel(), providerContract, managementClusterContract)
		}
	}

	// Validate if provider CRDs comply with the naming scheme.
	for _, components := range i.installQueue {
		componentObjects := components.Objs()

		for _, obj := range componentObjects {
			// Continue if object is not a CRD.
			if obj.GetKind() != customResourceDefinitionKind {
				continue
			}

			gk, err := getCRDGroupKind(obj)
			if err != nil {
				return errors.Wrap(err, "failed to read group and kind from CustomResourceDefinition")
			}

			if err := validateCRDName(obj, gk); err != nil {
				return err
			}
		}
	}

	return nil
}

func getCRDGroupKind(obj unstructured.Unstructured) (*schema.GroupKind, error) {
	var group string
	var kind string
	version := obj.GroupVersionKind().Version
	switch version {
	case apiextensionsv1beta1.SchemeGroupVersion.Version:
		crd := &apiextensionsv1beta1.CustomResourceDefinition{}
		if err := scheme.Scheme.Convert(&obj, crd, nil); err != nil {
			return nil, errors.Wrapf(err, "failed to convert %s CustomResourceDefinition %q", version, obj.GetName())
		}
		group = crd.Spec.Group
		kind = crd.Spec.Names.Kind
	case apiextensionsv1.SchemeGroupVersion.Version:
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := scheme.Scheme.Convert(&obj, crd, nil); err != nil {
			return nil, errors.Wrapf(err, "failed to convert %s CustomResourceDefinition %q", version, obj.GetName())
		}
		group = crd.Spec.Group
		kind = crd.Spec.Names.Kind
	default:
		return nil, errors.Errorf("failed to read %s CustomResourceDefinition %q", version, obj.GetName())
	}
	return &schema.GroupKind{Group: group, Kind: kind}, nil
}

func validateCRDName(obj unstructured.Unstructured, gk *schema.GroupKind) error {
	// Return if CRD has skip CRD name preflight check annotation.
	if _, ok := obj.GetAnnotations()[clusterctlv1.SkipCRDNamePreflightCheckAnnotation]; ok {
		return nil
	}

	correctCRDName := contract.CalculateCRDName(gk.Group, gk.Kind)

	// Return if name is correct.
	if obj.GetName() == correctCRDName {
		return nil
	}

	return errors.Errorf("ERROR: CRD name %q is invalid for a CRD referenced in a core Cluster API CRD,"+
		"it should be %q. Support for CRDs that are referenced in core Cluster API resources with invalid names has been "+
		"dropped. Note: Please check if this CRD is actually referenced in core Cluster API "+
		"CRDs. If not, this warning can be hidden by setting the %q' annotation.", obj.GetName(), correctCRDName, clusterctlv1.SkipCRDNamePreflightCheckAnnotation)
}

// getProviderContract returns the API Version of Cluster API (contract) for a provider instance.
func (i *providerInstaller) getProviderContract(providerInstanceContracts map[string]string, provider clusterctlv1.Provider) (string, error) {
	// If the contract for the provider instance is already known, return it.
	if contract, ok := providerInstanceContracts[provider.InstanceName()]; ok {
		return contract, nil
	}

	// Otherwise get the contract for the providers instance.

	// Gets the providers metadata.
	configRepository, err := i.configClient.Providers().Get(provider.ProviderName, provider.GetProviderType())
	if err != nil {
		return "", err
	}

	providerRepository, err := i.repositoryClientFactory(configRepository, i.configClient)
	if err != nil {
		return "", err
	}

	latestMetadata, err := providerRepository.Metadata(provider.Version).Get()
	if err != nil {
		return "", err
	}

	// Gets the contract for the current release.
	currentVersion, err := version.ParseSemantic(provider.Version)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse current version for the %s provider", provider.InstanceName())
	}

	releaseSeries := latestMetadata.GetReleaseSeriesForVersion(currentVersion)
	if releaseSeries == nil {
		return "", errors.Errorf("invalid provider metadata: version %s for the provider %s does not match any release series", provider.Version, provider.InstanceName())
	}

	if releaseSeries.Contract != clusterv1.GroupVersion.Version {
		return "", errors.Errorf("current version of clusterctl is only compatible with %s providers, detected %s for provider %s", clusterv1.GroupVersion.Version, releaseSeries.Contract, provider.ManifestLabel())
	}

	providerInstanceContracts[provider.InstanceName()] = releaseSeries.Contract
	return releaseSeries.Contract, nil
}

// simulateInstall adds a provider to the list of providers in a cluster (without installing it).
func simulateInstall(providerList *clusterctlv1.ProviderList, components repository.Components) (*clusterctlv1.ProviderList, error) {
	provider := components.InventoryObject()

	existingInstances := providerList.FilterByProviderNameAndType(provider.ProviderName, provider.GetProviderType())
	if len(existingInstances) > 0 {
		namespaces := func() string {
			var namespaces []string
			for _, provider := range existingInstances {
				namespaces = append(namespaces, provider.Namespace)
			}
			return strings.Join(namespaces, ", ")
		}()
		return providerList, errors.Errorf("there is already an instance of the %q provider installed in the %q namespace", provider.ManifestLabel(), namespaces)
	}

	providerList.Items = append(providerList.Items, provider)
	return providerList, nil
}

func (i *providerInstaller) Images() []string {
	ret := sets.Set[string]{}
	for _, components := range i.installQueue {
		ret = ret.Insert(components.Images()...)
	}
	return sets.List(ret)
}

func newProviderInstaller(configClient config.Client, repositoryClientFactory RepositoryClientFactory, proxy Proxy, providerMetadata InventoryClient, providerComponents ComponentsClient) *providerInstaller {
	return &providerInstaller{
		configClient:            configClient,
		repositoryClientFactory: repositoryClientFactory,
		proxy:                   proxy,
		providerComponents:      providerComponents,
		providerInventory:       providerMetadata,
	}
}
