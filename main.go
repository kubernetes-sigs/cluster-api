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
	"k8s.io/apimachinery/pkg/selection"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	addonscontrollers "sigs.k8s.io/cluster-api/exp/addons/controllers"
	addonswebhooks "sigs.k8s.io/cluster-api/exp/addons/webhooks"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	expcontrollers "sigs.k8s.io/cluster-api/exp/controllers"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	expipamwebhooks "sigs.k8s.io/cluster-api/exp/ipam/webhooks"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	runtimecontrollers "sigs.k8s.io/cluster-api/exp/runtime/controllers"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	expwebhooks "sigs.k8s.io/cluster-api/exp/webhooks"
	"sigs.k8s.io/cluster-api/feature"
	addonsv1alpha3 "sigs.k8s.io/cluster-api/internal/apis/core/exp/addons/v1alpha3"
	addonsv1alpha4 "sigs.k8s.io/cluster-api/internal/apis/core/exp/addons/v1alpha4"
	expv1alpha3 "sigs.k8s.io/cluster-api/internal/apis/core/exp/v1alpha3"
	expv1alpha4 "sigs.k8s.io/cluster-api/internal/apis/core/exp/v1alpha4"
	clusterv1alpha3 "sigs.k8s.io/cluster-api/internal/apis/core/v1alpha3"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/internal/apis/core/v1alpha4"
	internalruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	runtimewebhooks "sigs.k8s.io/cluster-api/internal/webhooks/runtime"
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
	healthAddr                  string
	managerOptions              = flags.ManagerOptions{}
	logOptions                  = logs.NewOptions()
	// core Cluster API specific flags.
	remoteConnectionGracePeriod     time.Duration
	remoteConditionsGracePeriod     time.Duration
	clusterTopologyConcurrency      int
	clusterCacheConcurrency         int
	clusterClassConcurrency         int
	clusterConcurrency              int
	extensionConfigConcurrency      int
	machineConcurrency              int
	machineSetConcurrency           int
	machineDeploymentConcurrency    int
	machinePoolConcurrency          int
	clusterResourceSetConcurrency   int
	machineHealthCheckConcurrency   int
	useDeprecatedInfraMachineNaming bool
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)

	_ = clusterv1alpha3.AddToScheme(scheme)
	_ = clusterv1alpha4.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	_ = expv1alpha3.AddToScheme(scheme)
	_ = expv1alpha4.AddToScheme(scheme)
	_ = expv1.AddToScheme(scheme)

	_ = addonsv1alpha3.AddToScheme(scheme)
	_ = addonsv1alpha4.AddToScheme(scheme)
	_ = addonsv1.AddToScheme(scheme)

	_ = runtimev1.AddToScheme(scheme)

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

	fs.BoolVar(&useDeprecatedInfraMachineNaming, "use-deprecated-infra-machine-naming", false,
		"Use deprecated infrastructure machine naming")
	_ = fs.MarkDeprecated("use-deprecated-infra-machine-naming", "This flag will be removed in v1.9.")

	flags.AddManagerOptions(fs, &managerOptions)

	feature.MutableGates.AddFlag(fs)
}

// Add RBAC for the authorized diagnostics endpoint.
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func main() {
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
	restConfig.WarningHandler = apiwarnings.DefaultHandler(klog.Background().WithName("API Server Warning"))

	minVer := version.MinimumKubernetesVersion
	if feature.Gates.Enabled(feature.ClusterTopology) {
		minVer = version.MinimumKubernetesVersionClusterTopology
	}

	if !(remoteConditionsGracePeriod > remoteConnectionGracePeriod) {
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
			UsePriorityQueue: feature.Gates.Enabled(feature.PriorityQueue),
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
	setupWebhooks(mgr, clusterCache)

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

	var runtimeClient runtimeclient.Client
	if feature.Gates.Enabled(feature.RuntimeSDK) {
		// This is the creation of the runtimeClient for the controllers, embedding a shared catalog and registry instance.
		runtimeClient = internalruntimeclient.New(internalruntimeclient.Options{
			Catalog:  catalog,
			Registry: runtimeregistry.New(),
			Client:   mgr.GetClient(),
		})
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
		if err = (&runtimecontrollers.ExtensionConfigReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			RuntimeClient:    runtimeClient,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(extensionConfigConcurrency), partialSecretCache); err != nil {
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
	if err := (&controllers.MachineReconciler{
		Client:                      mgr.GetClient(),
		APIReader:                   mgr.GetAPIReader(),
		ClusterCache:                clusterCache,
		WatchFilterValue:            watchFilterValue,
		RemoteConditionsGracePeriod: remoteConditionsGracePeriod,
	}).SetupWithManager(ctx, mgr, concurrency(machineConcurrency)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Machine")
		os.Exit(1)
	}
	if err := (&controllers.MachineSetReconciler{
		Client:                       mgr.GetClient(),
		APIReader:                    mgr.GetAPIReader(),
		ClusterCache:                 clusterCache,
		WatchFilterValue:             watchFilterValue,
		DeprecatedInfraMachineNaming: useDeprecatedInfraMachineNaming,
	}).SetupWithManager(ctx, mgr, concurrency(machineSetConcurrency)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "MachineSet")
		os.Exit(1)
	}
	if err := (&controllers.MachineDeploymentReconciler{
		Client:           mgr.GetClient(),
		APIReader:        mgr.GetAPIReader(),
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(machineDeploymentConcurrency)); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "MachineDeployment")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&expcontrollers.MachinePoolReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			ClusterCache:     clusterCache,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(machinePoolConcurrency)); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "MachinePool")
			os.Exit(1)
		}
	}

	if feature.Gates.Enabled(feature.ClusterResourceSet) {
		if err := (&addonscontrollers.ClusterResourceSetReconciler{
			Client:           mgr.GetClient(),
			ClusterCache:     clusterCache,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(clusterResourceSetConcurrency), partialSecretCache); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "ClusterResourceSet")
			os.Exit(1)
		}
		if err := (&addonscontrollers.ClusterResourceSetBindingReconciler{
			Client:           mgr.GetClient(),
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(clusterResourceSetConcurrency)); err != nil {
			setupLog.Error(err, "Unable to create controller", "controller", "ClusterResourceSetBinding")
			os.Exit(1)
		}
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

func setupWebhooks(mgr ctrl.Manager, clusterCacheReader webhooks.ClusterCacheReader) {
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
	if err := (&expwebhooks.MachinePool{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "MachinePool")
		os.Exit(1)
	}

	// NOTE: ClusterResourceSet is behind ClusterResourceSet feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&addonswebhooks.ClusterResourceSet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "ClusterResourceSet")
		os.Exit(1)
	}
	// NOTE: ClusterResourceSetBinding is behind ClusterResourceSet feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&addonswebhooks.ClusterResourceSetBinding{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "ClusterResourceSetBinding")
		os.Exit(1)
	}

	if err := (&webhooks.MachineHealthCheck{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "MachineHealthCheck")
		os.Exit(1)
	}

	// NOTE: ExtensionConfig is behind the RuntimeSDK feature gate flag. The webhook will prevent creating or updating
	// new objects if the feature flag is disabled.
	if err := (&runtimewebhooks.ExtensionConfig{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "ExtensionConfig")
		os.Exit(1)
	}

	if err := (&expipamwebhooks.IPAddress{
		// We are using GetAPIReader here to avoid caching all IPAddressClaims
		Client: mgr.GetAPIReader(),
	}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "IPAddress")
		os.Exit(1)
	}
	if err := (&expipamwebhooks.IPAddressClaim{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create webhook", "webhook", "IPAddressClaim")
		os.Exit(1)
	}
}

func concurrency(c int) controller.Options {
	return controller.Options{MaxConcurrentReconciles: c}
}
