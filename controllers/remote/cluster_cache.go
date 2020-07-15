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
	"reflect"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	defaultClientTimeout = 10 * time.Second

	healthCheckPollInterval       = 10 * time.Second
	healthCheckRequestTimeout     = 5 * time.Second
	healthCheckUnhealthyThreshold = 10
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
	scheme *runtime.Scheme

	delegatingClientsLock sync.RWMutex
	delegatingClients     map[client.ObjectKey]*client.DelegatingClient

	clusterCachesLock sync.RWMutex
	clusterCaches     map[client.ObjectKey]*clusterCache

	watchesLock sync.RWMutex
	watches     map[client.ObjectKey]map[watchInfo]struct{}
}

// NewClusterCacheTracker creates a new ClusterCacheTracker.
func NewClusterCacheTracker(log logr.Logger, manager ctrl.Manager) (*ClusterCacheTracker, error) {
	m := &ClusterCacheTracker{
		log:               log,
		client:            manager.GetClient(),
		scheme:            manager.GetScheme(),
		delegatingClients: make(map[client.ObjectKey]*client.DelegatingClient),
		clusterCaches:     make(map[client.ObjectKey]*clusterCache),
		watches:           make(map[client.ObjectKey]map[watchInfo]struct{}),
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
	watcher Watcher
	gvk     schema.GroupVersionKind

	// Comparing the eventHandler as an interface doesn't work because reflect.DeepEqual
	// will assert functions are false if they are non-nil.
	// Use a signature string representation instead as this can be compared.
	// The signature function is expected to produce a unique output for each unique handler
	// function that is passed to it.
	// In combination with the watcher, this should be enough to identify unique watches.
	eventHandlerSignature string
}

// eventHandlerSignature generates a unique identifier for the given eventHandler by
// printing it to a string using "%#v".
// Eg "&handler.EnqueueRequestsFromMapFunc{ToRequests:(handler.ToRequestsFunc)(0x271afb0)}"
func eventHandlerSignature(h handler.EventHandler) string {
	return fmt.Sprintf("%#v", h)
}

// watchExists returns true if watch has already been established. This does NOT hold any lock.
func (m *ClusterCacheTracker) watchExists(cluster client.ObjectKey, watch watchInfo) bool {
	watchesForCluster, clusterFound := m.watches[cluster]
	if !clusterFound {
		return false
	}

	for w := range watchesForCluster {
		if reflect.DeepEqual(w, watch) {
			return true
		}
	}
	return false
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

	// EventHandler contains the event handlers to invoke for resource events.
	EventHandler handler.EventHandler

	// Predicates is used to filter resource events.
	Predicates []predicate.Predicate
}

// Watch watches a remote cluster for resource events. If the watch already exists based on cluster, watcher,
// kind, and eventHandler, then this is a no-op.
func (m *ClusterCacheTracker) Watch(ctx context.Context, input WatchInput) error {
	gvk, err := apiutil.GVKForObject(input.Kind, m.scheme)
	if err != nil {
		return err
	}

	wi := watchInfo{
		watcher:               input.Watcher,
		gvk:                   gvk,
		eventHandlerSignature: eventHandlerSignature(input.EventHandler),
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

	cache, err := m.getOrCreateClusterCache(ctx, input.Cluster)
	if err != nil {
		return err
	}

	if err := input.Watcher.Watch(source.NewKindWithCache(input.Kind, cache), input.EventHandler, input.Predicates...); err != nil {
		return errors.Wrap(err, "error creating watch")
	}

	watchesForCluster[wi] = struct{}{}

	return nil
}

// GetClient returns a client for the given cluster.
func (m *ClusterCacheTracker) GetClient(ctx context.Context, cluster client.ObjectKey) (client.Client, error) {
	return m.getOrCreateDelegatingClient(ctx, cluster)
}

// getOrCreateClusterClient returns a delegating client for the specified cluster, creating a new one if needed.
func (m *ClusterCacheTracker) getOrCreateDelegatingClient(ctx context.Context, cluster client.ObjectKey) (client.Client, error) {
	c := m.getDelegatingClient(cluster)
	if c != nil {
		return c, nil
	}

	return m.newDelegatingClient(ctx, cluster)
}

// getClusterCache returns the clusterCache for cluster, or nil if it does not exist.
func (m *ClusterCacheTracker) getDelegatingClient(cluster client.ObjectKey) *client.DelegatingClient {
	m.delegatingClientsLock.RLock()
	defer m.delegatingClientsLock.RUnlock()

	return m.delegatingClients[cluster]
}

// newDelegatingClient creates a new delegating client.
func (m *ClusterCacheTracker) newDelegatingClient(ctx context.Context, cluster client.ObjectKey) (*client.DelegatingClient, error) {
	m.delegatingClientsLock.Lock()
	defer m.delegatingClientsLock.Unlock()

	// If another goroutine created the client while this one was waiting to acquire the write lock, return that
	// instead of overwriting it.
	if delegatingClient, exists := m.delegatingClients[cluster]; exists {
		return delegatingClient, nil
	}

	cache, err := m.getOrCreateClusterCache(ctx, cluster)
	if err != nil {
		return nil, err
	}
	config, err := RESTConfig(ctx, m.client, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching REST client config for remote cluster")
	}
	config.Timeout = defaultClientTimeout
	c, err := client.New(config, client.Options{Scheme: m.scheme})
	if err != nil {
		return nil, err
	}
	delegatingClient := &client.DelegatingClient{
		Reader:       cache,
		Writer:       c,
		StatusClient: c,
	}
	m.delegatingClients[cluster] = delegatingClient
	return delegatingClient, nil
}

func (m *ClusterCacheTracker) deleteDelegatingClient(cluster client.ObjectKey) {
	m.delegatingClientsLock.Lock()
	defer m.delegatingClientsLock.Unlock()

	delete(m.delegatingClients, cluster)
}

// getOrCreateClusterCache returns the clusterCache for cluster, creating a new ClusterCache if needed.
func (m *ClusterCacheTracker) getOrCreateClusterCache(ctx context.Context, cluster client.ObjectKey) (*clusterCache, error) {
	cache := m.getClusterCache(cluster)
	if cache != nil {
		return cache, nil
	}

	return m.newClusterCache(ctx, cluster)
}

// getClusterCache returns the clusterCache for cluster, or nil if it does not exist.
func (m *ClusterCacheTracker) getClusterCache(cluster client.ObjectKey) *clusterCache {
	m.clusterCachesLock.RLock()
	defer m.clusterCachesLock.RUnlock()

	return m.clusterCaches[cluster]
}

// newClusterCache creates and starts a new clusterCache for cluster.
func (m *ClusterCacheTracker) newClusterCache(ctx context.Context, cluster client.ObjectKey) (*clusterCache, error) {
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

	mapper, err := apiutil.NewDynamicRESTMapper(config)
	if err != nil {
		return nil, errors.Wrap(err, "error creating dynamic rest mapper for remote cluster")
	}

	cacheOptions := cache.Options{
		Scheme: m.scheme,
		Mapper: mapper,
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
	go m.healthCheckCluster(ctx, &healthCheckInput{
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
func (m *ClusterCacheTracker) healthCheckCluster(ctx context.Context, in *healthCheckInput) {
	// populate optional params for healthCheckInput
	in.validate()

	unhealthyCount := 0

	runHealthCheckWithThreshold := func() (bool, error) {
		cluster := &clusterv1.Cluster{}
		if err := m.client.Get(context.TODO(), in.cluster, cluster); err != nil {
			if apierrors.IsNotFound(err) {
				// If the cluster can't be found, we should delete the cache.
				return false, err
			}
			// Otherwise, requeue.
			return false, nil
		}
		if !cluster.Status.InfrastructureReady || !cluster.Status.ControlPlaneInitialized {
			// If the infrastructure or control plane aren't marked as ready, we should requeue and wait.
			return false, nil
		}

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
		m.deleteDelegatingClient(in.cluster)
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

	_, err = restClient.Get().AbsPath(path).Timeout(requestTimeout).Do(context.TODO()).Get()
	if err != nil {
		return err
	}

	return nil
}

// ClusterCacheReconciler is responsible for stopping remote cluster caches when
// the cluster for the remote cache is being deleted.
type ClusterCacheReconciler struct {
	Log     logr.Logger
	Client  client.Client
	Tracker *ClusterCacheTracker
}

func (r *ClusterCacheReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

// Reconcile reconciles Clusters and removes ClusterCaches for any Cluster that cannot be retrieved from the
// management cluster.
func (r *ClusterCacheReconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()

	log := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)
	log.V(4).Info("Reconciling")

	var cluster clusterv1.Cluster

	err := r.Client.Get(ctx, req.NamespacedName, &cluster)
	if err == nil {
		log.V(4).Info("Cluster still exists")
		return reconcile.Result{}, nil
	} else if !kerrors.IsNotFound(err) {
		log.Error(err, "Error retrieving cluster")
		return reconcile.Result{}, err
	}

	log.V(4).Info("Cluster no longer exists")

	c := r.Tracker.getClusterCache(req.NamespacedName)
	if c == nil {
		log.V(4).Info("No current cluster cache exists - nothing to do")
		return reconcile.Result{}, nil
	}

	log.V(4).Info("Stopping cluster cache")
	c.Stop()

	r.Tracker.deleteClusterCache(req.NamespacedName)
	r.Tracker.deleteDelegatingClient(req.NamespacedName)

	log.V(4).Info("Deleting watches for cluster cache")
	r.Tracker.deleteWatchesForCluster(req.NamespacedName)

	return reconcile.Result{}, nil
}
