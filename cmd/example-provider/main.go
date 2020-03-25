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
	"math/rand"
	"os"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	klog.InitFlags(nil)
	var enableLeaderElection bool
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	var metricsAddr string
	var healthAddr string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080",
		"The address the metric endpoint binds to.")
	flag.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")
	flag.Parse()

	cfg := ctrl.GetConfigOrDie()

	// Setup a Manager
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "controller-leader-election-capi-example",
		MetricsBindAddress:     metricsAddr,
		HealthProbeBindAddress: healthAddr,
	})
	if err != nil {
		klog.Fatalf("Failed to set up controller manager: %v", err)
	}

	if err := clusterv1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatal(err)
	}

	if err = (&controllers.ClusterReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Cluster"),
	}).SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		os.Exit(1)
	}
	if err = (&controllers.MachineReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Machine"),
	}).SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		klog.Fatalf("unable to create health check: %v", err)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		klog.Fatalf("unable to create health check: %v", err)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Fatalf("Failed to run manager: %v", err)
	}
}
