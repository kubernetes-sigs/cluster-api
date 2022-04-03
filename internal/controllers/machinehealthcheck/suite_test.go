/*
Copyright 2022 The Kubernetes Authors.

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

package machinehealthcheck

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	// +kubebuilder:scaffold:imports
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/remote"
	clustercontroller "sigs.k8s.io/cluster-api/internal/controllers/cluster"
	machinecontroller "sigs.k8s.io/cluster-api/internal/controllers/machine"
	machinesetcontroller "sigs.k8s.io/cluster-api/internal/controllers/machineset"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

const (
	timeout         = time.Second * 30
	testClusterName = "test-cluster"
)

var (
	env        *envtest.Environment
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = clusterv1.AddToScheme(fakeScheme)
	_ = apiextensionsv1.AddToScheme(fakeScheme)
}

func TestMain(m *testing.M) {
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}

	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
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
			panic(fmt.Sprintf("unable to create cluster cache tracker: %v", err))
		}
		if err := (&remote.ClusterCacheReconciler{
			Client:  mgr.GetClient(),
			Tracker: tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start ClusterCacheReconciler: %v", err))
		}
		if err := (&clustercontroller.Reconciler{
			Client:    mgr.GetClient(),
			APIReader: mgr.GetClient(),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start ClusterReconciler: %v", err))
		}
		if err := (&Reconciler{
			Client:  mgr.GetClient(),
			Tracker: tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start Reconciler : %v", err))
		}
		if err := (&machinecontroller.Reconciler{
			Client:    mgr.GetClient(),
			APIReader: mgr.GetAPIReader(),
			Tracker:   tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MachineReconciler: %v", err))
		}
		if err := (&machinesetcontroller.Reconciler{
			Client:    mgr.GetClient(),
			APIReader: mgr.GetAPIReader(),
			Tracker:   tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MMachineSetReconciler: %v", err))
		}
	}

	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(timeout)

	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:                m,
		SetupEnv:         func(e *envtest.Environment) { env = e },
		SetupIndexes:     setupIndexes,
		SetupReconcilers: setupReconcilers,
	}))
}
