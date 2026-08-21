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
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/dynamiccache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	contractv1beta1 "sigs.k8s.io/cluster-api/internal/contract/api/v1beta1"
	contractv1 "sigs.k8s.io/cluster-api/internal/contract/api/v1beta2"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
	"sigs.k8s.io/cluster-api/util/secret"
)

// ManagerCacheOptions provides cache.Options for the manager.
func ManagerCacheOptions(scheme *runtime.Scheme, controllerName, watchNamespace string, syncPeriod time.Duration) cache.Options {
	var watchNamespaces map[string]cache.Config
	if watchNamespace != "" {
		watchNamespaces = map[string]cache.Config{
			watchNamespace: {},
		}
	}

	req, _ := labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Exists, nil)
	clusterSecretCacheSelector := labels.NewSelector().Add(*req)

	informerName, err := toolscache.NewInformerName(controllerName)
	if err != nil {
		panic("cache.NewInformerName was called twice with the same name, that should never happen")
	}

	return cache.Options{
		DefaultNamespaces: watchNamespaces,
		SyncPeriod:        &syncPeriod,
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
						if !strings.HasSuffix(s.Name, fmt.Sprintf("-%s", secret.Kubeconfig)) {
							s.Data = nil
						}
					}
					return in, nil
				},
			},
		},
		NewInformer: capicontrollerutil.NewInformerFunc(scheme, informerName),
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
	return clustercache.CacheOptions{
		DefaultTransform: cache.TransformStripManagedFields(),
		Indexes:          []clustercache.CacheOptionsIndex{clustercache.NodeProviderIDIndex},
	}
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
				// Don't cache Pods & DaemonSets (we get/list them e.g. during drain).
				&corev1.Pod{},
				&appsv1.DaemonSet{},
				// Don't cache PersistentVolumes and VolumeAttachments (we get/list them e.g. during wait for volumes to detach)
				&storagev1.VolumeAttachment{},
				&corev1.PersistentVolume{},
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

// Object types used to configure the DynamicCache below.
const (
	DynamicCacheInfraMachineObjectType    dynamiccache.ObjectType = "DynamicCacheInfraMachineObjectType"
	DynamicCacheBootstrapConfigObjectType dynamiccache.ObjectType = "DynamicCacheBootstrapConfigObjectType"
)

// NewDynamicCache creates a new DynamicCache for the core CAPI controller.
func NewDynamicCache(mgr ctrl.Manager, controllerName, watchNamespace string) dynamiccache.DynamicCache {
	return dynamiccache.New(mgr, mgr.GetClient(), DynamicCacheOptions(), controllerName, watchNamespace)
}

// DynamicCacheOptions returns the DynamicCache options used by the core CAPI controller.
func DynamicCacheOptions() map[dynamiccache.ObjectType]dynamiccache.ByObjectTypeOptions {
	return map[dynamiccache.ObjectType]dynamiccache.ByObjectTypeOptions{
		DynamicCacheInfraMachineObjectType: {
			ContractObj: map[string]client.Object{
				"v1beta1": &contractv1beta1.InfraMachine{},
				"v1beta2": &contractv1.InfraMachine{},
			},
			ContractObjList: map[string]client.ObjectList{
				"v1beta1": &contractv1beta1.InfraMachineList{},
				"v1beta2": &contractv1.InfraMachineList{},
			},
		},
		DynamicCacheBootstrapConfigObjectType: {
			ContractObj: map[string]client.Object{
				"v1beta1": &contractv1beta1.BootstrapConfig{},
				"v1beta2": &contractv1.BootstrapConfig{},
			},
			ContractObjList: map[string]client.ObjectList{
				"v1beta1": &contractv1beta1.BootstrapConfigList{},
				"v1beta2": &contractv1.BootstrapConfigList{},
			},
		},
	}
}
