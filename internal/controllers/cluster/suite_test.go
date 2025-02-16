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

package cluster

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	machinecontroller "sigs.k8s.io/cluster-api/internal/controllers/machine"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

const (
	timeout = time.Second * 30
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
		go func() {
			<-ctx.Done()
			clusterCache.(interface{ Shutdown() }).Shutdown()
		}()

		// Setting ConnectionCreationRetryInterval to 2 seconds, otherwise client creation is
		// only retried every 30s. If we get unlucky tests are then failing with timeout.
		clusterCache.(interface{ SetConnectionCreationRetryInterval(time.Duration) }).
			SetConnectionCreationRetryInterval(2 * time.Second)

		if err := (&Reconciler{
			Client:                      mgr.GetClient(),
			APIReader:                   mgr.GetClient(),
			ClusterCache:                clusterCache,
			RemoteConnectionGracePeriod: 50 * time.Second,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start ClusterReconciler: %v", err))
		}
		if err := (&machinecontroller.Reconciler{
			Client:                      mgr.GetClient(),
			APIReader:                   mgr.GetAPIReader(),
			ClusterCache:                clusterCache,
			RemoteConditionsGracePeriod: 5 * time.Minute,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MachineReconciler: %v", err))
		}
	}

	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(timeout)

	req, _ := labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Exists, nil)
	clusterSecretCacheSelector := labels.NewSelector().Add(*req)

	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M: m,
		ManagerCacheOptions: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				// Only cache Secrets with the cluster name label.
				// This is similar to the real world.
				&corev1.Secret{}: {
					Label: clusterSecretCacheSelector,
				},
			},
		},
		ManagerUncachedObjs: []client.Object{
			&corev1.ConfigMap{},
			&corev1.Secret{},
		},
		SetupEnv:         func(e *envtest.Environment) { env = e },
		SetupIndexes:     setupIndexes,
		SetupReconcilers: setupReconcilers,
	}))
}
