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

// main is the main package for the Docker Infrastructure Provider.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	goruntime "runtime"
	"time"

	"github.com/spf13/pflag"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/crdmigrator"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1alpha3 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	infrav1alpha4 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha4"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/controllers"
	infraexpv1alpha3 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1alpha3"
	infraexpv1alpha4 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1alpha4"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	expcontrollers "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/controllers"
	infraexpwebhooks "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/webhooks"
	infrawebhooks "sigs.k8s.io/cluster-api/test/infrastructure/docker/webhooks"
	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/cloud/api/v1alpha1"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryserver "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server"
	"sigs.k8s.io/cluster-api/util/flags"
	"sigs.k8s.io/cluster-api/version"
)

var (
	inmemoryScheme = runtime.NewScheme()
	scheme         = runtime.NewScheme()
	setupLog       = ctrl.Log.WithName("setup")
	controllerName = "cluster-api-docker-controller-manager"

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
	healthAddr                  string
	managerOptions              = flags.ManagerOptions{}
	logOptions                  = logs.NewOptions()
	// CAPD specific flags.
	concurrency             int
	clusterCacheConcurrency int
	skipCRDMigrationPhases  string
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = infrav1alpha3.AddToScheme(scheme)
	_ = infrav1alpha4.AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)
	_ = infraexpv1alpha3.AddToScheme(scheme)
	_ = infraexpv1alpha4.AddToScheme(scheme)
	_ = infraexpv1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = expv1.AddToScheme(scheme)

	// scheme used for operating on the cloud resource.
	_ = cloudv1.AddToScheme(inmemoryScheme)
	_ = corev1.AddToScheme(inmemoryScheme)
	_ = appsv1.AddToScheme(inmemoryScheme)
	_ = rbacv1.AddToScheme(inmemoryScheme)
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

	fs.IntVar(&concurrency, "concurrency", 10,
		"The number of docker machines to process simultaneously")

	fs.IntVar(&clusterCacheConcurrency, "clustercache-concurrency", 100,
		"Number of clusters to process simultaneously")

	fs.StringVar(&skipCRDMigrationPhases, "skip-crd-migration-phases", "",
		"Comma-separated list of CRD migration phases to skip. Valid values are: All, StorageVersionMigration, CleanupManagedFields.")

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
		"Webhook cert name.")

	fs.StringVar(&webhookKeyName, "webhook-key-name", "tls.key",
		"Webhook key name.")

	fs.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")

	flags.AddManagerOptions(fs, &managerOptions)

	feature.MutableGates.AddFlag(fs)
}

// Add RBAC for the authorized diagnostics endpoint.
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// ADD RBAC for CRD Migrator.
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions/status,verbs=update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockerclustertemplates;dockermachinetemplates;dockermachinepooltemplates,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=devclustertemplates;devmachinetemplates,verbs=get;list;watch;patch;update

func main() {
	if _, err := os.ReadDir("/tmp/"); err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	InitFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	// Set log level 2 as default.
	if err := pflag.CommandLine.Set("v", "2"); err != nil {
		setupLog.Error(err, "Failed to set default log level")
		os.Exit(1)
	}
	pflag.Parse()

	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	// klog.Background will automatically use the right logger.
	ctrl.SetLogger(klog.Background())

	restConfig := ctrl.GetConfigOrDie()
	restConfig.QPS = restConfigQPS
	restConfig.Burst = restConfigBurst
	restConfig.UserAgent = remote.DefaultClusterAPIUserAgent(controllerName)

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
		LeaderElectionID:           "controller-leader-election-capd",
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
	setupReconcilers(ctx, mgr)
	setupWebhooks(mgr)

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

func setupReconcilers(ctx context.Context, mgr ctrl.Manager) {
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

	// Set our runtime client into the context for later use
	runtimeClient, err := container.NewDockerClient()
	if err != nil {
		setupLog.Error(err, "Unable to establish container runtime connection", "controller", "reconciler")
		os.Exit(1)
	}

	clusterCache, err := clustercache.SetupWithManager(ctx, mgr, clustercache.Options{
		SecretClient: secretCachingClient,
		Cache:        clustercache.CacheOptions{},
		Client: clustercache.ClientOptions{
			QPS:       clusterCacheClientQPS,
			Burst:     clusterCacheClientBurst,
			UserAgent: remote.DefaultClusterAPIUserAgent(controllerName),
			Cache: clustercache.ClientCacheOptions{
				DisableFor: []client.Object{
					// Don't cache ConfigMaps & Secrets.
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
		WatchFilterValue: watchFilterValue,
	}, controller.Options{
		MaxConcurrentReconciles: clusterCacheConcurrency,
	})
	if err != nil {
		setupLog.Error(err, "Unable to create ClusterCache")
		os.Exit(1)
	}

	crdMigratorConfig := map[client.Object]crdmigrator.ByObjectConfig{
		&infrav1.DockerCluster{}:         {UseCache: true},
		&infrav1.DockerClusterTemplate{}: {UseCache: false},
		&infrav1.DockerMachine{}:         {UseCache: true},
		&infrav1.DockerMachineTemplate{}: {UseCache: false},
		&infrav1.DevCluster{}:            {UseCache: true},
		&infrav1.DevClusterTemplate{}:    {UseCache: false},
		&infrav1.DevMachine{}:            {UseCache: true},
		&infrav1.DevMachineTemplate{}:    {UseCache: false},
	}
	if feature.Gates.Enabled(feature.MachinePool) {
		crdMigratorConfig[&infraexpv1.DockerMachinePool{}] = crdmigrator.ByObjectConfig{UseCache: true}
		crdMigratorConfig[&infraexpv1.DockerMachinePoolTemplate{}] = crdmigrator.ByObjectConfig{UseCache: false}
	}
	if err := (&crdmigrator.CRDMigrator{
		Client:                 mgr.GetClient(),
		APIReader:              mgr.GetAPIReader(),
		SkipCRDMigrationPhases: skipCRDMigrationPhases,
		Config:                 crdMigratorConfig,
	}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "CRDMigrator")
		os.Exit(1)
	}

	// Start in memory manager
	inMemoryManager := inmemoryruntime.NewManager(inmemoryScheme)
	if err := inMemoryManager.Start(ctx); err != nil {
		setupLog.Error(err, "Unable to start a in memory manager")
		os.Exit(1)
	}

	// Start an http server
	podIP := os.Getenv("POD_IP")
	apiServerMux, err := inmemoryserver.NewWorkloadClustersMux(inMemoryManager, podIP)
	if err != nil {
		setupLog.Error(err, "Unable to create workload clusters mux")
		os.Exit(1)
	}

	if err := (&controllers.DockerMachineReconciler{
		Client:           mgr.GetClient(),
		ContainerRuntime: runtimeClient,
		ClusterCache:     clusterCache,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, controller.Options{
		MaxConcurrentReconciles: concurrency,
	}); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "DockerMachine")
		os.Exit(1)
	}

	if err := (&controllers.DockerClusterReconciler{
		Client:           mgr.GetClient(),
		ContainerRuntime: runtimeClient,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "DockerCluster")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&expcontrollers.DockerMachinePoolReconciler{
			Client:           mgr.GetClient(),
			ContainerRuntime: runtimeClient,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: concurrency}); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "DockerMachinePool")
			os.Exit(1)
		}
	}

	if err := (&controllers.DevMachineReconciler{
		Client:           mgr.GetClient(),
		WatchFilterValue: watchFilterValue,
		ContainerRuntime: runtimeClient,
		ClusterCache:     clusterCache,
		InMemoryManager:  inMemoryManager,
		APIServerMux:     apiServerMux,
	}).SetupWithManager(ctx, mgr, controller.Options{
		MaxConcurrentReconciles: concurrency,
	}); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "DevMachine")
		os.Exit(1)
	}

	if err := (&controllers.DevClusterReconciler{
		Client:           mgr.GetClient(),
		WatchFilterValue: watchFilterValue,
		ContainerRuntime: runtimeClient,
		InMemoryManager:  inMemoryManager,
		APIServerMux:     apiServerMux,
	}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "DevCluster")
		os.Exit(1)
	}
}

func setupWebhooks(mgr ctrl.Manager) {
	if err := (&infrawebhooks.DockerMachineTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "DockerMachineTemplate")
		os.Exit(1)
	}

	if err := (&infrawebhooks.DockerCluster{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "DockerCluster")
		os.Exit(1)
	}

	if err := (&infrawebhooks.DockerClusterTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "DockerClusterTemplate")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&infraexpwebhooks.DockerMachinePool{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Unable to create webhook", "webhook", "DockerMachinePool")
			os.Exit(1)
		}
	}

	if err := (&infrawebhooks.DevMachine{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "DevMachine")
		os.Exit(1)
	}

	if err := (&infrawebhooks.DevMachineTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "DevMachineTemplate")
		os.Exit(1)
	}

	if err := (&infrawebhooks.DevCluster{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "DevCluster")
		os.Exit(1)
	}

	if err := (&infrawebhooks.DevClusterTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "DevClusterTemplate")
		os.Exit(1)
	}
}
