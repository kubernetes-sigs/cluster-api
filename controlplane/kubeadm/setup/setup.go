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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/pkg/dynamiccache"
	"sigs.k8s.io/cluster-api/util"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
	"sigs.k8s.io/cluster-api/util/secret"
)

// ManagerCacheOptions provides cache.Options for the manager.
func ManagerCacheOptions(scheme *runtime.Scheme, controllerName, watchNamespace string, syncPeriod time.Duration) ctrlcache.Options {
	var watchNamespaces map[string]ctrlcache.Config
	if watchNamespace != "" {
		watchNamespaces = map[string]ctrlcache.Config{
			watchNamespace: {},
		}
	}

	req, _ := labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Exists, nil)
	clusterSecretCacheSelector := labels.NewSelector().Add(*req)

	req, _ = labels.NewRequirement(clusterv1.MachineControlPlaneLabel, selection.Exists, nil)
	controlPlaneMachineSelector := labels.NewSelector().Add(*req)

	informerName, err := toolscache.NewInformerName(controllerName)
	if err != nil {
		panic("cache.NewInformerName was called twice with the same name, that should never happen")
	}

	return ctrlcache.Options{
		DefaultNamespaces: watchNamespaces,
		SyncPeriod:        &syncPeriod,
		DefaultTransform:  ctrlcache.TransformStripManagedFields(),
		ByObject: map[client.Object]ctrlcache.ByObject{
			// mgr.GetClient() (that is configured via ManagerClientOptions) will never read secrets from the cache.
			// secretCachingClient (that is configured via CreateSecretCachingClient) will read secrets from the cache.
			&corev1.Secret{}: {
				// We only cache secrets that have the cluster-name label
				Label: clusterSecretCacheSelector,
				// We cache we are only keeping the data for secrets with -kubeconfig suffix
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
				// We cannot use a label selector to only cache CP Machines, because there are code paths that
				// check if there are worker Machines.
				// We are dropping data of worker Machines.
				// Intentionally keeping the managedFields for CP Machines for ssa.MitigateManagedFieldsIssue/MigrateManagedFields.
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
				// We only cache CP KubeadmConfigs.
				Label: controlPlaneMachineSelector,
				// Intentionally keeping the managedFields for CP KubeadmConfigs for ssa.MitigateManagedFieldsIssue/MigrateManagedFields.
				// This Transform configuration is needed to overwrite the DefaultTransform from ctrlcache.Options.DefaultTransform.
				Transform: func(in any) (any, error) {
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
			// Use the cache for all Unstructured get/list calls that are done with this client.
			// Some Unstructured get/list calls are done via the dynamic cache created via NewDynamicCache.
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
		DefaultTransform: ctrlcache.TransformStripManagedFields(),
		ByObject: map[client.Object]ctrlcache.ByObject{
			&corev1.Pod{}: {
				Namespaces: map[string]ctrlcache.Config{
					metav1.NamespaceSystem: {
						// We only cache kubeadm static pods for Kubernetes control plane components.
						LabelSelector: podSelector,
						// We are dropping managedFields and spec.
						// This must be aligned to TransformPod in controlplane/kubeadm/pkg/clustercache_utils.go
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
				// We cannot use a label selector to only cache CP Nodes, because we have to cover cases
				// where the `node-role.kubernetes.io/control-plane` label has been removed.
				// We are dropping all the fields that we are not reading.
				// This must be aligned to TransformNode in controlplane/kubeadm/pkg/clustercache_utils.go
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
				// Don't cache ConfigMaps & Secrets.
				&corev1.ConfigMap{},
				&corev1.Secret{},
				&appsv1.Deployment{},
				&appsv1.DaemonSet{},
			},
		},
	}
}

// CreateSecretCachingClient creates a secret caching client that should be used when accessing cached
// secrets on the management cluster.
// The backing cache is configured in ManagerCacheOptions and only a subset of the secrets is cached.
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
	DynamicCacheInfraMachineObjectType         dynamiccache.ObjectType = "InfraMachine"
	DynamicCacheInfraMachineTemplateObjectType dynamiccache.ObjectType = "InfraMachineTemplate"
)

// NewDynamicCache creates a new DynamicCache for the KubeadmControlPlane controller.
func NewDynamicCache(mgr ctrl.Manager, controllerName, watchNamespace string) (dynamiccache.DynamicCache, error) {
	return dynamiccache.New(mgr, DynamicCacheOptions(), controllerName, watchNamespace)
}

// DynamicCacheOptions returns the DynamicCache options used by the KubeadmControlPlane controller.
func DynamicCacheOptions() map[dynamiccache.ObjectType]dynamiccache.ByObjectTypeOptions {
	req, _ := labels.NewRequirement(clusterv1.MachineControlPlaneLabel, selection.Exists, nil)
	controlPlaneMachineSelector := labels.NewSelector().Add(*req)

	return map[dynamiccache.ObjectType]dynamiccache.ByObjectTypeOptions{
		DynamicCacheInfraMachineObjectType: {
			IsUnstructured: new(true),
			// We only cache CP InfraMachines.
			Label: controlPlaneMachineSelector,
			// Intentionally keeping the managedFields for CP InfraMachines for ssa.MitigateManagedFieldsIssue/MigrateManagedFields.
		},
		DynamicCacheInfraMachineTemplateObjectType: {
			IsUnstructured: new(true),
			// We cannot use a label selector to only cache CP InfraMachineTemplates, because there is
			// no way to identify them via label.
			// We are dropping all data from worker InfraMachineTemplates because we don't use them.
			// This only works for Clusters with ClusterClass because only they have a label which
			// allows us to identify worker InfraMachineTemplates.
			Transform: func(in any) (any, error) {
				if imt, ok := in.(*unstructured.Unstructured); ok {
					imt.SetManagedFields(nil)
					if _, ok := imt.GetLabels()[clusterv1.ClusterTopologyMachineDeploymentNameLabel]; ok {
						delete(imt.Object, "spec")
						delete(imt.Object, "status")
					}
				}
				return in, nil
			},
		},
	}
}
