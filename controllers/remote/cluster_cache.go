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
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	defaultClientTimeout = 10 * time.Second

	healthCheckPollInterval       = 10 * time.Second
	healthCheckRequestTimeout     = 5 * time.Second
	healthCheckUnhealthyThreshold = 10
)

// ClusterCacheTracker manages client caches for workload clusters.
type ClusterCacheTracker struct {
	log    logr.Logger
	client client.Client
	scheme *runtime.Scheme

	lock             sync.RWMutex
	clusterAccessors map[client.ObjectKey]*clusterAccessor
}

// NewClusterCacheTracker creates a new ClusterCacheTracker.
func NewClusterCacheTracker(log logr.Logger, manager ctrl.Manager) (*ClusterCacheTracker, error) {
	return &ClusterCacheTracker{
		log:              log,
		client:           manager.GetClient(),
		scheme:           manager.GetScheme(),
		clusterAccessors: make(map[client.ObjectKey]*clusterAccessor),
	}, nil
}

// GetClient returns a client for the given cluster.
func (t *ClusterCacheTracker) GetClient(ctx context.Context, cluster client.ObjectKey) (client.Client, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	accessor, err := t.getClusterAccessorLH(ctx, cluster)
	if err != nil {
		return nil, err
	}

	return accessor.client, nil
}

// clusterAccessor represents the combination of a client, cache, and watches for a remote cluster.
type clusterAccessor struct {
	cache   *stoppableCache
	client  client.Client
	watches sets.String
}

// clusterAccessorExists returns true if a clusterAccessor exists for cluster.
func (t *ClusterCacheTracker) clusterAccessorExists(cluster client.ObjectKey) bool {
	t.lock.RLock()
	defer t.lock.RUnlock()

	_, exists := t.clusterAccessors[cluster]
	return exists
}

// getClusterAccessorLH first tries to return an already-created clusterAccessor for cluster, falling back to creating a
// new clusterAccessor if needed. Note, this method requires t.lock to already be held (LH=lock held).
func (t *ClusterCacheTracker) getClusterAccessorLH(ctx context.Context, cluster client.ObjectKey) (*clusterAccessor, error) {
	a := t.clusterAccessors[cluster]
	if a != nil {
		return a, nil
	}

	a, err := t.newClusterAccessor(ctx, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "error creating client and cache for remote cluster")
	}

	t.clusterAccessors[cluster] = a

	return a, nil
}

// newClusterAccessor creates a new clusterAccessor.
func (t *ClusterCacheTracker) newClusterAccessor(ctx context.Context, cluster client.ObjectKey) (*clusterAccessor, error) {
	// Get a rest config for the remote cluster
	config, err := RESTConfig(ctx, t.client, cluster)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching REST client config for remote cluster %q", cluster.String())
	}
	config.Timeout = defaultClientTimeout

	// Create a mapper for it
	mapper, err := apiutil.NewDynamicRESTMapper(config)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating dynamic rest mapper for remote cluster %q", cluster.String())
	}

	// Create the client for the remote cluster
	c, err := client.New(config, client.Options{Scheme: t.scheme, Mapper: mapper})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating client for remote cluster %q", cluster.String())
	}

	// Create the cache for the remote cluster
	cacheOptions := cache.Options{
		Scheme: t.scheme,
		Mapper: mapper,
	}
	remoteCache, err := cache.New(config, cacheOptions)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating cache for remote cluster %q", cluster.String())
	}

	cacheCtx, cacheCtxCancel := context.WithCancel(ctx)

	// We need to be able to stop the cache's shared informers, so wrap this in a stoppableCache.
	cache := &stoppableCache{
		Cache:      remoteCache,
		cancelFunc: cacheCtxCancel,
	}

	// Start the cache!!!
	go cache.Start(cacheCtx)

	// Start cluster healthcheck!!!
	go t.healthCheckCluster(cacheCtx, &healthCheckInput{
		cluster: cluster,
		cfg:     config,
	})

	delegatingClient := client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader: cache,
		Client:      c,
	})

	return &clusterAccessor{
		cache:   cache,
		client:  delegatingClient,
		watches: sets.NewString(),
	}, nil
}

// deleteAccessor stops a clusterAccessor's cache and removes the clusterAccessor from the tracker.
func (t *ClusterCacheTracker) deleteAccessor(cluster client.ObjectKey) {
	t.lock.Lock()
	defer t.lock.Unlock()

	a, exists := t.clusterAccessors[cluster]
	if !exists {
		return
	}

	t.log.V(2).Info("Deleting clusterAccessor", "cluster", cluster.String())

	t.log.V(4).Info("Stopping cache", "cluster", cluster.String())
	a.cache.Stop()
	t.log.V(4).Info("Cache stopped", "cluster", cluster.String())

	delete(t.clusterAccessors, cluster)
}

// Watcher is a scoped-down interface from Controller that only knows how to watch.
type Watcher interface {
	// Watch watches src for changes, sending events to eventHandler if they pass predicates.
	Watch(src source.Source, eventHandler handler.EventHandler, predicates ...predicate.Predicate) error
}

// WatchInput specifies the parameters used to establish a new watch for a remote cluster.
type WatchInput struct {
	// Name represents a unique watch request for the specified Cluster.
	Name string

	// Cluster is the key for the remote cluster.
	Cluster client.ObjectKey

	// Watcher is the watcher (controller) whose Reconcile() function will be called for events.
	Watcher Watcher

	// Kind is the type of resource to watch.
	Kind client.Object

	// EventHandler contains the event handlers to invoke for resource events.
	EventHandler handler.EventHandler

	// Predicates is used to filter resource events.
	Predicates []predicate.Predicate
}

// Watch watches a remote cluster for resource events. If the watch already exists based on input.Name, this is a no-op.
func (t *ClusterCacheTracker) Watch(ctx context.Context, input WatchInput) error {
	if input.Name == "" {
		return errors.New("input.Name is required")
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	a, err := t.getClusterAccessorLH(ctx, input.Cluster)
	if err != nil {
		return err
	}

	if a.watches.Has(input.Name) {
		t.log.V(4).Info("Watch already exists", "namespace", input.Cluster.Namespace, "cluster", input.Cluster.Name, "name", input.Name)
		return nil
	}

	// Need to create the watch
	if err := input.Watcher.Watch(source.NewKindWithCache(input.Kind, a.cache), input.EventHandler, input.Predicates...); err != nil {
		return errors.Wrap(err, "error creating watch")
	}

	a.watches.Insert(input.Name)

	return nil
}

// healthCheckInput provides the input for the healthCheckCluster method
type healthCheckInput struct {
	cluster            client.ObjectKey
	cfg                *rest.Config
	interval           time.Duration
	requestTimeout     time.Duration
	unhealthyThreshold int
	path               string
}

// setDefaults sets default values if optional parameters are not set
func (h *healthCheckInput) setDefaults() {
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
func (t *ClusterCacheTracker) healthCheckCluster(ctx context.Context, in *healthCheckInput) {
	// populate optional params for healthCheckInput
	in.setDefaults()

	unhealthyCount := 0

	// This gets us a client that can make raw http(s) calls to the remote apiserver. We only need to create it once
	// and we can reuse it inside the polling loop.
	codec := runtime.NoopEncoder{Decoder: scheme.Codecs.UniversalDecoder()}
	cfg := rest.CopyConfig(in.cfg)
	cfg.NegotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{Serializer: codec})
	restClient, restClientErr := rest.UnversionedRESTClientFor(cfg)

	runHealthCheckWithThreshold := func() (bool, error) {
		if restClientErr != nil {
			return false, restClientErr
		}

		cluster := &clusterv1.Cluster{}
		if err := t.client.Get(ctx, in.cluster, cluster); err != nil {
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

		if !t.clusterAccessorExists(in.cluster) {
			// Cache for this cluster has already been cleaned up.
			// Nothing to do, so return true.
			return true, nil
		}

		// An error here means there was either an issue connecting or the API returned an error.
		// If no error occurs, reset the unhealthy counter.
		_, err := restClient.Get().AbsPath(in.path).Timeout(in.requestTimeout).DoRaw(ctx)
		if err != nil {
			unhealthyCount++
		} else {
			unhealthyCount = 0
		}

		if unhealthyCount >= in.unhealthyThreshold {
			// Cluster is now considered unhealthy.
			return false, err
		}

		return false, nil
	}

	err := wait.PollImmediateUntil(in.interval, runHealthCheckWithThreshold, ctx.Done())
	// An error returned implies the health check has failed a sufficient number of
	// times for the cluster to be considered unhealthy
	if err != nil {
		t.log.Error(err, "Error health checking cluster", "cluster", in.cluster.String())
		t.deleteAccessor(in.cluster)
	}
}
