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

package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	// +kubebuilder:scaffold:imports
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers"
	"sigs.k8s.io/cluster-api/controllers/remote"
	addonsv1alpha3 "sigs.k8s.io/cluster-api/exp/addons/api/v1alpha3"
	addonsv1alpha4 "sigs.k8s.io/cluster-api/exp/addons/api/v1alpha4"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	addonscontrollers "sigs.k8s.io/cluster-api/exp/addons/controllers"
	expv1alpha3 "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
	expv1alpha4 "sigs.k8s.io/cluster-api/exp/api/v1alpha4"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	expcontrollers "sigs.k8s.io/cluster-api/exp/controllers"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	expipamwebhooks "sigs.k8s.io/cluster-api/exp/ipam/webhooks"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimecontrollers "sigs.k8s.io/cluster-api/exp/runtime/controllers"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	runtimewebhooks "sigs.k8s.io/cluster-api/internal/webhooks/runtime"
	"sigs.k8s.io/cluster-api/version"
	"sigs.k8s.io/cluster-api/webhooks"
)

var (
	catalog  = runtimecatalog.New()
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// flags.
	metricsBindAddr               string
	enableLeaderElection          bool
	leaderElectionLeaseDuration   time.Duration
	leaderElectionRenewDeadline   time.Duration
	leaderElectionRetryPeriod     time.Duration
	watchNamespace                string
	watchFilterValue              string
	profilerAddress               string
	clusterTopologyConcurrency    int
	clusterClassConcurrency       int
	clusterConcurrency            int
	extensionConfigConcurrency    int
	machineConcurrency            int
	machineSetConcurrency         int
	machineDeploymentConcurrency  int
	machinePoolConcurrency        int
	clusterResourceSetConcurrency int
	machineHealthCheckConcurrency int
	syncPeriod                    time.Duration
	webhookPort                   int
	webhookCertDir                string
	healthAddr                    string
	logOptions                    = logs.NewOptions()
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
	// +kubebuilder:scaffold:scheme

	// Register the RuntimeHook types into the catalog.
	_ = runtimehooksv1.AddToCatalog(catalog)
}

// InitFlags initializes the flags.
func InitFlags(fs *pflag.FlagSet) {
	logs.AddFlags(fs, logs.SkipLoggingConfigurationFlags())
	logOptions.AddFlags(fs)

	fs.StringVar(&metricsBindAddr, "metrics-bind-addr", "localhost:8080",
		"The address the metric endpoint binds to.")

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

	fs.IntVar(&clusterTopologyConcurrency, "clustertopology-concurrency", 10,
		"Number of clusters to process simultaneously")

	fs.IntVar(&clusterClassConcurrency, "clusterclass-concurrency", 10,
		"Number of cluster classes to process simultaneously")

	fs.IntVar(&clusterConcurrency, "cluster-concurrency", 10,
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

	fs.IntVar(&webhookPort, "webhook-port", 9443,
		"Webhook Server port")

	fs.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs/",
		"Webhook cert dir, only used when webhook-port is specified.")

	fs.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")

	feature.MutableGates.AddFlag(fs)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	InitFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	if err := logOptions.ValidateAndApply(nil); err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// klog.Background will automatically use the right logger.
	ctrl.SetLogger(klog.Background())

	if profilerAddress != "" {
		klog.Infof("Profiler listening for requests at %s", profilerAddress)
		go func() {
			klog.Info(http.ListenAndServe(profilerAddress, nil))
		}()
	}

	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = remote.DefaultClusterAPIUserAgent("cluster-api-controller-manager")

	minVer := version.MinimumKubernetesVersion
	if feature.Gates.Enabled(feature.ClusterTopology) {
		minVer = version.MinimumKubernetesVersionClusterTopology
	}

	if err := version.CheckKubernetesVersion(restConfig, minVer); err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         metricsBindAddr,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "controller-leader-election-capi",
		LeaseDuration:              &leaderElectionLeaseDuration,
		RenewDeadline:              &leaderElectionRenewDeadline,
		RetryPeriod:                &leaderElectionRetryPeriod,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		Namespace:                  watchNamespace,
		SyncPeriod:                 &syncPeriod,
		ClientDisableCacheFor: []client.Object{
			&corev1.ConfigMap{},
			&corev1.Secret{},
		},
		Port:                   webhookPort,
		CertDir:                webhookCertDir,
		HealthProbeBindAddress: healthAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupIndexes(ctx, mgr)
	setupReconcilers(ctx, mgr)
	setupWebhooks(mgr)

	// +kubebuilder:scaffold:builder
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

func setupReconcilers(ctx context.Context, mgr ctrl.Manager) {
	// Set up a ClusterCacheTracker and ClusterCacheReconciler to provide to controllers
	// requiring a connection to a remote cluster
	log := ctrl.Log.WithName("remote").WithName("ClusterCacheTracker")
	tracker, err := remote.NewClusterCacheTracker(
		mgr,
		remote.ClusterCacheTrackerOptions{
			Log:     &log,
			Indexes: remote.DefaultIndexes,
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
	}).SetupWithManager(ctx, mgr, concurrency(clusterConcurrency)); err != nil {
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

	if feature.Gates.Enabled(feature.ClusterTopology) {
		unstructuredCachingClient, err := client.NewDelegatingClient(
			client.NewDelegatingClientInput{
				// Use the default client for write operations.
				Client: mgr.GetClient(),
				// For read operations, use the same cache used by all the controllers but ensure
				// unstructured objects will be also cached (this does not happen with the default client).
				CacheReader:       mgr.GetCache(),
				CacheUnstructured: true,
			},
		)
		if err != nil {
			setupLog.Error(err, "unable to create unstructured caching client", "controller", "ClusterTopology")
			os.Exit(1)
		}

		if err := (&controllers.ClusterClassReconciler{
			Client:                    mgr.GetClient(),
			APIReader:                 mgr.GetAPIReader(),
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
		Client:           mgr.GetClient(),
		APIReader:        mgr.GetAPIReader(),
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(clusterConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cluster")
		os.Exit(1)
	}
	if err := (&controllers.MachineReconciler{
		Client:           mgr.GetClient(),
		APIReader:        mgr.GetAPIReader(),
		Tracker:          tracker,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(machineConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Machine")
		os.Exit(1)
	}
	if err := (&controllers.MachineSetReconciler{
		Client:           mgr.GetClient(),
		APIReader:        mgr.GetAPIReader(),
		Tracker:          tracker,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(machineSetConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineSet")
		os.Exit(1)
	}
	if err := (&controllers.MachineDeploymentReconciler{
		Client:           mgr.GetClient(),
		APIReader:        mgr.GetAPIReader(),
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, concurrency(machineDeploymentConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineDeployment")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&expcontrollers.MachinePoolReconciler{
			Client:           mgr.GetClient(),
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
}

func setupWebhooks(mgr ctrl.Manager) {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled.
	if err := (&webhooks.ClusterClass{Client: mgr.GetClient()}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ClusterClass")
		os.Exit(1)
	}

	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the webhook
	// is going to prevent usage of Cluster.Topology in case the feature flag is disabled.
	if err := (&webhooks.Cluster{Client: mgr.GetClient()}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Cluster")
		os.Exit(1)
	}

	if err := (&clusterv1.Machine{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Machine")
		os.Exit(1)
	}

	if err := (&clusterv1.MachineSet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineSet")
		os.Exit(1)
	}

	if err := (&clusterv1.MachineDeployment{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineDeployment")
		os.Exit(1)
	}

	// NOTE: MachinePool is behind MachinePool feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&expv1.MachinePool{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachinePool")
		os.Exit(1)
	}

	// NOTE: ClusterResourceSet is behind ClusterResourceSet feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled
	if err := (&addonsv1.ClusterResourceSet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ClusterResourceSet")
		os.Exit(1)
	}

	if err := (&clusterv1.MachineHealthCheck{}).SetupWebhookWithManager(mgr); err != nil {
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
