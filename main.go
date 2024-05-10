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
	"errors"
	"flag"
	"fmt"
	"os"
	goruntime "runtime"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers"
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
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	runtimewebhooks "sigs.k8s.io/cluster-api/internal/webhooks/runtime"
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
	webhookPort                 int
	webhookCertDir              string
	webhookCertName             string
	webhookKeyName              string
	healthAddr                  string
	tlsOptions                  = flags.TLSOptions{}
	diagnosticsOptions          = flags.DiagnosticsOptions{}
	logOptions                  = logs.NewOptions()
	// core Cluster API specific flags.
	clusterTopologyConcurrency      int
	clusterCacheTrackerConcurrency  int
	clusterClassConcurrency         int
	clusterConcurrency              int
	extensionConfigConcurrency      int
	machineConcurrency              int
	machineSetConcurrency           int
	machineDeploymentConcurrency    int
	machinePoolConcurrency          int
	clusterResourceSetConcurrency   int
	machineHealthCheckConcurrency   int
	nodeDrainClientTimeout          time.Duration
	useDeprecatedInfraMachineNaming bool
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)

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

	fs.IntVar(&clusterTopologyConcurrency, "clustertopology-concurrency", 10,
		"Number of clusters to process simultaneously")

	fs.IntVar(&clusterClassConcurrency, "clusterclass-concurrency", 10,
		"Number of ClusterClasses to process simultaneously")

	fs.IntVar(&clusterConcurrency, "cluster-concurrency", 10,
		"Number of clusters to process simultaneously")

	fs.IntVar(&clusterCacheTrackerConcurrency, "clustercachetracker-concurrency", 10,
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
		"Maximum queries per second from the controller client to the Kubernetes API server. Defaults to 20")

	fs.IntVar(&restConfigBurst, "kube-api-burst", 30,
		"Maximum number of queries that should be allowed in one burst from the controller client to the Kubernetes API server. Default 30")

	fs.DurationVar(&nodeDrainClientTimeout, "node-drain-client-timeout-duration", time.Second*10,
		"The timeout of the client used for draining nodes. Defaults to 10s")

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

	flags.AddDiagnosticsOptions(fs, &diagnosticsOptions)
	flags.AddTLSOptions(fs, &tlsOptions)

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
		setupLog.Error(err, "failed to set default log level")
		os.Exit(1)
	}
	pflag.Parse()

	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// klog.Background will automatically use the right logger.
	ctrl.SetLogger(klog.Background())

	restConfig := ctrl.GetConfigOrDie()
	restConfig.QPS = restConfigQPS
	restConfig.Burst = restConfigBurst
	restConfig.UserAgent = remote.DefaultClusterAPIUserAgent(controllerName)

	if nodeDrainClientTimeout <= 0 {
		setupLog.Error(errors.New("node drain client timeout must be greater than zero"), "unable to start manager")
		os.Exit(1)
	}

	minVer := version.MinimumKubernetesVersion
	if feature.Gates.Enabled(feature.ClusterTopology) {
		minVer = version.MinimumKubernetesVersionClusterTopology
	}

	if err := version.CheckKubernetesVersion(restConfig, minVer); err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	tlsOptionOverrides, err := flags.GetTLSOptionOverrideFuncs(tlsOptions)
	if err != nil {
		setupLog.Error(err, "unable to add TLS settings to the webhook server")
		os.Exit(1)
	}

	diagnosticsOpts := flags.GetDiagnosticsOptions(diagnosticsOptions)

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
		Scheme:                     scheme,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "controller-leader-election-capi",
		LeaseDuration:              &leaderElectionLeaseDuration,
		RenewDeadline:              &leaderElectionRenewDeadline,
		RetryPeriod:                &leaderElectionRetryPeriod,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		HealthProbeBindAddress:     healthAddr,
		PprofBindAddress:           profilerAddress,
		Metrics:                    diagnosticsOpts,
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
			},
		},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port:     webhookPort,
				CertDir:  webhookCertDir,
				CertName: webhookCertName,
				KeyName:  webhookKeyName,
				TLSOpts:  tlsOptionOverrides,
			},
		),
	}

	mgr, err := ctrl.NewManager(restConfig, ctrlOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupIndexes(ctx, mgr)
	tracker := setupReconcilers(ctx, mgr)
	setupWebhooks(mgr, tracker)

	setupLog.Info("starting manager", "version", version.Get().String())
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupChecks(mgr ctrl.Manager) {
	if err := mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}
}

func setupIndexes(ctx context.Context, mgr ctrl.Manager) {
	if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to setup indexes")
		os.Exit(1)
	}
}

func setupReconcilers(ctx context.Context, mgr ctrl.Manager) webhooks.ClusterCacheTrackerReader {
	secretCachingClient, err := client.New(mgr.GetConfig(), client.Options{
		HTTPClient: mgr.GetHTTPClient(),
		Cache: &client.CacheOptions{
			Reader: mgr.GetCache(),
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to create secret caching client")
		os.Exit(1)
	}

	// Set up a ClusterCacheTracker and ClusterCacheReconciler to provide to controllers
	// requiring a connection to a remote cluster
	tracker, err := remote.NewClusterCacheTracker(
		mgr,
		remote.ClusterCacheTrackerOptions{
			SecretCachingClient: secretCachingClient,
			ControllerName:      controllerName,
			Log:                 &ctrl.Log,
			Indexes:             []remote.Index{remote.NodeProviderIDIndex},
		},
	)
	if err != nil {
		setupLog.Error(err, "unable to create cluster cache tracker")
		os.Exit(1)
	}

	if err := (&remote.ClusterCacheReconciler{
		Client:           mgr.GetClient(),
		Tracker:          tracker,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(clusterCacheTrackerConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterCacheReconciler")
		os.Exit(1)
	}

	var runtimeClient runtimeclient.Client
	if feature.Gates.Enabled(feature.RuntimeSDK) {
		// This is the creation of the runtimeClient for the controllers, embedding a shared catalog and registry instance.
		runtimeClient = runtimeclient.New(runtimeclient.Options{
			Catalog:  catalog,
			Registry: runtimeregistry.New(),
			Client:   mgr.GetClient(),
		})
	}

	unstructuredCachingClient, err := client.New(mgr.GetConfig(), client.Options{
		HTTPClient: mgr.GetHTTPClient(),
		Cache: &client.CacheOptions{
			Reader:       mgr.GetCache(),
			Unstructured: true,
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to create unstructured caching client")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.ClusterTopology) {
		if err := (&controllers.ClusterClassReconciler{
			Client:                    mgr.GetClient(),
			RuntimeClient:             runtimeClient,
			UnstructuredCachingClient: unstructuredCachingClient,
			WatchFilterValue:          watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(clusterClassConcurrency)); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ClusterClass")
			os.Exit(1)
		}

		if err := (&controllers.ClusterTopologyReconciler{
			Client:                    mgr.GetClient(),
			APIReader:                 mgr.GetAPIReader(),
			RuntimeClient:             runtimeClient,
			Tracker:                   tracker,
			UnstructuredCachingClient: unstructuredCachingClient,
			WatchFilterValue:          watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(clusterTopologyConcurrency)); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ClusterTopology")
			os.Exit(1)
		}

		if err := (&controllers.MachineDeploymentTopologyReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "MachineDeploymentTopology")
			os.Exit(1)
		}

		if err := (&controllers.MachineSetTopologyReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "MachineSetTopology")
			os.Exit(1)
		}
	}

	if feature.Gates.Enabled(feature.RuntimeSDK) {
		if err = (&runtimecontrollers.ExtensionConfigReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			RuntimeClient:    runtimeClient,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(extensionConfigConcurrency)); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ExtensionConfig")
			os.Exit(1)
		}
	}

	if err := (&controllers.ClusterReconciler{
		Client:                    mgr.GetClient(),
		UnstructuredCachingClient: unstructuredCachingClient,
		APIReader:                 mgr.GetAPIReader(),
		WatchFilterValue:          watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(clusterConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cluster")
		os.Exit(1)
	}
	if err := (&controllers.MachineReconciler{
		Client:                    mgr.GetClient(),
		UnstructuredCachingClient: unstructuredCachingClient,
		APIReader:                 mgr.GetAPIReader(),
		Tracker:                   tracker,
		WatchFilterValue:          watchFilterValue,
		NodeDrainClientTimeout:    nodeDrainClientTimeout,
	}).SetupWithManager(ctx, mgr, concurrency(machineConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Machine")
		os.Exit(1)
	}
	if err := (&controllers.MachineSetReconciler{
		Client:                       mgr.GetClient(),
		UnstructuredCachingClient:    unstructuredCachingClient,
		APIReader:                    mgr.GetAPIReader(),
		Tracker:                      tracker,
		WatchFilterValue:             watchFilterValue,
		DeprecatedInfraMachineNaming: useDeprecatedInfraMachineNaming,
	}).SetupWithManager(ctx, mgr, concurrency(machineSetConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineSet")
		os.Exit(1)
	}
	if err := (&controllers.MachineDeploymentReconciler{
		Client:                    mgr.GetClient(),
		UnstructuredCachingClient: unstructuredCachingClient,
		APIReader:                 mgr.GetAPIReader(),
		WatchFilterValue:          watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(machineDeploymentConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineDeployment")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&expcontrollers.MachinePoolReconciler{
			Client:           mgr.GetClient(),
			APIReader:        mgr.GetAPIReader(),
			Tracker:          tracker,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(machinePoolConcurrency)); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "MachinePool")
			os.Exit(1)
		}
	}

	if feature.Gates.Enabled(feature.ClusterResourceSet) {
		if err := (&addonscontrollers.ClusterResourceSetReconciler{
			Client:           mgr.GetClient(),
			Tracker:          tracker,
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(clusterResourceSetConcurrency)); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ClusterResourceSet")
			os.Exit(1)
		}
		if err := (&addonscontrollers.ClusterResourceSetBindingReconciler{
			Client:           mgr.GetClient(),
			WatchFilterValue: watchFilterValue,
		}).SetupWithManager(ctx, mgr, concurrency(clusterResourceSetConcurrency)); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ClusterResourceSetBinding")
			os.Exit(1)
		}
	}

	if err := (&controllers.MachineHealthCheckReconciler{
		Client:           mgr.GetClient(),
		Tracker:          tracker,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(machineHealthCheckConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineHealthCheck")
		os.Exit(1)
	}

	return tracker
}

func setupWebhooks(mgr ctrl.Manager, tracker webhooks.ClusterCacheTrackerReader) {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled.
	if err := (&webhooks.ClusterClass{Client: mgr.GetClient()}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ClusterClass")
		os.Exit(1)
	}

	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the webhook
	// is going to prevent usage of Cluster.Topology in case the feature flag is disabled.
	if err := (&webhooks.Cluster{Client: mgr.GetClient(), ClusterCacheTrackerReader: tracker}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Cluster")
		os.Exit(1)
	}

	if err := (&webhooks.Machine{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Machine")
		os.Exit(1)
	}

	if err := (&webhooks.MachineSet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineSet")
		os.Exit(1)
	}

	if err := (&webhooks.MachineDeployment{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineDeployment")
		os.Exit(1)
	}

	// NOTE: MachinePool is behind MachinePool feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&expwebhooks.MachinePool{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachinePool")
		os.Exit(1)
	}

	// NOTE: ClusterResourceSet is behind ClusterResourceSet feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&addonswebhooks.ClusterResourceSet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ClusterResourceSet")
		os.Exit(1)
	}
	// NOTE: ClusterResourceSetBinding is behind ClusterResourceSet feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&addonswebhooks.ClusterResourceSetBinding{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ClusterResourceSetBinding")
		os.Exit(1)
	}

	if err := (&webhooks.MachineHealthCheck{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineHealthCheck")
		os.Exit(1)
	}

	// NOTE: ExtensionConfig is behind the RuntimeSDK feature gate flag. The webhook will prevent creating or updating
	// new objects if the feature flag is disabled.
	if err := (&runtimewebhooks.ExtensionConfig{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ExtensionConfig")
		os.Exit(1)
	}

	if err := (&expipamwebhooks.IPAddress{
		// We are using GetAPIReader here to avoid caching all IPAddressClaims
		Client: mgr.GetAPIReader(),
	}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "IPAddress")
		os.Exit(1)
	}
	if err := (&expipamwebhooks.IPAddressClaim{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "IPAddressClaim")
		os.Exit(1)
	}
}

func concurrency(c int) controller.Options {
	return controller.Options{MaxConcurrentReconciles: c}
}
