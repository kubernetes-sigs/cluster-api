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
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/spf13/pflag"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	clusterv1alpha2 "sigs.k8s.io/cluster-api/api/v1alpha2"
	clusterv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers"
	expv1alpha3 "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
	expcontrollers "sigs.k8s.io/cluster-api/exp/controllers"
	"sigs.k8s.io/cluster-api/feature"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// flags
	metricsAddr                   string
	enableLeaderElection          bool
	leaderElectionLeaseDuration   time.Duration
	leaderElectionRenewDeadline   time.Duration
	leaderElectionRetryPeriod     time.Duration
	watchNamespace                string
	profilerAddress               string
	clusterConcurrency            int
	machineConcurrency            int
	machineSetConcurrency         int
	machineDeploymentConcurrency  int
	machinePoolConcurrency        int
	machineHealthCheckConcurrency int
	syncPeriod                    time.Duration
	webhookPort                   int
	healthAddr                    string
)

func init() {
	klog.InitFlags(nil)

	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1alpha2.AddToScheme(scheme)
	_ = clusterv1alpha3.AddToScheme(scheme)
	_ = expv1alpha3.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

// InitFlags initializes the flags.
func InitFlags(fs *pflag.FlagSet) {
	fs.StringVar(&metricsAddr, "metrics-addr", ":8080",
		"The address the metric endpoint binds to.")

	fs.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")

	fs.DurationVar(&leaderElectionLeaseDuration, "leader-election-lease-duration", 15*time.Second,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")

	fs.DurationVar(&leaderElectionRenewDeadline, "leader-election-renew-deadline", 10*time.Second,
		"Duration that the acting master will retry refreshing leadership before giving up (duration string)")

	fs.DurationVar(&leaderElectionRetryPeriod, "leader-election-retry-period", 2*time.Second,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")

	fs.StringVar(&watchNamespace, "namespace", "",
		"Namespace that the controller watches to reconcile cluster-api objects. If unspecified, the controller watches for cluster-api objects across all namespaces.")

	fs.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")

	fs.IntVar(&clusterConcurrency, "cluster-concurrency", 10,
		"Number of clusters to process simultaneously")

	fs.IntVar(&machineConcurrency, "machine-concurrency", 10,
		"Number of machines to process simultaneously")

	fs.IntVar(&machineSetConcurrency, "machineset-concurrency", 10,
		"Number of machine sets to process simultaneously")

	fs.IntVar(&machineDeploymentConcurrency, "machinedeployment-concurrency", 10,
		"Number of machine deployments to process simultaneously")

	fs.IntVar(&machinePoolConcurrency, "machinepool-concurrency", 10,
		"Number of machine pools to process simultaneously")

	fs.IntVar(&machineHealthCheckConcurrency, "machinehealthcheck-concurrency", 10,
		"Number of machine health checks to process simultaneously")

	fs.DurationVar(&syncPeriod, "sync-period", 10*time.Minute,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")

	fs.IntVar(&webhookPort, "webhook-port", 0,
		"Webhook Server port, disabled by default. When enabled, the manager will only work as webhook server, no reconcilers are installed.")

	fs.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")

	feature.MutableGates.AddFlag(fs)
}

func main() {
	InitFlags(pflag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(klogr.New())

	if profilerAddress != "" {
		klog.Infof("Profiler listening for requests at %s", profilerAddress)
		go func() {
			klog.Info(http.ListenAndServe(profilerAddress, nil))
		}()
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "controller-leader-election-capi",
		LeaseDuration:          &leaderElectionLeaseDuration,
		RenewDeadline:          &leaderElectionRenewDeadline,
		RetryPeriod:            &leaderElectionRetryPeriod,
		Namespace:              watchNamespace,
		SyncPeriod:             &syncPeriod,
		NewClient:              newClientFunc,
		Port:                   webhookPort,
		HealthProbeBindAddress: healthAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupChecks(mgr)
	setupReconcilers(mgr)
	setupWebhooks(mgr)

	// +kubebuilder:scaffold:builder
	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupChecks(mgr ctrl.Manager) {
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}
}

func setupReconcilers(mgr ctrl.Manager) {
	if webhookPort != 0 {
		return
	}
	if err := (&controllers.ClusterReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Cluster"),
	}).SetupWithManager(mgr, concurrency(clusterConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cluster")
		os.Exit(1)
	}
	if err := (&controllers.MachineReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Machine"),
	}).SetupWithManager(mgr, concurrency(machineConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Machine")
		os.Exit(1)
	}
	if err := (&controllers.MachineSetReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("MachineSet"),
	}).SetupWithManager(mgr, concurrency(machineSetConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineSet")
		os.Exit(1)
	}
	if err := (&controllers.MachineDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("MachineDeployment"),
	}).SetupWithManager(mgr, concurrency(machineDeploymentConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineDeployment")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&expcontrollers.MachinePoolReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("MachinePool"),
		}).SetupWithManager(mgr, concurrency(machinePoolConcurrency)); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "MachinePool")
			os.Exit(1)
		}
	}
	if err := (&controllers.MachineHealthCheckReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("MachineHealthCheck"),
	}).SetupWithManager(mgr, concurrency(machineHealthCheckConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineHealthCheck")
		os.Exit(1)
	}
}

func setupWebhooks(mgr ctrl.Manager) {
	if webhookPort == 0 {
		return
	}

	if err := (&clusterv1alpha2.Cluster{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Cluster")
		os.Exit(1)
	}
	if err := (&clusterv1alpha3.Cluster{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Cluster")
		os.Exit(1)
	}

	if err := (&clusterv1alpha2.ClusterList{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ClusterList")
		os.Exit(1)
	}

	if err := (&clusterv1alpha2.Machine{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Machine")
		os.Exit(1)
	}
	if err := (&clusterv1alpha3.Machine{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Machine")
		os.Exit(1)
	}

	if err := (&clusterv1alpha2.MachineList{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineList")
		os.Exit(1)
	}

	if err := (&clusterv1alpha2.MachineSet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineSet")
		os.Exit(1)
	}
	if err := (&clusterv1alpha3.MachineSet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineSet")
		os.Exit(1)
	}

	if err := (&clusterv1alpha2.MachineSetList{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineSetList")
		os.Exit(1)
	}

	if err := (&clusterv1alpha2.MachineDeployment{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineDeployment")
		os.Exit(1)
	}
	if err := (&clusterv1alpha3.MachineDeployment{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineDeployment")
		os.Exit(1)
	}

	if err := (&clusterv1alpha2.MachineDeploymentList{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineDeploymentList")
		os.Exit(1)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := (&expv1alpha3.MachinePool{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "MachinePool")
			os.Exit(1)
		}
	}

	if err := (&clusterv1alpha3.MachineHealthCheck{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MachineHealthCheck")
		os.Exit(1)
	}
}

func concurrency(c int) controller.Options {
	return controller.Options{MaxConcurrentReconciles: c}
}

// newClientFunc returns a client reads from cache and write directly to the server
// this avoid get unstructured object directly from the server
// see issue: https://github.com/kubernetes-sigs/cluster-api/issues/1663
func newClientFunc(cache cache.Cache, config *rest.Config, options client.Options) (client.Client, error) {
	// Create the Client for Write operations.
	c, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	return &client.DelegatingClient{
		Reader:       cache,
		Writer:       c,
		StatusClient: c,
	}, nil
}
