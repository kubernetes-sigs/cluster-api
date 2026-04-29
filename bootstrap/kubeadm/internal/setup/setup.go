/*
Copyright 2026 The Kubernetes Authors.

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

// Package setup provides utils for the setup of CABPK.
package setup

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/secret"
)

// ManagerCacheOptions provides cache.Options for the manager.
func ManagerCacheOptions(watchNamespace string, syncPeriod time.Duration) cache.Options {
	var watchNamespaces map[string]cache.Config
	if watchNamespace != "" {
		watchNamespaces = map[string]cache.Config{
			watchNamespace: {},
		}
	}

	req, _ := labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Exists, nil)
	clusterSecretCacheSelector := labels.NewSelector().Add(*req)

	return cache.Options{
		DefaultNamespaces: watchNamespaces,
		SyncPeriod:        &syncPeriod,
		DefaultTransform:  cache.TransformStripManagedFields(),
		ByObject: map[client.Object]cache.ByObject{
			// Note: Only Secrets with the cluster name label are cached.
			// The default client of the manager won't use the cache for secrets at all (see Client.Cache.DisableFor).
			// The cached secrets will only be used by the secretCachingClient we create below.
			&corev1.Secret{}: {
				Label: clusterSecretCacheSelector,
				// Drop data of secrets that we don't use.
				Transform: func(in any) (any, error) {
					if s, ok := in.(*corev1.Secret); ok {
						s.SetManagedFields(nil)
						if !secret.HasPurposeSuffix(s.Name) {
							s.Data = nil
						}
					}
					return in, nil
				},
			},
			&clusterv1.MachineSet{}: {
				// Drop data of MachineSets as we only use ownerRefs of MachineSets in clog.AddOwners.
				Transform: func(in any) (any, error) {
					if m, ok := in.(*clusterv1.MachineSet); ok {
						m.SetManagedFields(nil)
						m.Spec = clusterv1.MachineSetSpec{}
						m.Status = clusterv1.MachineSetStatus{}
					}
					return in, nil
				},
			},
		},
	}
}

// ManagerClientOptions provides client.Options for the manager.
func ManagerClientOptions() client.Options {
	return client.Options{
		Cache: &client.CacheOptions{
			DisableFor: []client.Object{
				&corev1.ConfigMap{},
				&corev1.Secret{},
			},
			// Use the cache for all Unstructured get/list calls.
			Unstructured: true,
		},
	}
}

// ClusterCacheCacheOptions provides clustercache.CacheOptions for the ClusterCache.
func ClusterCacheCacheOptions() clustercache.CacheOptions {
	return clustercache.CacheOptions{}
}

// ClusterCacheClientOptions provides clustercache.ClientOptions for the ClusterCache.
func ClusterCacheClientOptions(controllerName string, qps float32, burst int) clustercache.ClientOptions {
	return clustercache.ClientOptions{
		QPS:       qps,
		Burst:     burst,
		UserAgent: remote.DefaultClusterAPIUserAgent(controllerName),
		Cache: clustercache.ClientCacheOptions{
			DisableFor: []client.Object{
				// Don't cache ConfigMaps & Secrets.
				&corev1.ConfigMap{},
				&corev1.Secret{},
			},
		},
	}
}

// CreateSecretCachingClient creates a secret caching client that should be used when accessing cached
// clients on the management cluster.
func CreateSecretCachingClient(mgr ctrl.Manager) (client.Client, error) {
	return client.New(mgr.GetConfig(), client.Options{
		HTTPClient: mgr.GetHTTPClient(),
		Cache: &client.CacheOptions{
			Reader: mgr.GetCache(),
		},
	})
}
