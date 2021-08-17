package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	versionutil "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	configclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	"sigs.k8s.io/cluster-api/exp/operator/api/v1alpha1"
	operatorv1 "sigs.k8s.io/cluster-api/exp/operator/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/operator/controllers/genericprovider"
	"sigs.k8s.io/cluster-api/exp/operator/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

type reconciler struct {
	ctrlClient           client.Client
	certManagerInstaller SingletonInstaller

	repo               repository.Repository
	contract           string
	options            repository.ComponentsOptions
	providerConfig     configclient.Provider
	configClient       configclient.Client
	components         repository.Components
	clusterctlProvider *clusterctlv1.Provider
}

type reconcilePhaseFn func(context.Context, genericprovider.GenericProvider) (reconcile.Result, error)

type PhaseError struct {
	Reason   string
	Type     clusterv1.ConditionType
	Severity clusterv1.ConditionSeverity
	Err      error
}

func (s *PhaseError) Error() string {
	return s.Err.Error()
}

func ifErrorWrapPhaseError(err error, reason string, ctype clusterv1.ConditionType) error {
	if err == nil {
		return nil
	}
	return &PhaseError{
		Err:      err,
		Type:     ctype,
		Reason:   reason,
		Severity: clusterv1.ConditionSeverityWarning,
	}
}

func newReconcilePhases(c client.Client, certManagerInstaller SingletonInstaller) *reconciler {
	return &reconciler{
		ctrlClient:           c,
		certManagerInstaller: certManagerInstaller,
		clusterctlProvider:   &clusterctlv1.Provider{},
	}
}

func (s *reconciler) load(ctx context.Context, provider genericprovider.GenericProvider) (reconcile.Result, error) {
	reader, err := s.secretReader(ctx, provider)
	if err != nil {
		return reconcile.Result{}, ifErrorWrapPhaseError(err, "failed to load the secret reader", v1alpha1.PreflightCheckCondition)
	}

	s.configClient, err = configclient.New("", configclient.InjectReader(reader))
	if err != nil {
		return reconcile.Result{}, err
	}

	s.providerConfig, err = s.configClient.Providers().Get(provider.GetName(), util.ClusterctlProviderType(provider))
	if err != nil {
		return reconcile.Result{}, ifErrorWrapPhaseError(err, operatorv1.UnknownProviderReason, v1alpha1.PreflightCheckCondition)
	}

	spec := provider.GetSpec()

	if spec.FetchConfig != nil && spec.FetchConfig.Selector != nil {
		s.repo, err = s.configmapRepository(ctx, provider)
	} else {
		s.repo, err = repository.NewGitHubRepository(s.providerConfig, s.configClient.Variables())
	}
	if err != nil {
		return reconcile.Result{}, ifErrorWrapPhaseError(err, "failed to load the repository", v1alpha1.PreflightCheckCondition)
	}

	s.options = repository.ComponentsOptions{
		TargetNamespace:     provider.GetNamespace(),
		SkipTemplateProcess: false,
		Version:             s.repo.DefaultVersion(),
	}
	if spec.Version != nil {
		s.options.Version = *spec.Version
	}

	err = s.validateRepoCAPIVersion(provider)
	return reconcile.Result{}, ifErrorWrapPhaseError(err, operatorv1.CAPIVersionIncompatibilityReason, v1alpha1.PreflightCheckCondition)
}

func (s *reconciler) fetch(ctx context.Context, provider genericprovider.GenericProvider) (reconcile.Result, error) {
	componentsFile, err := s.repo.GetFile(s.options.Version, s.repo.ComponentsPath())
	if err != nil {
		err = errors.Wrapf(err, "failed to read %q from provider's repository %q", s.repo.ComponentsPath(), s.providerConfig.ManifestLabel())
		return reconcile.Result{}, ifErrorWrapPhaseError(err, operatorv1.ComponentsFetchErrorReason, v1alpha1.PreflightCheckCondition)
	}

	s.components, err = repository.NewComponents(repository.ComponentsInput{
		Provider:     s.providerConfig,
		ConfigClient: s.configClient,
		Processor:    yamlprocessor.NewSimpleProcessor(),
		RawYaml:      componentsFile,
		ObjModifier:  customizeObjectsFn(provider),
		Options:      s.options})
	if err != nil {
		return reconcile.Result{}, ifErrorWrapPhaseError(err, operatorv1.ComponentsFetchErrorReason, v1alpha1.PreflightCheckCondition)
	}
	conditions.Set(provider, conditions.TrueCondition(operatorv1.PreflightCheckCondition))
	return reconcile.Result{}, nil
}

// preInstall will
// 1. ensure basic CRDs.
// 2. delete existing components if required.
func (s *reconciler) preInstall(ctx context.Context, provider genericprovider.GenericProvider) (reconcile.Result, error) {
	clusterClient := cluster.New(cluster.Kubeconfig{}, s.configClient)

	err := clusterClient.ProviderInventory().EnsureCustomResourceDefinitions()
	if err != nil {
		return reconcile.Result{}, ifErrorWrapPhaseError(err, "failed installing clusterctl CRDs", v1alpha1.ProviderInstalledCondition)
	}

	needPreDelete, err := s.updateRequiresPreDeletion(ctx, provider)
	if err != nil || !needPreDelete {
		return reconcile.Result{}, ifErrorWrapPhaseError(err, "failed getting clusterctl Provider", v1alpha1.ProviderInstalledCondition)
	}
	return s.delete(ctx, provider)
}

func (s *reconciler) delete(_ context.Context, provider genericprovider.GenericProvider) (reconcile.Result, error) {
	clusterClient := cluster.New(cluster.Kubeconfig{}, s.configClient)

	s.clusterctlProvider.Name = clusterctlProviderName(provider).Name
	s.clusterctlProvider.Namespace = provider.GetNamespace()
	s.clusterctlProvider.Type = string(util.ClusterctlProviderType(provider))
	s.clusterctlProvider.ProviderName = provider.GetName()
	if s.clusterctlProvider.Version == "" {
		// fake these values to get the delete working in case there is not
		// a real provider (perhaps a failed install).
		s.clusterctlProvider.Version = s.options.Version
	}

	err := clusterClient.ProviderComponents().Delete(cluster.DeleteOptions{
		Provider:         *s.clusterctlProvider,
		IncludeNamespace: false,
		IncludeCRDs:      false,
	})
	return reconcile.Result{}, ifErrorWrapPhaseError(err, operatorv1.OldComponentsDeletionErrorReason, v1alpha1.ProviderInstalledCondition)
}

// updateRequiresPreDeletion get the clusterctlProvider and compare it's version with the Spec.Version
// if different, it's an upgrade.
func (s *reconciler) updateRequiresPreDeletion(ctx context.Context, provider genericprovider.GenericProvider) (bool, error) {
	// TODO: We should replace this with an Installed/Applied version in the providerStatus.
	err := s.ctrlClient.Get(ctx, clusterctlProviderName(provider), s.clusterctlProvider)
	if apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if s.clusterctlProvider.Version == "" {
		return false, nil
	}
	nextVersion := versionutil.MustParseSemantic(s.components.Version())
	return versionutil.MustParseSemantic(s.clusterctlProvider.Version).LessThan(nextVersion), nil
}

func clusterctlProviderName(provider genericprovider.GenericProvider) client.ObjectKey {
	prefix := ""
	switch provider.GetObject().(type) {
	case *operatorv1.BootstrapProvider:
		prefix = "bootstrap-"
	case *operatorv1.ControlPlaneProvider:
		prefix = "control-plane-"
	case *operatorv1.InfrastructureProvider:
		prefix = "infrastructure-"
	}

	return client.ObjectKey{Name: prefix + provider.GetName(), Namespace: provider.GetNamespace()}
}

func (s *reconciler) installCertManager(ctx context.Context, provider genericprovider.GenericProvider) (reconcile.Result, error) {
	if s.isCertManagerRequired(ctx) {
		clusterClient := cluster.New(cluster.Kubeconfig{}, s.configClient)
		status, err := s.certManagerInstaller.Install(func() error {
			return clusterClient.CertManager().EnsureInstalled()
		})
		if err != nil || status == InstallStatusUnknown {
			return reconcile.Result{RequeueAfter: time.Minute}, ifErrorWrapPhaseError(err, string(status), v1alpha1.CertManagerReadyCondition)
		}
		if status == InstallStatusInstalling {
			conditions.Set(provider, conditions.FalseCondition(operatorv1.CertManagerReadyCondition, string(status), clusterv1.ConditionSeverityInfo, "Busy Installing"))
			return reconcile.Result{RequeueAfter: time.Minute}, err
		}
	}
	conditions.Set(provider, conditions.TrueCondition(operatorv1.CertManagerReadyCondition))
	return reconcile.Result{}, nil
}

func (s *reconciler) isCertManagerRequired(ctx context.Context) bool {
	log := ctrl.LoggerFrom(ctx)

	for _, obj := range s.components.Objs() {
		if strings.Contains(obj.GetAPIVersion(), "cert-manager.io/") {
			log.V(2).Info("cert-manager is required by", "kind", obj.GetKind(), "namespace", obj.GetNamespace(), "name", obj.GetName())
			return true
		}
	}
	log.V(2).Info("cert-manager not required")
	return false
}

func (s *reconciler) install(ctx context.Context, provider genericprovider.GenericProvider) (reconcile.Result, error) {
	clusterClient := cluster.New(cluster.Kubeconfig{}, s.configClient)
	installer := clusterClient.ProviderInstaller()
	installer.Add(s.components)

	_, err := installer.Install(cluster.InstallOptions{})
	if err != nil {
		reason := "Install failed"
		if err == wait.ErrWaitTimeout {
			reason = "Timedout waiting for deployment to become ready"
		}
		return reconcile.Result{}, ifErrorWrapPhaseError(err, reason, v1alpha1.ProviderInstalledCondition)
	}

	spec := provider.GetSpec()
	if spec.Version == nil {
		// the proposal says to do this.. but it causes the Generation to bump
		// and thus a repeated Reconcile(). IMHO I think this should really be
		// "status.targetVersion" and then we also have "status.installedVersion" so
		// we can see what we are working towards without having to edit the spec.
		spec.Version = pointer.StringPtr(s.components.Version())
		provider.SetSpec(spec)
	}
	status := provider.GetStatus()
	status.Contract = &s.contract
	status.ObservedGeneration = provider.GetGeneration()
	provider.SetStatus(status)

	conditions.Set(provider, conditions.TrueCondition(operatorv1.ProviderInstalledCondition))
	return reconcile.Result{}, nil
}

func (s *reconciler) validateRepoCAPIVersion(provider genericprovider.GenericProvider) error {
	name := provider.GetName()
	file, err := s.repo.GetFile(s.options.Version, metadataFile)
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
	currentVersion, err := versionutil.ParseSemantic(s.options.Version)
	if err != nil {
		return errors.Wrapf(err, "failed to parse current version for the %s provider", name)
	}

	releaseSeries := latestMetadata.GetReleaseSeriesForVersion(currentVersion)
	if releaseSeries == nil {
		return errors.Errorf("invalid provider metadata: version %s for the provider %s does not match any release series", s.options.Version, name)
	}
	if releaseSeries.Contract != clusterv1.GroupVersion.Version {
		return errors.Errorf(capiVersionIncompatibilityMessage, clusterv1.GroupVersion.Version, releaseSeries.Contract, name)
	}
	s.contract = releaseSeries.Contract
	return nil
}

func (s *reconciler) secretReader(ctx context.Context, provider genericprovider.GenericProvider) (configclient.Reader, error) {
	log := ctrl.LoggerFrom(ctx)

	mr := configclient.NewMemoryReader()
	err := mr.Init("")
	if err != nil {
		return nil, err
	}

	if provider.GetSpec().SecretName != nil {
		secret := &corev1.Secret{}
		key := types.NamespacedName{Namespace: provider.GetNamespace(), Name: *provider.GetSpec().SecretName}
		if err := s.ctrlClient.Get(ctx, key, secret); err != nil {
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

func (s *reconciler) configmapRepository(ctx context.Context, provider genericprovider.GenericProvider) (repository.Repository, error) {
	mr := repository.NewMemoryRepository()
	mr.WithPaths("", "components.yaml")

	cml := &corev1.ConfigMapList{}
	selector, err := metav1.LabelSelectorAsSelector(provider.GetSpec().FetchConfig.Selector)
	if err != nil {
		return nil, err
	}
	err = s.ctrlClient.List(ctx, cml, &client.ListOptions{LabelSelector: selector})
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
