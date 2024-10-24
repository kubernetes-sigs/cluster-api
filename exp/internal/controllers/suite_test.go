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

package controllers

import (
	"context"
	"fmt"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
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
		clusterCache, err := clustercache.SetupWithManager(ctx, mgr, clustercache.Options{
			SecretClient: mgr.GetClient(),
			Cache: clustercache.CacheOptions{
				Indexes: []clustercache.CacheOptionsIndex{clustercache.NodeProviderIDIndex},
			},
			Client: clustercache.ClientOptions{
				UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
				Cache: clustercache.ClientCacheOptions{
					DisableFor: []client.Object{
						// Don't cache ConfigMaps & Secrets.
						&corev1.ConfigMap{},
						&corev1.Secret{},
					},
				},
			},
		}, controller.Options{MaxConcurrentReconciles: 10})
		if err != nil {
			panic(fmt.Sprintf("Failed to create ClusterCache: %v", err))
		}

		if err := (&MachinePoolReconciler{
			Client:       mgr.GetClient(),
			APIReader:    mgr.GetAPIReader(),
			ClusterCache: clusterCache,
			recorder:     mgr.GetEventRecorderFor("machinepool-controller"),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to set up machine pool reconciler: %v", err))
		}
	}

	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:                m,
		SetupEnv:         func(e *envtest.Environment) { env = e },
		SetupIndexes:     setupIndexes,
		SetupReconcilers: setupReconcilers,
	}))
}
