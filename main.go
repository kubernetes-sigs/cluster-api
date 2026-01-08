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

// main is the main package for the Cluster API Core Provider.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	goruntime "runtime"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	addonsv1beta1 "sigs.k8s.io/cluster-api/api/addons/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	ipamv1alpha1 "sigs.k8s.io/cluster-api/api/ipam/v1alpha1"
	ipamv1beta1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1alpha1 "sigs.k8s.io/cluster-api/api/runtime/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/controllers"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/crdmigrator"
	"sigs.k8s.io/cluster-api/controllers/remote"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	internalruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	"sigs.k8s.io/cluster-api/util/apiwarnings"
	"sigs.k8s.io/cluster-api/util/flags"
	"sigs.k8s.io/cluster-api/version"
	"sigs.k8s.io/cluster-api/webhooks"
)

var (
	catalog        = runtimecatalog.New()
	scheme         = runtime.NewScheme()
	setupLog       = ctrl.Log.WithName("setup")
	controllerName = "cluster-api-controller-manager"

	// flags.
	enableLeaderElection        bool
	leaderElectionLeaseDuration time.Duration
	leaderElectionRenewDeadline time.Duration
	leaderElectionRetryPeriod   time.Duration
	watchFilterValue            string
	watchNamespace              string
	profilerAddress             string
	enableContentionProfiling   bool
	syncPeriod                  time.Duration
	restConfigQPS               float32
	restConfigBurst             int
	clusterCacheClientQPS       float32
	clusterCacheClientBurst     int
	webhookPort                 int
	webhookCertDir              string
	webhookCertName             string
	webhookKeyName              string
	runtimeExtensionCertFile    string
	runtimeExtensionKeyFile     string
	healthAddr                  string
	managerOptions              = flags.ManagerOptions{}
	logOptions                  = logs.NewOptions()
	// core Cluster API specific flags.
	remoteConnectionGracePeriod      time.Duration
	remoteConditionsGracePeriod      time.Duration
	clusterTopologyConcurrency       int
	clusterCacheConcurrency          int
	clusterClassConcurrency          int
	clusterConcurrency               int
	extensionConfigConcurrency       int
	machineConcurrency               int
	machineSetConcurrency            int
	machineDeploymentConcurrency     int
	machinePoolConcurrency           int
	clusterResourceSetConcurrency    int
	machineHealthCheckConcurrency    int
	machineSetPreflightChecks        []string
	skipCRDMigrationPhases           []string
	additionalSyncMachineLabels      []string
	additionalSyncMachineAnnotations []string
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)

	_ = clusterv1beta1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	_ = addonsv1beta1.AddToScheme(scheme)
	_ = addonsv1.AddToScheme(scheme)

	_ = runtimev1alpha1.AddToScheme(scheme)
	_ = runtimev1.AddToScheme(scheme)

	_ = ipamv1alpha1.AddToScheme(scheme)
	_ = ipamv1beta1.AddToScheme(scheme)
	_ = ipamv1.AddToScheme(scheme)

	// Register the RuntimeHook types into the catalog.
	_ = runtimehooksv1.AddToCatalog(catalog)
}

// InitFlags initializes the flags.
func InitFlags(fs *pflag.FlagSet) {
	logsv1.AddFlags(logOptions, fs)

	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")

	fs.DurationVar(&leaderElectionLeaseDuration, "leader-elect-lease-duration", 15*time.Second,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")

	fs.DurationVar(&leaderElectionRenewDeadline, "leader-elect-renew-deadline", 10*time.Second,
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)")

	fs.DurationVar(&leaderElectionRetryPeriod, "leader-elect-retry-period", 2*time.Second,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")

	fs.StringVar(&watchNamespace, "namespace", "",
		"Namespace that the controller watches to reconcile cluster-api objects. If unspecified, the controller watches for cluster-api objects across all namespaces.")

	fs.StringVar(&watchFilterValue, "watch-filter", "",
		fmt.Sprintf("Label value that the controller watches to reconcile cluster-api objects. Label key is always %s. If unspecified, the controller watches for all cluster-api objects.", clusterv1.WatchLabel))

	fs.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")

	fs.BoolVar(&enableContentionProfiling, "contention-profiling", false,
		"Enable block profiling")

	fs.DurationVar(&remoteConnectionGracePeriod, "remote-connection-grace-period", 50*time.Second,
		"Grace period after which the RemoteConnectionProbe condition on a Cluster goes to `False`, "+
			"the grace period starts from the last successful health probe to the workload cluster")

	fs.DurationVar(&remoteConditionsGracePeriod, "remote-conditions-grace-period", 5*time.Minute,
		"Grace period after which remote conditions (e.g. `NodeHealthy`) are set to `Unknown`, "+
			"the grace period starts from the last successful health probe to the workload cluster")

	fs.IntVar(&clusterTopologyConcurrency, "clustertopology-concurrency", 10,
		"Number of clusters to process simultaneously")

	fs.IntVar(&clusterClassConcurrency, "clusterclass-concurrency", 10,
		"Number of ClusterClasses to process simultaneously")

	fs.IntVar(&clusterConcurrency, "cluster-concurrency", 10,
		"Number of clusters to process simultaneously")

	fs.IntVar(&clusterCacheConcurrency, "clustercache-concurrency", 100,
		"Number of clusters to process simultaneously")

	fs.IntVar(&extensionConfigConcurrency, "extensionconfig-concurrency", 10,
		"Number of extension configs to process simultaneously")

	fs.IntVar(&machineConcurrency, "machine-concurrency", 10,
		"Number of machines to process simultaneously")

	fs.IntVar(&machineSetConcurrency, "machineset-concurrency", 10,
		"Number of machine sets to process simultaneously")

	fs.IntVar(&machineDeploymentConcurrency, "machinedeployment-concurrency", 10,
		"Number of machine deployments to process simultaneously")

	fs.IntVar(&machinePoolConcurrency, "machinepool-concurrency", 10,
		"Number of machine pools to process simultaneously")

	fs.IntVar(&clusterResourceSetConcurrency, "clusterresourceset-concurrency", 10,
		"Number of cluster resource sets to process simultaneously")

	fs.IntVar(&machineHealthCheckConcurrency, "machinehealthcheck-concurrency", 10,
		"Number of machine health checks to process simultaneously")

	fs.StringSliceVar(&machineSetPreflightChecks, "machineset-preflight-checks", []string{
		string(clusterv1.MachineSetPreflightCheckAll)},
		"List of MachineSet preflight checks that should be run. Per default all of them are enabled."+
			"Set this flag to only enable a subset of them. The MachineSet preflight checks can be then also disabled"+
			"on MachineSets via the 'machineset.cluster.x-k8s.io/skip-preflight-checks' annotation."+
			"Valid values are: All or a list of KubeadmVersionSkew, KubernetesVersionSkew, ControlPlaneIsStable, ControlPlaneVersionSkew")

	fs.StringSliceVar(&skipCRDMigrationPhases, "skip-crd-migration-phases", []string{},
		"List of CRD migration phases to skip. Valid values are: StorageVersionMigration, CleanupManagedFields.")

	fs.DurationVar(&syncPeriod, "sync-period", 10*time.Minute,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")

	fs.Float32Var(&restConfigQPS, "kube-api-qps", 20,
		"Maximum queries per second from the controller client to the Kubernetes API server.")

	fs.IntVar(&restConfigBurst, "kube-api-burst", 30,
		"Maximum number of queries that should be allowed in one burst from the controller client to the Kubernetes API server.")

	fs.Float32Var(&clusterCacheClientQPS, "clustercache-client-qps", 20,
		"Maximum queries per second from the cluster cache clients to the Kubernetes API server of workload clusters.")

	fs.IntVar(&clusterCacheClientBurst, "clustercache-client-burst", 30,
		"Maximum number of queries that should be allowed in one burst from the cluster cache clients to the Kubernetes API server of workload clusters.")

	fs.IntVar(&webhookPort, "webhook-port", 9443,
		"Webhook Server port")

	fs.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs/",
		"Webhook cert dir.")

	fs.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt",
		"Name of the file for webhook's server certificate; the file must be placed under webhook-cert-dir.")

	fs.StringVar(&webhookKeyName, "webhook-key-name", "tls.key",
		"Name of the file for webhook's server key; the file must be placed under webhook-cert-dir.")

	fs.StringVar(&runtimeExtensionCertFile, "runtime-extension-client-cert-file", "",
		"Path of the PEM-encoded client certificate to be used when calling runtime extensions.")

	fs.StringVar(&runtimeExtensionKeyFile, "runtime-extension-client-key-file", "",
		"Path of the PEM-encoded client key to be used when calling runtime extensions.")

	fs.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")

	fs.StringSliceVar(&additionalSyncMachineLabels, "additional-sync-machine-labels", []string{},
		"List of regexes to select an additional set of labels to sync from a Machine to its associated Node. A label will be synced as long as it matches at least one of the regexes.")

	fs.StringSliceVar(&additionalSyncMachineAnnotations, "additional-sync-machine-annotations", []string{},
		"List of regexes to select an additional set of labels to sync from a Machine to its associated Node. An annotation will be synced as long as it matches at least one of the regexes.")

	flags.AddManagerOptions(fs, &managerOptions)

	feature.MutableGates.AddFlag(fs)
}

// Add RBAC for the authorized diagnostics endpoint.
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// ADD CRD RBAC for CRD Migrator.
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions;customresourcedefinitions/status,verbs=update;patch,resourceNames=clusterclasses.cluster.x-k8s.io;clusterresourcesetbindings.addons.cluster.x-k8s.io;clusterresourcesets.addons.cluster.x-k8s.io;clusters.cluster.x-k8s.io;extensionconfigs.runtime.cluster.x-k8s.io;ipaddressclaims.ipam.cluster.x-k8s.io;ipaddresses.ipam.cluster.x-k8s.io;machinedeployments.cluster.x-k8s.io;machinedrainrules.cluster.x-k8s.io;machinehealthchecks.cluster.x-k8s.io;machinepools.cluster.x-k8s.io;machines.cluster.x-k8s.io;machinesets.cluster.x-k8s.io
// ADD CR RBAC for CRD Migrator.
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddresses;ipaddressclaims,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/status,verbs=patch;update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedrainrules,verbs=get;list;watch;patch;update

func main() {
	InitFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	// Set log level 2 as default.
	if err := pflag.CommandLine.Set("v", "2"); err != nil {
		fmt.Printf("Failed to set default log level: %v\n", err)
		os.Exit(1)
	}
	pflag.Parse()

	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		fmt.Printf("Unable to start manager: %v\n", err)
		os.Exit(1)
	}

	// klog.Background will automatically use the right logger.
	ctrl.SetLogger(klog.Background())

	// Note: setupLog can only be used after ctrl.SetLogger was called
	setupLog.Info(fmt.Sprintf("Version: %s (git commit: %s)", version.Get().String(), version.Get().GitCommit))

	restConfig := ctrl.GetConfigOrDie()
	restConfig.QPS = restConfigQPS
	restConfig.Burst = restConfigBurst
	restConfig.UserAgent = remote.DefaultClusterAPIUserAgent(controllerName)
	restConfig.WarningHandler = apiwarnings.DefaultHandler(klog.Background().WithName("API Server Warning"))

	minVer := version.MinimumKubernetesVersion
	if feature.Gates.Enabled(feature.ClusterTopology) {
		minVer = version.MinimumKubernetesVersionClusterTopology
	}

	if remoteConditionsGracePeriod <= remoteConnectionGracePeriod {
		setupLog.Error(errors.Errorf("--remote-conditions-grace-period must be greater than --remote-connection-grace-period"), "Unable to start manager")
		os.Exit(1)
	}
	if remoteConditionsGracePeriod < 2*time.Minute {
		// A minimum of 2m is enforced to ensure the ClusterCache always drops the connection before the grace period is reached.
		// In the worst case the ClusterCache will take FailureThreshold x (Interval + Timeout) = 5x(10s+5s) = 75s to drop a
		// connection. There might be some additional delays in health checking under high load. So we use 2m as a minimum
		// to have some buffer.
		setupLog.Error(errors.Errorf("--remote-conditions-grace-period must be at least 2m"), "Unable to start manager")
		os.Exit(1)
	}

	if err := version.CheckKubernetesVersion(restConfig, minVer); err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	tlsOptions, metricsOptions, err := flags.GetManagerOptions(managerOptions)
	if err != nil {
		setupLog.Error(err, "Unable to start manager: invalid flags")
		os.Exit(1)
	}

	var watchNamespaces map[string]cache.Config
	if watchNamespace != "" {
		watchNamespaces = map[string]cache.Config{
			watchNamespace: {},
		}
	}

	if enableContentionProfiling {
		goruntime.SetBlockProfileRate(1)
	}

	req, _ := labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Exists, nil)
	clusterSecretCacheSelector := labels.NewSelector().Add(*req)

	ctrlOptions := ctrl.Options{
		Controller: config.Controller{
			UsePriorityQueue: ptr.To[bool](feature.Gates.Enabled(feature.PriorityQueue)),
		},
		Scheme:                     scheme,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "controller-leader-election-capi",
		LeaseDuration:              &leaderElectionLeaseDuration,
		RenewDeadline:              &leaderElectionRenewDeadline,
		RetryPeriod:                &leaderElectionRetryPeriod,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		HealthProbeBindAddress:     healthAddr,
		PprofBindAddress:           profilerAddress,
		Metrics:                    *metricsOptions,
		Cache: cache.Options{
			DefaultNamespaces: watchNamespaces,
			SyncPeriod:        &syncPeriod,
			ByObject: map[client.Object]cache.ByObject{
				// Note: Only Secrets with the cluster name label are cached.
				// The default client of the manager won't use the cache for secrets at all (see Client.Cache.DisableFor).
				// The cached secrets will only be used by the secretCachingClient we create below.
				&corev1.Secret{}: {
					Label: clusterSecretCacheSelector,
				},
			},
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
				// Use the cache for all Unstructured get/list calls.
				Unstructured: true,
			},
		},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port:     webhookPort,
				CertDir:  webhookCertDir,
				CertName: webhookCertName,
				KeyName:  webhookKeyName,
				TLSOpts:  tlsOptions,
			},
		),
	}

	mgr, err := ctrl.NewManager(restConfig, ctrlOptions)
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupIndexes(ctx, mgr)
	clusterCache := setupReconcilers(ctx, mgr, watchNamespaces, &syncPeriod)
	setupWebhooks(ctx, mgr, clusterCache)

	setupLog.Info("Starting manager", "version", version.Get().String())
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

func setupChecks(mgr ctrl.Manager) {
	if err := mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "Unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "Unable to create health check")
		os.Exit(1)
	}
}

func setupIndexes(ctx context.Context, mgr ctrl.Manager) {
	if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
		setupLog.Error(err, "Unable to setup indexes")
		os.Exit(1)
	}
}

func setupReconcilers(ctx context.Context, mgr ctrl.Manager, watchNamespaces map[string]cache.Config, syncPeriod *time.Duration) clustercache.ClusterCache {
	secretCachingClient, err := client.New(mgr.GetConfig(), client.Options{
		HTTPClient: mgr.GetHTTPClient(),
		Cache: &client.CacheOptions{
			Reader: mgr.GetCache(),
		},
	})
	if err != nil {
		setupLog.Error(err, "Unable to create secret caching client")
		os.Exit(1)
	}

	clusterCache, err := clustercache.SetupWithManager(ctx, mgr, clustercache.Options{
		SecretClient: secretCachingClient,
		Cache: clustercache.CacheOptions{
			Indexes: []clustercache.CacheOptionsIndex{clustercache.NodeProviderIDIndex},
		},
		Client: clustercache.ClientOptions{
			QPS:       clusterCacheClientQPS,
			Burst:     clusterCacheClientBurst,
			UserAgent: remote.DefaultClusterAPIUserAgent(controllerName),
			Cache: clustercache.ClientCacheOptions{
				DisableFor: []client.Object{
					// Don't cache ConfigMaps & Secrets.
					&corev1.ConfigMap{},
					&corev1.Secret{},
					// Don't cache Pods & DaemonSets (we get/list them e.g. during drain).
					&corev1.Pod{},
					&appsv1.DaemonSet{},
					// Don't cache PersistentVolumes and VolumeAttachments (we get/list them e.g. during wait for volumes to detach)
					&storagev1.VolumeAttachment{},
					&corev1.PersistentVolume{},
				},
			},
		},
		WatchFilterValue: watchFilterValue,
	}, concurrency(clusterCacheConcurrency))
	if err != nil {
		setupLog.Error(err, "Unable to create ClusterCache")
		os.Exit(1)
	}

	// Note: The kubebuilder RBAC markers above has to be kept in sync
	// with the CRDs that should be migrated by this provider.
	crdMigratorConfig := map[client.Object]crdmigrator.ByObjectConfig{
		&addonsv1.ClusterResourceSetBinding{}: {UseCache: true},
		&addonsv1.ClusterResourceSet{}:        {UseCache: true, UseStatusForStorageVersionMigration: true},
		&clusterv1.Cluster{}:                  {UseCache: true, UseStatusForStorageVersionMigration: true},
		&clusterv1.MachineDeployment{}:        {UseCache: true, UseStatusForStorageVersionMigration: true},
		&clusterv1.MachineDrainRule{}:         {UseCache: true},
		&clusterv1.MachineHealthCheck{}:       {UseCache: true, UseStatusForStorageVersionMigration: true},
		&clusterv1.Machine{}:                  {UseCache: true, UseStatusForStorageVersionMigration: true},
		&clusterv1.MachineSet{}:               {UseCache: true, UseStatusForStorageVersionMigration: true},
		&ipamv1.IPAddress{}:                   {UseCache: false},
		&ipamv1.IPAddressClaim{}:              {UseCache: false, UseStatusForStorageVersionMigration: true},
	}
	if feature.Gates.Enabled(feature.ClusterTopology) {
		crdMigratorConfig[&clusterv1.ClusterClass{}] = crdmigrator.ByObjectConfig{UseCache: true, UseStatusForStorageVersionMigration: true}
	}
	if feature.Gates.Enabled(feature.RuntimeSDK) {
		crdMigratorConfig[&runtimev1.ExtensionConfig{}] = crdmigrator.ByObjectConfig{UseCache: true, UseStatusForStorageVersionMigration: true}
	}
	if feature.Gates.Enabled(feature.MachinePool) {
		crdMigratorConfig[&clusterv1.MachinePool{}] = crdmigrator.ByObjectConfig{UseCache: true, UseStatusForStorageVersionMigration: true}
	}
	crdMigratorSkipPhases := []crdmigrator.Phase{}
	for _, p := range skipCRDMigrationPhases {
		crdMigratorSkipPhases = append(crdMigratorSkipPhases, crdmigrator.Phase(p))
	}
	if err := (&crdmigrator.CRDMigrator{
		Client:                 mgr.GetClient(),
		APIReader:              mgr.GetAPIReader(),
		SkipCRDMigrationPhases: crdMigratorSkipPhases,
		Config:                 crdMigratorConfig,
		// The CRDMigrator is run with only concurrency 1 to ensure we don't overwhelm the apiserver by patching a
		// lot of CRs concurrently.
	}).SetupWithManager(ctx, mgr, concurrency(1)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "CRDMigrator")
		os.Exit(1)
	}

	var runtimeClient runtimeclient.Client
	if feature.Gates.Enabled(feature.RuntimeSDK) {
		// This is the creation of the runtimeClient for the controllers, embedding a shared catalog and registry instance.
		var certWatcher *certwatcher.CertWatcher
		runtimeClient, certWatcher, err = internalruntimeclient.New(internalruntimeclient.Options{
			CertFile: runtimeExtensionCertFile,
			KeyFile:  runtimeExtensionKeyFile,
			Catalog:  catalog,
			Registry: runtimeregistry.New(),
			Client:   mgr.GetClient(),
		})
		if err != nil {
			setupLog.Error(err, "Unable to create RuntimeSDK client")
			os.Exit(1)
		}
		if certWatcher != nil {
			// Note: certWatcher is managed by the manager to ensure that a certWatcher failure leads to a binary restart.
			if err := mgr.Add(certWatcher); err != nil {
				setupLog.Error(err, "Unable to add RuntimeSDK client cert-watcher to the manager")
				os.Exit(1)
			}
		}
	}

	// Setup a separate cache without label selector for secrets, to be used
	// when we need to watch for secrets that are not specific to a single cluster (e.g. ClusterResourceSet or ExtensionConfig controllers).
	partialSecretCache, err := cache.New(mgr.GetConfig(), cache.Options{
		Scheme:            mgr.GetScheme(),
		Mapper:            mgr.GetRESTMapper(),
		HTTPClient:        mgr.GetHTTPClient(),
		SyncPeriod:        syncPeriod,
		DefaultNamespaces: watchNamespaces,
		DefaultTransform: func(in interface{}) (interface{}, error) {
			// Use DefaultTransform to drop objects we don't expect to get into this cache.
			obj, ok := in.(*metav1.PartialObjectMetadata)
			if !ok {
				panic(fmt.Sprintf("cache expected to only get PartialObjectMetadata, got %T", in))
			}
			if obj.GetObjectKind().GroupVersionKind() != corev1.SchemeGroupVersion.WithKind("Secret") {
				panic(fmt.Sprintf("cache expected to only get Secrets, got %s", obj.GetObjectKind()))
			}
			// Additionally strip managed fields.
			return cache.TransformStripManagedFields()(obj)
		},
	})
	if err != nil {
		setupLog.Error(err, "Failed to create cache for metadata only Secret watches")
		os.Exit(1)
	}
	if err := mgr.Add(partialSecretCache); err != nil {
		setupLog.Error(err, "Failed to start cache for metadata only Secret watches")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.ClusterTopology) {
		if err := (&controllers.ClusterClassReconciler{
			Client:           mgr.GetClient(),
			RuntimeClient:    runtimeClient,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(clusterClassConcurrency)); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "ClusterClass")
			os.Exit(1)
		}

		if err := (&controllers.ClusterTopologyReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			RuntimeClient:    runtimeClient,
			ClusterCache:     clusterCache,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(clusterTopologyConcurrency)); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "ClusterTopology")
			os.Exit(1)
		}

		if err := (&controllers.MachineDeploymentTopologyReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "MachineDeploymentTopology")
			os.Exit(1)
		}

		if err := (&controllers.MachineSetTopologyReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "MachineSetTopology")
			os.Exit(1)
		}
	}

	if feature.Gates.Enabled(feature.RuntimeSDK) {
		if err = (&controllers.ExtensionConfigReconciler{
			Client:             mgr.GetClient(),
			APIReader:          mgr.GetAPIReader(),
			RuntimeClient:      runtimeClient,
			PartialSecretCache: partialSecretCache,
			WatchFilterValue:   watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(extensionConfigConcurrency)); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "ExtensionConfig")
			os.Exit(1)
		}
	}

	if err := (&controllers.ClusterReconciler{
		Client:                      mgr.GetClient(),
		APIReader:                   mgr.GetAPIReader(),
		ClusterCache:                clusterCache,
		WatchFilterValue:            watchFilterValue,
		RemoteConnectionGracePeriod: remoteConnectionGracePeriod,
	}).SetupWithManager(ctx, mgr, concurrency(clusterConcurrency)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Cluster")
		os.Exit(1)
	}

	var errs []error
	var additionalSyncMachineLabelRegexes, additionalSyncMachineAnnotationRegexes []*regexp.Regexp
	for _, re := range additionalSyncMachineLabels {
		reg, err := regexp.Compile(re)
		if err != nil {
			errs = append(errs, err)
		} else {
			additionalSyncMachineLabelRegexes = append(additionalSyncMachineLabelRegexes, reg)
		}
	}
	if len(errs) > 0 {
		setupLog.Error(fmt.Errorf("at least one of --additional-sync-machine-labels regexes is invalid: %w", kerrors.NewAggregate(errs)), "Unable to start manager")
		os.Exit(1)
	}
	for _, re := range additionalSyncMachineAnnotations {
		reg, err := regexp.Compile(re)
		if err != nil {
			errs = append(errs, err)
		} else {
			additionalSyncMachineAnnotationRegexes = append(additionalSyncMachineAnnotationRegexes, reg)
		}
	}
	if len(errs) > 0 {
		setupLog.Error(fmt.Errorf("at least one of --additional-sync-machine-annotations regexes is invalid: %w", kerrors.NewAggregate(errs)), "Unable to start manager")
		os.Exit(1)
	}
	if err := (&controllers.MachineReconciler{
		Client:                           mgr.GetClient(),
		APIReader:                        mgr.GetAPIReader(),
		ClusterCache:                     clusterCache,
		RuntimeClient:                    runtimeClient,
		WatchFilterValue:                 watchFilterValue,
		RemoteConditionsGracePeriod:      remoteConditionsGracePeriod,
		AdditionalSyncMachineLabels:      additionalSyncMachineLabelRegexes,
		AdditionalSyncMachineAnnotations: additionalSyncMachineAnnotationRegexes,
	}).SetupWithManager(ctx, mgr, concurrency(machineConcurrency)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Machine")
		os.Exit(1)
	}
	machineSetPreflightChecksSet := sets.Set[clusterv1.MachineSetPreflightCheck]{}
	supportedMachineSetPreflightChecks := sets.New[clusterv1.MachineSetPreflightCheck](
		clusterv1.MachineSetPreflightCheckAll,
		clusterv1.MachineSetPreflightCheckKubeadmVersionSkew,
		clusterv1.MachineSetPreflightCheckKubernetesVersionSkew,
		clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
		clusterv1.MachineSetPreflightCheckControlPlaneVersionSkew,
	)
	for _, c := range machineSetPreflightChecks {
		if c == "" {
			continue
		}
		preflightCheck := clusterv1.MachineSetPreflightCheck(c)
		if !supportedMachineSetPreflightChecks.Has(preflightCheck) {
			setupLog.Error(err, "Unable to create controller: invalid preflight check configured via --machineset-preflight-checks", "controller", "MachineSet")
			os.Exit(1)
		}
		machineSetPreflightChecksSet.Insert(preflightCheck)
	}
	if err := (&controllers.MachineSetReconciler{
		Client:           mgr.GetClient(),
		APIReader:        mgr.GetAPIReader(),
		ClusterCache:     clusterCache,
		PreflightChecks:  machineSetPreflightChecksSet,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(machineSetConcurrency)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "MachineSet")
		os.Exit(1)
	}
	if err := (&controllers.MachineDeploymentReconciler{
		Client:           mgr.GetClient(),
		APIReader:        mgr.GetAPIReader(),
		RuntimeClient:    runtimeClient,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(machineDeploymentConcurrency)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "MachineDeployment")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&controllers.MachinePoolReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			ClusterCache:     clusterCache,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(machinePoolConcurrency)); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "MachinePool")
			os.Exit(1)
		}
	}

	if err := (&controllers.ClusterResourceSetReconciler{
		Client:           mgr.GetClient(),
		ClusterCache:     clusterCache,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(clusterResourceSetConcurrency), partialSecretCache); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "ClusterResourceSet")
		os.Exit(1)
	}
	if err := (&controllers.ClusterResourceSetBindingReconciler{
		Client:           mgr.GetClient(),
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(clusterResourceSetConcurrency)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "ClusterResourceSetBinding")
		os.Exit(1)
	}

	if err := (&controllers.MachineHealthCheckReconciler{
		Client:           mgr.GetClient(),
		ClusterCache:     clusterCache,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(machineHealthCheckConcurrency)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "MachineHealthCheck")
		os.Exit(1)
	}

	return clusterCache
}

func setupWebhooks(ctx context.Context, mgr ctrl.Manager, clusterCacheReader webhooks.ClusterCacheReader) {
	// Setup the func to retrieve apiVersion for a GroupKind for conversion webhooks.
	apiVersionGetter := func(gk schema.GroupKind) (string, error) {
		return contract.GetAPIVersion(ctx, mgr.GetClient(), gk)
	}
	clusterv1beta1.SetAPIVersionGetter(apiVersionGetter)

	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled.
	if err := (&webhooks.ClusterClass{Client: mgr.GetClient()}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "ClusterClass")
		os.Exit(1)
	}

	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the webhook
	// is going to prevent usage of Cluster.Topology in case the feature flag is disabled.
	if err := (&webhooks.Cluster{Client: mgr.GetClient(), ClusterCacheReader: clusterCacheReader}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "Cluster")
		os.Exit(1)
	}

	if err := (&webhooks.Machine{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "Machine")
		os.Exit(1)
	}

	if err := (&webhooks.MachineSet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "MachineSet")
		os.Exit(1)
	}

	if err := (&webhooks.MachineDeployment{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "MachineDeployment")
		os.Exit(1)
	}

	if err := (&webhooks.MachineDrainRule{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "MachineDrainRule")
		os.Exit(1)
	}

	// NOTE: MachinePool is behind MachinePool feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&webhooks.MachinePool{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "MachinePool")
		os.Exit(1)
	}

	// NOTE: ClusterResourceSet is behind ClusterResourceSet feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&webhooks.ClusterResourceSet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "ClusterResourceSet")
		os.Exit(1)
	}
	// NOTE: ClusterResourceSetBinding is behind ClusterResourceSet feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&webhooks.ClusterResourceSetBinding{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "ClusterResourceSetBinding")
		os.Exit(1)
	}

	if err := (&webhooks.MachineHealthCheck{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "MachineHealthCheck")
		os.Exit(1)
	}

	// NOTE: ExtensionConfig is behind the RuntimeSDK feature gate flag. The webhook will prevent creating or updating
	// new objects if the feature flag is disabled.
	if err := (&webhooks.ExtensionConfig{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "ExtensionConfig")
		os.Exit(1)
	}

	if err := (&webhooks.IPAddress{
		// We are using GetAPIReader here to avoid caching all IPAddressClaims
		Client: mgr.GetAPIReader(),
	}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "IPAddress")
		os.Exit(1)
	}
	if err := (&webhooks.IPAddressClaim{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "IPAddressClaim")
		os.Exit(1)
	}
}

func concurrency(c int) controller.Options {
	return controller.Options{MaxConcurrentReconciles: c}
}
