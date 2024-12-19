/*
Copyright 2020 The Kubernetes Authors.

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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

var (
	env          *envtest.Environment
	clusterCache clustercache.ClusterCache
	ctx          = ctrl.SetupSignalHandler()
)

func TestMain(m *testing.M) {
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}

	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		// Create partial cache analog to main.go.
		partialSecretCache, err := cache.New(mgr.GetConfig(), cache.Options{
			Scheme:     mgr.GetScheme(),
			Mapper:     mgr.GetRESTMapper(),
			HTTPClient: mgr.GetHTTPClient(),
			SyncPeriod: ptr.To(time.Minute * 10),
			DefaultTransform: func(in interface{}) (interface{}, error) {
				// Use DefaultTransform to drop objects we don't expect to get into this cache.
				obj, ok := in.(*metav1.PartialObjectMetadata)
				if !ok {
					panic(fmt.Sprintf("cache expected to only get PartialObjectMetadata, got %T", in))
				}
				if obj.GetObjectKind().GroupVersionKind() != corev1.SchemeGroupVersion.WithKind("Secret") {
					panic(fmt.Sprintf("cache expected to only get Secrets, got %s", obj.GetObjectKind()))
				}
				// Additionally strip managed fields.
				return cache.TransformStripManagedFields()(obj)
			},
		})
		if err != nil {
			panic(fmt.Sprintf("Failed to create cache for metadata only Secret watches: %v", err))
		}
		if err := mgr.Add(partialSecretCache); err != nil {
			panic(fmt.Sprintf("Failed to start cache for metadata only Secret watches: %v", err))
		}

		clusterCache, err = clustercache.SetupWithManager(ctx, mgr, clustercache.Options{
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

		reconciler := ClusterResourceSetReconciler{
			Client:       mgr.GetClient(),
			ClusterCache: clusterCache,
		}
		if err = reconciler.SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 10}, partialSecretCache); err != nil {
			panic(fmt.Sprintf("Failed to set up cluster resource set reconciler: %v", err))
		}
		bindingReconciler := ClusterResourceSetBindingReconciler{
			Client: mgr.GetClient(),
		}
		if err = bindingReconciler.SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 10}); err != nil {
			panic(fmt.Sprintf("Failed to set up cluster resource set binding reconciler: %v", err))
		}
	}

	req, _ := labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Exists, nil)
	clusterSecretCacheSelector := labels.NewSelector().Add(*req)
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:        m,
		SetupEnv: func(e *envtest.Environment) { env = e },
		CacheOptionsModifier: func(o *cache.Options) {
			o.ByObject = map[client.Object]cache.ByObject{
				// Only cache Secrets with the cluster name label.
				// This is similar to the real world.
				&corev1.Secret{}: {
					Label: clusterSecretCacheSelector,
				},
			}
		},
		ManagerUncachedObjs: []client.Object{
			&corev1.ConfigMap{},
			&corev1.Secret{},
			&addonsv1.ClusterResourceSetBinding{},
		},
		SetupIndexes:     setupIndexes,
		SetupReconcilers: setupReconcilers,
	}))
}
