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
	"math/rand"
	"net/http"
	"os"
	"time"

	// +kubebuilder:scaffold:imports
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
)

var (
	myscheme = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// flags.
	metricsBindAddr      string
	enableLeaderElection bool
	profilerAddress      string
	syncPeriod           time.Duration
	concurrency          int
	healthAddr           string
	webhookPort          int
	webhookCertDir       string
	logOptions           = logs.NewOptions()
)

func init() {
	_ = scheme.AddToScheme(myscheme)
	_ = infrav1alpha3.AddToScheme(myscheme)
	_ = infrav1alpha4.AddToScheme(myscheme)
	_ = infrav1.AddToScheme(myscheme)
	_ = infraexpv1alpha3.AddToScheme(myscheme)
	_ = infraexpv1alpha4.AddToScheme(myscheme)
	_ = infraexpv1.AddToScheme(myscheme)
	_ = clusterv1.AddToScheme(myscheme)
	_ = expv1.AddToScheme(myscheme)
	// +kubebuilder:scaffold:scheme
}

func initFlags(fs *pflag.FlagSet) {
	logs.AddFlags(fs, logs.SkipLoggingConfigurationFlags())
	logsv1.AddFlags(logOptions, fs)

	fs.StringVar(&metricsBindAddr, "metrics-bind-addr", "localhost:8080",
		"The address the metric endpoint binds to.")
	fs.IntVar(&concurrency, "concurrency", 10,
		"The number of docker machines to process simultaneously")
	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")
	fs.DurationVar(&syncPeriod, "sync-period", 10*time.Minute,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")
	fs.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")
	fs.IntVar(&webhookPort, "webhook-port", 9443,
		"Webhook Server port")
	fs.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs/",
		"Webhook cert dir, only used when webhook-port is specified.")

	feature.MutableGates.AddFlag(fs)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	if _, err := os.ReadDir("/tmp/"); err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	initFlags(pflag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.Parse()

	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// klog.Background will automatically use the right logger.
	ctrl.SetLogger(klog.Background())

	if profilerAddress != "" {
		setupLog.Info(fmt.Sprintf("Profiler listening for requests at %s", profilerAddress))
		go func() {
			srv := http.Server{Addr: profilerAddress, ReadHeaderTimeout: 2 * time.Second}
			if err := srv.ListenAndServe(); err != nil {
				setupLog.Error(err, "problem running profiler server")
			}
		}()
	}

	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = remote.DefaultClusterAPIUserAgent("cluster-api-docker-controller-manager")
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                     myscheme,
		MetricsBindAddress:         metricsBindAddr,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "controller-leader-election-capd",
		SyncPeriod:                 &syncPeriod,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		HealthProbeBindAddress:     healthAddr,
		Port:                       webhookPort,
		CertDir:                    webhookCertDir,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupReconcilers(ctx, mgr)
	setupWebhooks(mgr)

	// +kubebuilder:scaffold:builder
	setupLog.Info("starting manager")
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
	// Set our runtime client into the context for later use
	runtimeClient, err := container.NewDockerClient()
	if err != nil {
		setupLog.Error(err, "unable to establish container runtime connection", "controller", "reconciler")
		os.Exit(1)
	}

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
		Client:  mgr.GetClient(),
		Tracker: tracker,
	}).SetupWithManager(ctx, mgr, controller.Options{
		MaxConcurrentReconciles: concurrency,
	}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterCacheReconciler")
		os.Exit(1)
	}

	if err := (&controllers.DockerMachineReconciler{
		Client:           mgr.GetClient(),
		ContainerRuntime: runtimeClient,
		Tracker:          tracker,
	}).SetupWithManager(ctx, mgr, controller.Options{
		MaxConcurrentReconciles: concurrency,
	}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DockerMachine")
		os.Exit(1)
	}

	if err := (&controllers.DockerClusterReconciler{
		Client:           mgr.GetClient(),
		ContainerRuntime: runtimeClient,
	}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DockerCluster")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&expcontrollers.DockerMachinePoolReconciler{
			Client:           mgr.GetClient(),
			ContainerRuntime: runtimeClient,
			Tracker:          tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: concurrency}); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "DockerMachinePool")
			os.Exit(1)
		}
	}
}

func setupWebhooks(mgr ctrl.Manager) {
	if err := (&infrav1.DockerMachineTemplateWebhook{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "DockerMachineTemplate")
		os.Exit(1)
	}

	if err := (&infrav1.DockerCluster{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "DockerCluster")
		os.Exit(1)
	}

	if err := (&infrav1.DockerClusterTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "DockerClusterTemplate")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&infraexpv1.DockerMachinePool{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "DockerMachinePool")
			os.Exit(1)
		}
	}
}
