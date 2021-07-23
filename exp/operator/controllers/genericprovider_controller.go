/*
Copyright 2021 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	configclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	clusterctllog "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	operatorv1 "sigs.k8s.io/cluster-api/exp/operator/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/operator/controllers/genericprovider"
	"sigs.k8s.io/cluster-api/exp/operator/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
)

const metadataFile = "metadata.yaml"

// GenericProviderReconciler implements the controller.Reconciler interface.
type GenericProviderReconciler struct {
	Provider     client.Object
	ProviderList client.ObjectList
	Client       client.Client
}

func (r *GenericProviderReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	clusterctllog.SetLogger(mgr.GetLogger())
	return ctrl.NewControllerManagedBy(mgr).
		For(r.Provider).
		WithOptions(options).
		Complete(r)
}

func (r *GenericProviderReconciler) Reconcile(ctx context.Context, req reconcile.Request) (_ reconcile.Result, reterr error) {
	typedProvider, err := r.newGenericProvider()
	if err != nil {
		return ctrl.Result{}, err
	}

	typedProviderList, err := r.NewGenericProviderList()
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Client.Get(ctx, req.NamespacedName, typedProvider.GetObject()); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(typedProvider.GetObject(), r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to patch the object and status after each reconciliation.
		if err := patchHelper.Patch(ctx, typedProvider.GetObject()); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Ignore deleted provider, this can happen when foregroundDeletion
	// is enabled
	// Cleanup logic is not needed because owner references set on resource created by
	// Provider will cause GC to do the cleanup for us.
	if !typedProvider.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	return r.reconcile(ctx, typedProvider, typedProviderList)
}

func (r *GenericProviderReconciler) reconcile(ctx context.Context, provider genericprovider.GenericProvider, genericProviderList genericprovider.GenericProviderList) (_ ctrl.Result, reterr error) {
	// Run preflight checks to ensure that core provider can be installed properly
	result, err := preflightChecks(ctx, r.Client, provider, genericProviderList)
	if err != nil || !result.IsZero() {
		return result, err
	}

	// TODO how much of the stuff below should we add to preflight checks?
	// 1. if we add it, we will have to rerun much of the code again
	// 2. unit testing is going to be a pain
	reader, err := r.secretReader(ctx, provider)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfg, err := configclient.New("", configclient.InjectReader(reader))
	if err != nil {
		return ctrl.Result{}, err
	}

	providerConfig, err := cfg.Providers().Get(provider.GetName(), util.ClusterctlProviderType(provider))
	if err != nil {
		conditions.Set(provider, conditions.FalseCondition(
			operatorv1.PreflightCheckCondition,
			operatorv1.UnknownProviderReason,
			clusterv1.ConditionSeverityWarning,
			fmt.Sprintf(unknownProviderMessage, provider.GetName()),
		))
		// we don't want to reconcile again until the CR has been fixed.
		// TODO although, not the user could have fixed the secret...
		// 1. add annotation to the secret
		// 2. watch secrets and map the secret back to the provider?
		return ctrl.Result{}, nil // nolint:nilerr
	}

	spec := provider.GetSpec()

	var repo repository.Repository
	if spec.FetchConfig != nil && spec.FetchConfig.Selector != nil {
		repo, err = r.configmapRepository(ctx, provider)
	} else {
		repo, err = repository.NewGitHubRepository(providerConfig, cfg.Variables())
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	options := repository.ComponentsOptions{
		TargetNamespace: provider.GetNamespace(),
		Version:         repo.DefaultVersion(),
	}
	if spec.Version != nil {
		options.Version = *spec.Version
	}

	err = validateRepoCAPIVersion(repo, provider.GetName(), options.Version)
	if err != nil {
		conditions.Set(provider, conditions.FalseCondition(
			operatorv1.PreflightCheckCondition,
			operatorv1.CAPIVersionIncompatibilityReason,
			clusterv1.ConditionSeverityWarning,
			err.Error(),
		))
		return ctrl.Result{}, err
	}

	componentsFile, err := repo.GetFile(options.Version, repo.ComponentsPath())
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to read %q from provider's repository %q", repo.ComponentsPath(), providerConfig.ManifestLabel())
	}

	components, err := repository.NewComponents(repository.ComponentsInput{
		Provider:     providerConfig,
		ConfigClient: cfg,
		Processor:    yamlprocessor.NewSimpleProcessor(),
		RawYaml:      componentsFile,
		ObjModifier:  customizeObjectsFn(provider),
		Options:      options})
	if err != nil {
		conditions.Set(provider, conditions.FalseCondition(
			clusterv1.ReadyCondition,
			operatorv1.ComponentsFetchErrorReason,
			clusterv1.ConditionSeverityWarning,
			err.Error(),
		))

		return ctrl.Result{}, err
	}

	clusterClient := cluster.New(cluster.Kubeconfig{}, cfg)
	installer := clusterClient.ProviderInstaller()
	installer.Add(components)

	// ensure the custom resource definitions required by clusterctl are in place
	if err := clusterClient.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return ctrl.Result{}, err
	}

	if isCertManagerRequired(ctx, components) {
		// TODO: maybe set a shorter default, so we don't block when installing cert-manager.
		// reader.Set("cert-manager-timeout", "120s")
		if err := clusterClient.CertManager().EnsureInstalled(); err != nil {
			return ctrl.Result{}, err
		}
	}

	_, err = installer.Install(cluster.InstallOptions{
		WaitProviders:       true,
		WaitProviderTimeout: 5 * time.Minute,
	})
	if err != nil {
		conditions.Set(provider, conditions.FalseCondition(
			clusterv1.ReadyCondition,
			"Install failed",
			clusterv1.ConditionSeverityError,
			err.Error(),
		))
		return ctrl.Result{}, err
	}

	conditions.Set(provider, conditions.TrueCondition(clusterv1.ReadyCondition))
	return ctrl.Result{}, nil
}

func validateRepoCAPIVersion(repo repository.Repository, name, ver string) error {
	file, err := repo.GetFile(ver, metadataFile)
	if err != nil {
		return errors.Wrapf(err, "failed to read %q from the repository for provider %q", metadataFile, name)
	}

	// Convert the yaml into a typed object
	latestMetadata := &clusterctlv1.Metadata{}
	codecFactory := serializer.NewCodecFactory(scheme.Scheme)

	if err := runtime.DecodeInto(codecFactory.UniversalDecoder(), file, latestMetadata); err != nil {
		return errors.Wrapf(err, "error decoding %q for provider %q", metadataFile, name)
	}

	// Gets the contract for the current release.
	currentVersion, err := version.ParseSemantic(ver)
	if err != nil {
		return errors.Wrapf(err, "failed to parse current version for the %s provider", name)
	}

	releaseSeries := latestMetadata.GetReleaseSeriesForVersion(currentVersion)
	if releaseSeries == nil {
		return errors.Errorf("invalid provider metadata: version %s for the provider %s does not match any release series", ver, name)
	}

	if releaseSeries.Contract != clusterv1.GroupVersion.Version {
		return errors.Errorf(capiVersionIncompatibilityMessage, clusterv1.GroupVersion.Version, releaseSeries.Contract, name)
	}
	return nil
}

func (r *GenericProviderReconciler) secretReader(ctx context.Context, provider genericprovider.GenericProvider) (configclient.Reader, error) {
	log := ctrl.LoggerFrom(ctx)

	mr := configclient.NewMemoryReader()
	err := mr.Init("")
	if err != nil {
		return nil, err
	}

	if provider.GetSpec().SecretName != nil {
		secret := &corev1.Secret{}
		key := types.NamespacedName{Namespace: provider.GetNamespace(), Name: *provider.GetSpec().SecretName}
		if err := r.Client.Get(ctx, key, secret); err != nil {
			return nil, err
		}
		for k, v := range secret.Data {
			mr.Set(k, string(v))
		}
	} else {
		log.Info("no configuration secret was specified")
	}

	if provider.GetSpec().FetchConfig != nil && provider.GetSpec().FetchConfig.URL != nil {
		return mr.AddProvider(provider.GetName(), util.ClusterctlProviderType(provider), *provider.GetSpec().FetchConfig.URL)
	}

	return mr, nil
}

func (r *GenericProviderReconciler) configmapRepository(ctx context.Context, provider genericprovider.GenericProvider) (repository.Repository, error) {
	mr := repository.NewMemoryRepository()
	mr.WithPaths("", "components.yaml")

	cml := &corev1.ConfigMapList{}
	selector, err := metav1.LabelSelectorAsSelector(provider.GetSpec().FetchConfig.Selector)
	if err != nil {
		return nil, err
	}
	err = r.Client.List(ctx, cml, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if len(cml.Items) == 0 {
		return nil, fmt.Errorf("no ConfigMaps found with selector %s", provider.GetSpec().FetchConfig.Selector.String())
	}
	for _, cm := range cml.Items {
		metadata, ok := cm.Data["metadata"]
		if !ok {
			return nil, fmt.Errorf("ConfigMap %s/%s has no metadata", cm.Namespace, cm.Name)
		}
		mr.WithFile(cm.Name, metadataFile, []byte(metadata))
		components, ok := cm.Data["components"]
		if !ok {
			return nil, fmt.Errorf("ConfigMap %s/%s has no components", cm.Namespace, cm.Name)
		}
		mr.WithFile(cm.Name, mr.ComponentsPath(), []byte(components))
	}

	return mr, nil
}

func isCertManagerRequired(ctx context.Context, components repository.Components) bool {
	log := ctrl.LoggerFrom(ctx)

	for _, obj := range components.Objs() {
		if strings.Contains(obj.GetAPIVersion(), "cert-manager.io/") {
			log.V(2).Info("cert-manager is required by", "kind", obj.GetKind(), "namespace", obj.GetNamespace(), "name", obj.GetName())
			return true
		}
	}
	log.V(2).Info("cert-manager not required")
	return false
}

func (r *GenericProviderReconciler) newGenericProvider() (genericprovider.GenericProvider, error) {
	switch r.Provider.(type) {
	case *operatorv1.CoreProvider:
		return &genericprovider.CoreProviderWrapper{CoreProvider: &operatorv1.CoreProvider{}}, nil
	case *operatorv1.BootstrapProvider:
		return &genericprovider.BootstrapProviderWrapper{BootstrapProvider: &operatorv1.BootstrapProvider{}}, nil
	case *operatorv1.ControlPlaneProvider:
		return &genericprovider.ControlPlaneProviderWrapper{ControlPlaneProvider: &operatorv1.ControlPlaneProvider{}}, nil
	case *operatorv1.InfrastructureProvider:
		return &genericprovider.InfrastructureProviderWrapper{InfrastructureProvider: &operatorv1.InfrastructureProvider{}}, nil
	default:
		providerKind := reflect.Indirect(reflect.ValueOf(r.Provider)).Type().Name()
		failedToCastInterfaceErr := fmt.Errorf("failed to cast interface for type: %s", providerKind)
		return nil, failedToCastInterfaceErr
	}
}

func (r *GenericProviderReconciler) NewGenericProviderList() (genericprovider.GenericProviderList, error) {
	switch r.ProviderList.(type) {
	case *operatorv1.CoreProviderList:
		return &genericprovider.CoreProviderListWrapper{CoreProviderList: &operatorv1.CoreProviderList{}}, nil
	case *operatorv1.BootstrapProviderList:
		return &genericprovider.BootstrapProviderListWrapper{BootstrapProviderList: &operatorv1.BootstrapProviderList{}}, nil
	case *operatorv1.ControlPlaneProviderList:
		return &genericprovider.ControlPlaneProviderListWrapper{ControlPlaneProviderList: &operatorv1.ControlPlaneProviderList{}}, nil
	case *operatorv1.InfrastructureProviderList:
		return &genericprovider.InfrastructureProviderListWrapper{InfrastructureProviderList: &operatorv1.InfrastructureProviderList{}}, nil
	default:
		providerKind := reflect.Indirect(reflect.ValueOf(r.ProviderList)).Type().Name()
		failedToCastInterfaceErr := fmt.Errorf("failed to cast interface for type: %s", providerKind)
		return nil, failedToCastInterfaceErr
	}
}
