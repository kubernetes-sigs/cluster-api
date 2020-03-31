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

package remote

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ClusterCache extends cache.Cache and adds the ability to stop the cache.
type ClusterCache interface {
	cache.Cache

	// Stop stops the cache.
	Stop()
}

// clusterCache embeds cache.Cache and combines it with a stop channel.
type clusterCache struct {
	cache.Cache

	lock    sync.Mutex
	stopped bool
	stop    chan struct{}
}

var _ ClusterCache = &clusterCache{}

func (cc *clusterCache) Stop() {
	cc.lock.Lock()
	defer cc.lock.Unlock()

	if cc.stopped {
		return
	}

	cc.stopped = true
	close(cc.stop)
}

// ClusterCacheManager manages client caches for workload clusters.
type ClusterCacheManager struct {
	log                     logr.Logger
	managementClusterClient client.Client
	scheme                  *runtime.Scheme

	lock          sync.RWMutex
	clusterCaches map[client.ObjectKey]ClusterCache

	// For testing
	newRESTConfig func(ctx context.Context, cluster client.ObjectKey) (*rest.Config, error)
}

// NewClusterCacheManagerInput is used as input when creating a ClusterCacheManager and contains all the fields
// necessary for its construction.
type NewClusterCacheManagerInput struct {
	Log                     logr.Logger
	Manager                 ctrl.Manager
	ManagementClusterClient client.Client
	Scheme                  *runtime.Scheme
	ControllerOptions       controller.Options
}

// NewClusterCacheManager creates a new ClusterCacheManager.
func NewClusterCacheManager(input NewClusterCacheManagerInput) (*ClusterCacheManager, error) {
	m := &ClusterCacheManager{
		log:                     input.Log,
		managementClusterClient: input.ManagementClusterClient,
		scheme:                  input.Scheme,
		clusterCaches:           make(map[client.ObjectKey]ClusterCache),
	}

	// Make sure we're using the default new cluster cache function.
	m.newRESTConfig = m.defaultNewRESTConfig

	// Watch Clusters so we can stop and remove caches when Clusters are deleted.
	_, err := ctrl.NewControllerManagedBy(input.Manager).
		For(&clusterv1.Cluster{}).
		WithOptions(input.ControllerOptions).
		Build(m)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create cluster cache manager controller")
	}

	return m, nil
}

// ClusterCache returns the ClusterCache for cluster, creating a new ClusterCache if needed.
func (m *ClusterCacheManager) ClusterCache(ctx context.Context, cluster client.ObjectKey) (ClusterCache, error) {
	cache := m.getClusterCache(cluster)
	if cache != nil {
		return cache, nil
	}

	return m.newClusterCache(ctx, cluster)
}

// getClusterCache returns the ClusterCache for cluster, or nil if it does not exist.
func (m *ClusterCacheManager) getClusterCache(cluster client.ObjectKey) ClusterCache {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.clusterCaches[cluster]
}

// defaultNewClusterCache creates and starts a new ClusterCache for cluster.
func (m *ClusterCacheManager) newClusterCache(ctx context.Context, cluster client.ObjectKey) (ClusterCache, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// If another goroutine created the cache while this one was waiting to acquire the write lock, return that
	// instead of overwriting it.
	if c, exists := m.clusterCaches[cluster]; exists {
		return c, nil
	}

	config, err := m.newRESTConfig(ctx, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "error creating REST client config for remote cluster")
	}

	remoteCache, err := cache.New(config, cache.Options{})
	if err != nil {
		return nil, errors.Wrap(err, "error creating cache for remote cluster")
	}
	stop := make(chan struct{})

	cc := &clusterCache{
		Cache: remoteCache,
		stop:  stop,
	}
	m.clusterCaches[cluster] = cc

	// Start the cache!!!
	go remoteCache.Start(cc.stop)

	return cc, nil
}

func (m *ClusterCacheManager) defaultNewRESTConfig(ctx context.Context, cluster client.ObjectKey) (*rest.Config, error) {
	config, err := RESTConfig(ctx, m.managementClusterClient, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching remote cluster config")
	}
	return config, nil
}

// Reconcile reconciles Clusters and removes ClusterCaches for any Cluster that cannot be retrieved from the
// management cluster.
func (m *ClusterCacheManager) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()

	log := m.log.WithValues("namespace", req.Namespace, "name", req.Name)
	log.V(4).Info("Reconciling")

	var cluster clusterv1.Cluster

	err := m.managementClusterClient.Get(ctx, req.NamespacedName, &cluster)
	if err == nil {
		log.V(4).Info("Cluster still exists")
		return reconcile.Result{}, nil
	}

	log.V(4).Info("Error retrieving cluster", "error", err.Error())

	c := m.getClusterCache(req.NamespacedName)
	if c == nil {
		log.V(4).Info("No current cluster cache exists - nothing to do")
	}

	log.V(4).Info("Stopping cluster cache")
	c.Stop()

	delete(m.clusterCaches, req.NamespacedName)

	return reconcile.Result{}, nil
}
