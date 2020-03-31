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
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	healthCheckPollInterval       = 10 * time.Second
	healthCheckRequestTimeout     = 5 * time.Second
	healthCheckUnhealthyThreshold = 3
)

// clusterCache embeds cache.Cache and combines it with a stop channel.
type clusterCache struct {
	cache.Cache

	lock    sync.Mutex
	stopped bool
	stop    chan struct{}
}

// Stop closes the cache.Cache's stop channel if it has not already been stopped.
func (cc *clusterCache) Stop() {
	cc.lock.Lock()
	defer cc.lock.Unlock()

	if cc.stopped {
		return
	}

	cc.stopped = true
	close(cc.stop)
}

// ClusterCacheTracker manages client caches for workload clusters.
type ClusterCacheTracker struct {
	log    logr.Logger
	client client.Client

	clusterCachesLock sync.RWMutex
	clusterCaches     map[client.ObjectKey]*clusterCache

	watchesLock sync.RWMutex
	watches     map[client.ObjectKey]map[watchInfo]struct{}
}

// NewClusterCacheTracker creates a new ClusterCacheTracker.
func NewClusterCacheTracker(log logr.Logger, manager ctrl.Manager) (*ClusterCacheTracker, error) {
	m := &ClusterCacheTracker{
		log:           log,
		client:        manager.GetClient(),
		clusterCaches: make(map[client.ObjectKey]*clusterCache),
		watches:       make(map[client.ObjectKey]map[watchInfo]struct{}),
	}

	return m, nil
}

// Watcher is a scoped-down interface from Controller that only knows how to watch.
type Watcher interface {
	// Watch watches src for changes, sending events to eventHandler if they pass predicates.
	Watch(src source.Source, eventHandler handler.EventHandler, predicates ...predicate.Predicate) error
}

// watchInfo is used as a map key to uniquely identify a watch. Because predicates is a slice, it cannot be included.
type watchInfo struct {
	watcher      Watcher
	kind         runtime.Object
	eventHandler handler.EventHandler
}

// watchExists returns true if watch has already been established. This does NOT hold any lock.
func (m *ClusterCacheTracker) watchExists(cluster client.ObjectKey, watch watchInfo) bool {
	watchesForCluster, clusterFound := m.watches[cluster]
	if !clusterFound {
		return false
	}

	_, watchFound := watchesForCluster[watch]
	return watchFound
}

// deleteWatchesForCluster removes the watches for cluster from the tracker.
func (m *ClusterCacheTracker) deleteWatchesForCluster(cluster client.ObjectKey) {
	m.watchesLock.Lock()
	defer m.watchesLock.Unlock()

	delete(m.watches, cluster)
}

// WatchInput specifies the parameters used to establish a new watch for a remote cluster.
type WatchInput struct {
	// Cluster is the key for the remote cluster.
	Cluster client.ObjectKey

	// Watcher is the watcher (controller) whose Reconcile() function will be called for events.
	Watcher Watcher

	// Kind is the type of resource to watch.
	Kind runtime.Object

	// CacheOptions are used to specify options for the remote cache, such as the Scheme to use.
	CacheOptions cache.Options

	// EventHandler contains the event handlers to invoke for resource events.
	EventHandler handler.EventHandler

	// Predicates is used to filter resource events.
	Predicates []predicate.Predicate
}

// Watch watches a remote cluster for resource events. If the watch already exists based on cluster, watcher,
// kind, and eventHandler, then this is a no-op.
func (m *ClusterCacheTracker) Watch(ctx context.Context, input WatchInput) error {
	wi := watchInfo{
		watcher:      input.Watcher,
		kind:         input.Kind,
		eventHandler: input.EventHandler,
	}

	// First, check if the watch already exists
	var exists bool
	m.watchesLock.RLock()
	exists = m.watchExists(input.Cluster, wi)
	m.watchesLock.RUnlock()

	if exists {
		m.log.V(4).Info("Watch already exists", "namespace", input.Cluster.Namespace, "cluster", input.Cluster.Name, "kind", fmt.Sprintf("%T", input.Kind))
		return nil
	}

	// Doesn't exist - grab the write lock
	m.watchesLock.Lock()
	defer m.watchesLock.Unlock()

	// Need to check if another goroutine created the watch while this one was waiting for the lock
	if m.watchExists(input.Cluster, wi) {
		m.log.V(4).Info("Watch already exists", "namespace", input.Cluster.Namespace, "cluster", input.Cluster.Name, "kind", fmt.Sprintf("%T", input.Kind))
		return nil
	}

	// Need to create the watch
	watchesForCluster, found := m.watches[input.Cluster]
	if !found {
		watchesForCluster = make(map[watchInfo]struct{})
		m.watches[input.Cluster] = watchesForCluster
	}

	cache, err := m.getOrCreateClusterCache(ctx, input.Cluster, input.CacheOptions)
	if err != nil {
		return err
	}

	if err := input.Watcher.Watch(source.NewKindWithCache(input.Kind, cache), input.EventHandler, input.Predicates...); err != nil {
		return errors.Wrap(err, "error creating watch")
	}

	watchesForCluster[wi] = struct{}{}

	return nil
}

// getOrCreateClusterCache returns the clusterCache for cluster, creating a new ClusterCache if needed.
func (m *ClusterCacheTracker) getOrCreateClusterCache(ctx context.Context, cluster client.ObjectKey, cacheOptions cache.Options) (*clusterCache, error) {
	cache := m.getClusterCache(cluster)
	if cache != nil {
		return cache, nil
	}

	return m.newClusterCache(ctx, cluster, cacheOptions)
}

// getClusterCache returns the clusterCache for cluster, or nil if it does not exist.
func (m *ClusterCacheTracker) getClusterCache(cluster client.ObjectKey) *clusterCache {
	m.clusterCachesLock.RLock()
	defer m.clusterCachesLock.RUnlock()

	return m.clusterCaches[cluster]
}

// newClusterCache creates and starts a new clusterCache for cluster.
func (m *ClusterCacheTracker) newClusterCache(ctx context.Context, cluster client.ObjectKey, cacheOptions cache.Options) (*clusterCache, error) {
	m.clusterCachesLock.Lock()
	defer m.clusterCachesLock.Unlock()

	// If another goroutine created the cache while this one was waiting to acquire the write lock, return that
	// instead of overwriting it.
	if c, exists := m.clusterCaches[cluster]; exists {
		return c, nil
	}

	config, err := RESTConfig(ctx, m.client, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching REST client config for remote cluster")
	}

	remoteCache, err := cache.New(config, cacheOptions)
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
	// Start cluster healthcheck!!!
	go m.healthCheckCluster(&healthCheckInput{
		stop:    cc.stop,
		cluster: cluster,
		cfg:     config,
	})

	return cc, nil
}

func (m *ClusterCacheTracker) deleteClusterCache(cluster client.ObjectKey) {
	m.clusterCachesLock.Lock()
	defer m.clusterCachesLock.Unlock()

	delete(m.clusterCaches, cluster)
}

// healthCheckInput provides the input for the healthCheckCluster method
type healthCheckInput struct {
	stop               <-chan struct{}
	cluster            client.ObjectKey
	cfg                *rest.Config
	interval           time.Duration
	requestTimeout     time.Duration
	unhealthyThreshold int
	path               string
}

// validate sets default values if optional parameters are not set
func (h *healthCheckInput) validate() {
	if h.interval == 0 {
		h.interval = healthCheckPollInterval
	}
	if h.requestTimeout == 0 {
		h.requestTimeout = healthCheckRequestTimeout
	}
	if h.unhealthyThreshold == 0 {
		h.unhealthyThreshold = healthCheckUnhealthyThreshold
	}
	if h.path == "" {
		h.path = "/"
	}
}

// healthCheckCluster will poll the cluster's API at the path given and, if there are
// `unhealthyThreshold` consecutive failures, will deem the cluster unhealthy.
// Once the cluster is deemed unhealthy, the cluster's cache is stopped and removed.
func (m *ClusterCacheTracker) healthCheckCluster(in *healthCheckInput) {
	// populate optional params for healthCheckInput
	in.validate()

	unhealthyCount := 0

	runHealthCheckWithThreshold := func() (bool, error) {
		remoteCache := m.getClusterCache(in.cluster)
		if remoteCache == nil {
			// Cache for this cluster has already been cleaned up.
			// Nothing to do, so return true.
			return true, nil
		}

		// healthCheckPath returning an error is considered a failed health check
		// (Either an issue was encountered connecting or the API returned an error).
		// If no error occurs, reset the unhealthy coutner.
		err := healthCheckPath(in.cfg, in.requestTimeout, in.path)
		if err != nil {
			unhealthyCount++
		} else {
			unhealthyCount = 0
		}

		if unhealthyCount >= in.unhealthyThreshold {
			// `healthCheckUnhealthyThreshold` (or more) consecutive failures.
			// Cluster is now considered unhealthy.
			// return last error from `doHealthCheck`
			return false, err
		}

		return false, nil
	}

	err := wait.PollImmediateUntil(in.interval, runHealthCheckWithThreshold, in.stop)
	// An error returned implies the health check has failed a sufficient number of
	// times for the cluster to be considered unhealthy
	if err != nil {
		c := m.getClusterCache(in.cluster)
		if c == nil {
			return
		}

		// Stop the cache and clean up
		c.Stop()
		m.deleteClusterCache(in.cluster)
		m.deleteWatchesForCluster(in.cluster)
	}
}

// healthCheckPath attempts to request a given absolute path from the API server
// defined in the rest.Config and returns any errors that occurred during the request.
func healthCheckPath(sourceCfg *rest.Config, requestTimeout time.Duration, path string) error {
	codec := runtime.NoopEncoder{Decoder: scheme.Codecs.UniversalDecoder()}
	cfg := rest.CopyConfig(sourceCfg)
	cfg.NegotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{Serializer: codec})

	restClient, err := rest.UnversionedRESTClientFor(cfg)
	if err != nil {
		// Config is invalid, cannot perform health checks
		return err
	}

	_, err = restClient.Get().AbsPath(path).Timeout(requestTimeout).Do().Get()
	if err != nil {
		return err
	}

	return nil
}

// ClusterCacheReconciler is responsible for stopping remote cluster caches when
// the cluster for the remote cache is being deleted.
type ClusterCacheReconciler struct {
	log     logr.Logger
	client  client.Client
	tracker *ClusterCacheTracker
}

func NewClusterCacheReconciler(
	log logr.Logger,
	mgr ctrl.Manager,
	controllerOptions controller.Options,
	cct *ClusterCacheTracker,
) (*ClusterCacheReconciler, error) {
	r := &ClusterCacheReconciler{
		log:     log,
		client:  mgr.GetClient(),
		tracker: cct,
	}

	// Watch Clusters so we can stop and remove caches when Clusters are deleted.
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		WithOptions(controllerOptions).
		Build(r)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create cluster cache manager controller")
	}

	return r, nil
}

// Reconcile reconciles Clusters and removes ClusterCaches for any Cluster that cannot be retrieved from the
// management cluster.
func (r *ClusterCacheReconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()

	log := r.log.WithValues("namespace", req.Namespace, "name", req.Name)
	log.V(4).Info("Reconciling")

	var cluster clusterv1.Cluster

	err := r.client.Get(ctx, req.NamespacedName, &cluster)
	if err == nil {
		log.V(4).Info("Cluster still exists")
		return reconcile.Result{}, nil
	} else if !kerrors.IsNotFound(err) {
		log.Error(err, "Error retrieving cluster")
		return reconcile.Result{}, err
	}

	log.V(4).Info("Cluster no longer exists")

	c := r.tracker.getClusterCache(req.NamespacedName)
	if c == nil {
		log.V(4).Info("No current cluster cache exists - nothing to do")
		return reconcile.Result{}, nil
	}

	log.V(4).Info("Stopping cluster cache")
	c.Stop()

	r.tracker.deleteClusterCache(req.NamespacedName)

	log.V(4).Info("Deleting watches for cluster cache")
	r.tracker.deleteWatchesForCluster(req.NamespacedName)

	return reconcile.Result{}, nil
}
