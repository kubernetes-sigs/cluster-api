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

	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	machinecontroller "sigs.k8s.io/cluster-api/internal/controllers/machine"
	machinesetcontroller "sigs.k8s.io/cluster-api/internal/controllers/machineset"
	"sigs.k8s.io/cluster-api/internal/setup"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

const (
	timeout         = time.Second * 30
	testClusterName = "test-cluster"
)

var (
	env *envtest.Environment
	ctx = ctrl.SetupSignalHandler()
)

func TestMain(m *testing.M) {
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}

	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		secretCachingClient, err := setup.CreateSecretCachingClient(mgr)
		if err != nil {
			panic(fmt.Sprintf("Unable to create secret caching client: %s", err))
		}

		clusterCache, err := clustercache.SetupWithManager(ctx, mgr, clustercache.Options{
			SecretClient: secretCachingClient,
			Cache:        setup.ClusterCacheCacheOptions(),
			Client:       setup.ClusterCacheClientOptions("test-controller-manager", 20, 30),
		}, controller.Options{MaxConcurrentReconciles: 10})
		if err != nil {
			panic(fmt.Sprintf("Failed to create ClusterCache: %v", err))
		}
		go func() {
			<-ctx.Done()
			clusterCache.(interface{ Shutdown() }).Shutdown()
		}()

		// Setting ConnectionCreationRetryInterval to 2 seconds, otherwise client creation is
		// only retried every 30s. If we get unlucky tests are then failing with timeout.
		clusterCache.(interface{ SetConnectionCreationRetryInterval(time.Duration) }).
			SetConnectionCreationRetryInterval(2 * time.Second)

		if err := (&Reconciler{
			Client:       mgr.GetClient(),
			ClusterCache: clusterCache,
			// use a shorter rate limit for testing.
			overrideRateLimit: 1 * time.Second,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start Reconciler : %v", err))
		}
		if err := (&machinecontroller.Reconciler{
			Client:                      mgr.GetClient(),
			APIReader:                   mgr.GetAPIReader(),
			ClusterCache:                clusterCache,
			RemoteConditionsGracePeriod: 5 * time.Minute,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MachineReconciler: %v", err))
		}
		if err := (&machinesetcontroller.Reconciler{
			Client:       mgr.GetClient(),
			APIReader:    mgr.GetAPIReader(),
			ClusterCache: clusterCache,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MMachineSetReconciler: %v", err))
		}
	}

	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(timeout)

	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:                    m,
		ManagerCacheOptions:  setup.ManagerCacheOptions("", 10*time.Minute),
		ManagerClientOptions: setup.ManagerClientOptions(),
		SetupEnv:             func(e *envtest.Environment) { env = e },
		SetupIndexes:         setupIndexes,
		SetupReconcilers:     setupReconcilers,
	}))
}
