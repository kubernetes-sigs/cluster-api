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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
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

	req, _ = labels.NewRequirement(clusterv1.MachineControlPlaneLabel, selection.Exists, nil)
	controlPlaneMachineSelector := labels.NewSelector().Add(*req)

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
						if !secret.HasPurposeSuffix(s.Name) {
							s.Data = nil
						}
					}
					return in, nil
				},
			},
			&clusterv1.Machine{}: {
				// Drop data of worker Machines.
				// Note: There are code paths that check if there are worker Machines, so it's not possible to only
				// cache CP Machines.
				Transform: func(in any) (any, error) {
					if m, ok := in.(*clusterv1.Machine); ok && !util.IsControlPlaneMachine(m) {
						m.SetManagedFields(nil)
						m.Spec = clusterv1.MachineSpec{}
						m.Status = clusterv1.MachineStatus{}
					}
					return in, nil
				},
			},
			&bootstrapv1.KubeadmConfig{}: {
				// Only cache CP Machine KubeadmConfigs.
				Label: controlPlaneMachineSelector,
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
			// This config ensures that the default client uses the cache for all Unstructured get/list calls.
			// KCP is only using Unstructured to retrieve InfraMachines and InfraMachineTemplates.
			// As the cache should be used in those cases, caching is configured globally instead of
			// creating a separate client that caches Unstructured.
			Unstructured: true,
		},
	}
}

// ClusterCacheCacheOptions provides clustercache.CacheOptions for the ClusterCache.
func ClusterCacheCacheOptions() clustercache.CacheOptions {
	must := func(r *labels.Requirement, err error) labels.Requirement {
		if err != nil {
			panic(err)
		}
		return *r
	}

	podSelector := labels.NewSelector().Add(
		must(labels.NewRequirement("tier", selection.Equals, []string{"control-plane"})),
		must(labels.NewRequirement("component", selection.In, []string{"kube-apiserver", "kube-controller-manager", "kube-scheduler", "etcd"})),
	)

	return clustercache.CacheOptions{
		// Only cache kubeadm static pods
		ByObject: map[client.Object]cache.ByObject{
			&corev1.Pod{}: {
				Namespaces: map[string]cache.Config{
					metav1.NamespaceSystem: {
						LabelSelector: podSelector,
						// Note: This must be aligned to TransformPod in controlplane/kubeadm/internal/clustercache_utils.go
						Transform: func(in any) (any, error) {
							if p, ok := in.(*corev1.Pod); ok {
								p.SetManagedFields(nil)
								p.Spec = corev1.PodSpec{}
							}
							return in, nil
						},
					},
				},
			},
			&corev1.Node{}: {
				// Note: This must be aligned to TransformNode in controlplane/kubeadm/internal/clustercache_utils.go
				Transform: func(in any) (any, error) {
					if n, ok := in.(*corev1.Node); ok {
						n.SetManagedFields(nil)
						n.Spec = corev1.NodeSpec{
							ProviderID: n.Spec.ProviderID,
							Taints:     n.Spec.Taints,
						}
						n.Status = corev1.NodeStatus{
							Conditions: n.Status.Conditions,
						}
					}
					return in, nil
				},
			},
		},
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
				&corev1.ConfigMap{},
				&corev1.Secret{},
				&appsv1.Deployment{},
				&appsv1.DaemonSet{},
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
