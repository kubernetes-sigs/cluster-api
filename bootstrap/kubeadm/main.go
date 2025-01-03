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

// main is the main package for the Kubeadm Bootstrap provider.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	goruntime "runtime"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
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
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kubeadmbootstrapcontrollers "sigs.k8s.io/cluster-api/bootstrap/kubeadm/controllers"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/internal/webhooks"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	bootstrapv1alpha3 "sigs.k8s.io/cluster-api/internal/apis/bootstrap/kubeadm/v1alpha3"
	bootstrapv1alpha4 "sigs.k8s.io/cluster-api/internal/apis/bootstrap/kubeadm/v1alpha4"
	"sigs.k8s.io/cluster-api/util/apiwarnings"
	"sigs.k8s.io/cluster-api/util/flags"
	"sigs.k8s.io/cluster-api/version"
)

var (
	scheme         = runtime.NewScheme()
	setupLog       = ctrl.Log.WithName("setup")
	controllerName = "cluster-api-kubeadm-bootstrap-manager"

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
	// CABPK specific flags.
	clusterConcurrency       int
	clusterCacheConcurrency  int
	kubeadmConfigConcurrency int
	tokenTTL                 time.Duration
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = expv1.AddToScheme(scheme)
	_ = bootstrapv1alpha3.AddToScheme(scheme)
	_ = bootstrapv1alpha4.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)
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

	fs.IntVar(&clusterConcurrency, "cluster-concurrency", 10,
		"Number of clusters to process simultaneously")
	_ = fs.MarkDeprecated("cluster-concurrency", "This flag has no function anymore and is going to be removed in a next release. Use \"--clustercachetracker-concurrency\" instead.")

	fs.IntVar(&clusterCacheConcurrency, "clustercache-concurrency", 100,
		"Number of clusters to process simultaneously")

	fs.IntVar(&kubeadmConfigConcurrency, "kubeadmconfig-concurrency", 10,
		"Number of kubeadm configs to process simultaneously")

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

	fs.DurationVar(&tokenTTL, "bootstrap-token-ttl", kubeadmbootstrapcontrollers.DefaultTokenTTL,
		"The amount of time the bootstrap token will be valid")

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
	restConfig.WarningHandler = apiwarnings.DefaultHandler(klog.Background().WithName("API Server Warning"))

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
		LeaderElectionID:           "kubeadm-bootstrap-manager-leader-election-capi",
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
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupWebhooks(mgr)
	setupReconcilers(ctx, mgr)

	setupLog.Info("Starting manager", "version", version.Get().String())
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

func setupReconcilers(ctx context.Context, mgr ctrl.Manager) {
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
	}, concurrency(clusterCacheConcurrency))
	if err != nil {
		setupLog.Error(err, "Unable to create ClusterCache")
		os.Exit(1)
	}

	if err := (&kubeadmbootstrapcontrollers.KubeadmConfigReconciler{
		Client:              mgr.GetClient(),
		SecretCachingClient: secretCachingClient,
		ClusterCache:        clusterCache,
		WatchFilterValue:    watchFilterValue,
		TokenTTL:            tokenTTL,
	}).SetupWithManager(ctx, mgr, concurrency(kubeadmConfigConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KubeadmConfig")
		os.Exit(1)
	}
}

func setupWebhooks(mgr ctrl.Manager) {
	if err := (&webhooks.KubeadmConfig{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "KubeadmConfig")
		os.Exit(1)
	}
	if err := (&webhooks.KubeadmConfigTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "KubeadmConfigTemplate")
		os.Exit(1)
	}
}

func concurrency(c int) controller.Options {
	return controller.Options{MaxConcurrentReconciles: c}
}
