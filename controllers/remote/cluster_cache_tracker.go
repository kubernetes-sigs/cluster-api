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
	"crypto/rsa"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/conditions"
)

const (
	healthCheckPollInterval       = 10 * time.Second
	healthCheckRequestTimeout     = 5 * time.Second
	healthCheckUnhealthyThreshold = 10
	initialCacheSyncTimeout       = 5 * time.Minute
	clusterCacheControllerName    = "cluster-cache-tracker"
)

// ErrClusterLocked is returned in methods that require cluster-level locking
// if the cluster is already locked by another concurrent call.
var ErrClusterLocked = errors.New("cluster is locked already")

// ClusterCacheTracker manages client caches for workload clusters.
type ClusterCacheTracker struct {
	log                   logr.Logger
	clientUncachedObjects []client.Object
	client                client.Client
	scheme                *runtime.Scheme

	// clusterAccessorsLock is used to lock the access to the clusterAccessors map.
	clusterAccessorsLock sync.RWMutex
	// clusterAccessors is the map of clusterAccessors by cluster.
	clusterAccessors map[client.ObjectKey]*clusterAccessor
	// clusterLock is a per-cluster lock used whenever we're locking for a specific cluster.
	// E.g. for actions like creating a client or adding watches.
	clusterLock *keyedMutex

	indexes []Index

	// controllerPodMetadata is the Pod metadata of the controller using this ClusterCacheTracker.
	// This is only set when the POD_NAMESPACE, POD_NAME and POD_UID environment variables are set.
	// This information will be used to detected if the controller is running on a workload cluster, so
	// that we can then access the apiserver directly.
	controllerPodMetadata *metav1.ObjectMeta
}

// ClusterCacheTrackerOptions defines options to configure
// a ClusterCacheTracker.
type ClusterCacheTrackerOptions struct {
	// Log is the logger used throughout the lifecycle of caches.
	// Defaults to a no-op logger if it's not set.
	Log *logr.Logger

	// ClientUncachedObjects instructs the Client to never cache the following objects,
	// it'll instead query the API server directly.
	// Defaults to never caching ConfigMap and Secret if not set.
	ClientUncachedObjects []client.Object
	Indexes               []Index
}

func setDefaultOptions(opts *ClusterCacheTrackerOptions) {
	if opts.Log == nil {
		l := logr.New(log.NullLogSink{})
		opts.Log = &l
	}

	if len(opts.ClientUncachedObjects) == 0 {
		opts.ClientUncachedObjects = []client.Object{
			&corev1.ConfigMap{},
			&corev1.Secret{},
		}
	}
}

// NewClusterCacheTracker creates a new ClusterCacheTracker.
func NewClusterCacheTracker(manager ctrl.Manager, options ClusterCacheTrackerOptions) (*ClusterCacheTracker, error) {
	setDefaultOptions(&options)

	var controllerPodMetadata *metav1.ObjectMeta
	podNamespace := os.Getenv("POD_NAMESPACE")
	podName := os.Getenv("POD_NAME")
	podUID := os.Getenv("POD_UID")
	if podNamespace != "" && podName != "" && podUID != "" {
		options.Log.Info("Found controller pod metadata, the ClusterCacheTracker will try to access the cluster directly when possible")
		controllerPodMetadata = &metav1.ObjectMeta{
			Namespace: podNamespace,
			Name:      podName,
			UID:       types.UID(podUID),
		}
	} else {
		options.Log.Info("Couldn't find controller pod metadata, the ClusterCacheTracker will always access clusters using the regular apiserver endpoint")
	}

	return &ClusterCacheTracker{
		controllerPodMetadata: controllerPodMetadata,
		log:                   *options.Log,
		clientUncachedObjects: options.ClientUncachedObjects,
		client:                manager.GetClient(),
		scheme:                manager.GetScheme(),
		clusterAccessors:      make(map[client.ObjectKey]*clusterAccessor),
		clusterLock:           newKeyedMutex(),
		indexes:               options.Indexes,
	}, nil
}

// GetClient returns a cached client for the given cluster.
func (t *ClusterCacheTracker) GetClient(ctx context.Context, cluster client.ObjectKey) (client.Client, error) {
	accessor, err := t.getClusterAccessor(ctx, cluster, t.indexes...)
	if err != nil {
		return nil, err
	}

	return accessor.client, nil
}

// GetRESTConfig returns a cached REST config for the given cluster.
func (t *ClusterCacheTracker) GetRESTConfig(ctc context.Context, cluster client.ObjectKey) (*rest.Config, error) {
	accessor, err := t.getClusterAccessor(ctc, cluster, t.indexes...)
	if err != nil {
		return nil, err
	}

	return accessor.config, nil
}

// GetEtcdClientCertificateKey returns a cached certificate key to be used for generating certificates for accessing etcd in the given cluster.
func (t *ClusterCacheTracker) GetEtcdClientCertificateKey(ctx context.Context, cluster client.ObjectKey) (*rsa.PrivateKey, error) {
	accessor, err := t.getClusterAccessor(ctx, cluster, t.indexes...)
	if err != nil {
		return nil, err
	}

	return accessor.etcdClientCertificateKey, nil
}

// clusterAccessor represents the combination of a delegating client, cache, and watches for a remote cluster.
type clusterAccessor struct {
	cache                    *stoppableCache
	client                   client.Client
	watches                  sets.Set[string]
	config                   *rest.Config
	etcdClientCertificateKey *rsa.PrivateKey
}

// clusterAccessorExists returns true if a clusterAccessor exists for cluster.
func (t *ClusterCacheTracker) clusterAccessorExists(cluster client.ObjectKey) bool {
	t.clusterAccessorsLock.RLock()
	defer t.clusterAccessorsLock.RUnlock()

	_, exists := t.clusterAccessors[cluster]
	return exists
}

// loadAccessor loads a clusterAccessor.
func (t *ClusterCacheTracker) loadAccessor(cluster client.ObjectKey) (*clusterAccessor, bool) {
	t.clusterAccessorsLock.RLock()
	defer t.clusterAccessorsLock.RUnlock()

	accessor, ok := t.clusterAccessors[cluster]
	return accessor, ok
}

// storeAccessor stores a clusterAccessor.
func (t *ClusterCacheTracker) storeAccessor(cluster client.ObjectKey, accessor *clusterAccessor) {
	t.clusterAccessorsLock.Lock()
	defer t.clusterAccessorsLock.Unlock()

	t.clusterAccessors[cluster] = accessor
}

// getClusterAccessor returns a clusterAccessor for cluster.
// It first tries to return an already-created clusterAccessor.
// It then falls back to create a new clusterAccessor if needed.
// If there is already another go routine trying to create a clusterAccessor
// for the same cluster, an error is returned.
func (t *ClusterCacheTracker) getClusterAccessor(ctx context.Context, cluster client.ObjectKey, indexes ...Index) (*clusterAccessor, error) {
	log := ctrl.LoggerFrom(ctx, "cluster", klog.KRef(cluster.Namespace, cluster.Name))

	// If the clusterAccessor already exists, return early.
	if accessor, ok := t.loadAccessor(cluster); ok {
		return accessor, nil
	}

	// clusterAccessor doesn't exist yet, we might have to initialize one.
	// Lock on the cluster to ensure only one clusterAccessor is initialized
	// for the cluster at the same time.
	// Return an error if another go routine already tries to create a clusterAccessor.
	if ok := t.clusterLock.TryLock(cluster); !ok {
		return nil, errors.Wrapf(ErrClusterLocked, "failed to create cluster accessor: failed to get lock for cluster")
	}
	defer t.clusterLock.Unlock(cluster)

	// Until we got the cluster lock a different goroutine might have initialized the clusterAccessor
	// for this cluster successfully already. If this is the case we return it.
	if accessor, ok := t.loadAccessor(cluster); ok {
		return accessor, nil
	}

	// We are the go routine who has to initialize the clusterAccessor.
	log.V(4).Info("Creating new cluster accessor")
	accessor, err := t.newClusterAccessor(ctx, cluster, indexes...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cluster accessor")
	}

	log.V(4).Info("Storing new cluster accessor")
	t.storeAccessor(cluster, accessor)
	return accessor, nil
}

// newClusterAccessor creates a new clusterAccessor.
func (t *ClusterCacheTracker) newClusterAccessor(ctx context.Context, cluster client.ObjectKey, indexes ...Index) (*clusterAccessor, error) {
	log := ctrl.LoggerFrom(ctx)

	// Get a rest config for the remote cluster
	config, err := RESTConfig(ctx, clusterCacheControllerName, t.client, cluster)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching REST client config for remote cluster %q", cluster.String())
	}

	// Create a client and a mapper for the cluster.
	c, mapper, err := t.createClient(config, cluster)
	if err != nil {
		return nil, err
	}

	// Detect if the controller is running on the workload cluster.
	runningOnCluster, err := t.runningOnWorkloadCluster(ctx, c, cluster)
	if err != nil {
		return nil, err
	}

	// If the controller runs on the workload cluster, access the apiserver directly by using the
	// CA and Host from the in-cluster configuration.
	if runningOnCluster {
		inClusterConfig, err := ctrl.GetConfig()
		if err != nil {
			return nil, errors.Wrap(err, "error creating client for self-hosted cluster")
		}

		// Use CA and Host from in-cluster config.
		config.CAData = nil
		config.CAFile = inClusterConfig.CAFile
		config.Host = inClusterConfig.Host

		// Create a new client and overwrite the previously created client.
		c, mapper, err = t.createClient(config, cluster)
		if err != nil {
			return nil, errors.Wrap(err, "error creating client for self-hosted cluster")
		}
		log.Info(fmt.Sprintf("Creating cluster accessor for cluster %q with in-cluster service %q", cluster.String(), config.Host))
	} else {
		log.Info(fmt.Sprintf("Creating cluster accessor for cluster %q with the regular apiserver endpoint %q", cluster.String(), config.Host))
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

	for _, index := range indexes {
		if err := cache.IndexField(ctx, index.Object, index.Field, index.ExtractValue); err != nil {
			return nil, fmt.Errorf("failed to index field %s: %w", index.Field, err)
		}
	}

	// Start the cache!!!
	go cache.Start(cacheCtx) //nolint:errcheck

	// Wait until the cache is initially synced
	cacheSyncCtx, cacheSyncCtxCancel := context.WithTimeout(ctx, initialCacheSyncTimeout)
	defer cacheSyncCtxCancel()
	if !cache.WaitForCacheSync(cacheSyncCtx) {
		cache.Stop()
		return nil, fmt.Errorf("failed waiting for cache for remote cluster %v to sync: %w", cluster, cacheCtx.Err())
	}

	// Start cluster healthcheck!!!
	go t.healthCheckCluster(cacheCtx, &healthCheckInput{
		cluster: cluster,
		cfg:     config,
	})

	delegatingClient, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader:     cache,
		Client:          c,
		UncachedObjects: t.clientUncachedObjects,
	})
	if err != nil {
		return nil, err
	}

	// Generating a new private key to be used for generating temporary certificates to connect to
	// etcd on the target cluster.
	// NOTE: Generating a private key is an expensive operation, so we store it in the cluster accessor.
	etcdKey, err := certs.NewPrivateKey()
	if err != nil {
		return nil, errors.Wrapf(err, "error creating etcd client key for remote cluster %q", cluster.String())
	}

	return &clusterAccessor{
		cache:                    cache,
		config:                   config,
		client:                   delegatingClient,
		watches:                  sets.Set[string]{},
		etcdClientCertificateKey: etcdKey,
	}, nil
}

// runningOnWorkloadCluster detects if the current controller runs on the workload cluster.
func (t *ClusterCacheTracker) runningOnWorkloadCluster(ctx context.Context, c client.Client, cluster client.ObjectKey) (bool, error) {
	// Controller Pod metadata was not found, so we can't detect if we run on the workload cluster.
	if t.controllerPodMetadata == nil {
		return false, nil
	}

	// Try to get the controller pod.
	var pod corev1.Pod
	if err := c.Get(ctx, client.ObjectKey{
		Namespace: t.controllerPodMetadata.Namespace,
		Name:      t.controllerPodMetadata.Name,
	}, &pod); err != nil {
		// If the controller pod is not found, we assume we are not running on the workload cluster.
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		// If we got another error, we return the error so that this will be retried later.
		return false, errors.Wrapf(err, "error checking if we're running on workload cluster %q", cluster.String())
	}

	// If the uid is the same we found the controller pod on the workload cluster.
	return t.controllerPodMetadata.UID == pod.UID, nil
}

// createClient creates a client and a mapper based on a rest.Config.
func (t *ClusterCacheTracker) createClient(config *rest.Config, cluster client.ObjectKey) (client.Client, meta.RESTMapper, error) {
	var mapper meta.RESTMapper
	var err error

	// Create a mapper for it
	if !feature.Gates.Enabled(feature.LazyRestmapper) {
		mapper, err = apiutil.NewDynamicRESTMapper(config)
	} else {
		mapper, err = apiutil.NewDynamicRESTMapper(config, apiutil.WithExperimentalLazyMapper)
	}
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error creating dynamic rest mapper for remote cluster %q", cluster.String())
	}

	// Create the client for the remote cluster
	c, err := client.New(config, client.Options{Scheme: t.scheme, Mapper: mapper})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error creating client for remote cluster %q", cluster.String())
	}

	return c, mapper, nil
}

// deleteAccessor stops a clusterAccessor's cache and removes the clusterAccessor from the tracker.
func (t *ClusterCacheTracker) deleteAccessor(_ context.Context, cluster client.ObjectKey) {
	t.clusterAccessorsLock.Lock()
	defer t.clusterAccessorsLock.Unlock()

	a, exists := t.clusterAccessors[cluster]
	if !exists {
		return
	}

	log := t.log.WithValues("Cluster", klog.KRef(cluster.Namespace, cluster.Name))
	log.V(2).Info("Deleting clusterAccessor")
	log.V(4).Info("Stopping cache")
	a.cache.Stop()
	log.V(4).Info("Cache stopped")

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

	accessor, err := t.getClusterAccessor(ctx, input.Cluster, t.indexes...)
	if err != nil {
		return errors.Wrapf(err, "failed to add %s watch on cluster %s", input.Kind, klog.KRef(input.Cluster.Namespace, input.Cluster.Name))
	}

	// We have to lock the cluster, so that the watch is not created multiple times in parallel.
	ok := t.clusterLock.TryLock(input.Cluster)
	if !ok {
		return errors.Wrapf(ErrClusterLocked, "failed to add %T watch on cluster %s: failed to get lock for cluster", input.Kind, klog.KRef(input.Cluster.Namespace, input.Cluster.Name))
	}
	defer t.clusterLock.Unlock(input.Cluster)

	if accessor.watches.Has(input.Name) {
		log := ctrl.LoggerFrom(ctx)
		log.V(6).Info("Watch already exists", "Cluster", klog.KRef(input.Cluster.Namespace, input.Cluster.Name), "name", input.Name)
		return nil
	}

	// Need to create the watch
	if err := input.Watcher.Watch(source.NewKindWithCache(input.Kind, accessor.cache), input.EventHandler, input.Predicates...); err != nil {
		return errors.Wrapf(err, "failed to add %s watch on cluster %s: failed to create watch", input.Kind, klog.KRef(input.Cluster.Namespace, input.Cluster.Name))
	}

	accessor.watches.Insert(input.Name)

	return nil
}

// healthCheckInput provides the input for the healthCheckCluster method.
type healthCheckInput struct {
	cluster            client.ObjectKey
	cfg                *rest.Config
	interval           time.Duration
	requestTimeout     time.Duration
	unhealthyThreshold int
	path               string
}

// setDefaults sets default values if optional parameters are not set.
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

		if !cluster.Status.InfrastructureReady || !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
			// If the infrastructure or control plane aren't marked as ready, we should requeue and wait.
			return false, nil
		}

		if _, ok := t.loadAccessor(in.cluster); !ok {
			// If there is no accessor but the cluster is locked, we're probably in the middle of the cluster accessor
			// creation and we should requeue the health check until it's done.
			if ok := t.clusterLock.TryLock(in.cluster); !ok {
				t.log.V(4).Info("Waiting for cluster to be unlocked. Requeuing health check")
				return false, nil
			}
			t.clusterLock.Unlock(in.cluster)
			// Cache for this cluster has already been cleaned up.
			// Nothing to do, so return true.
			return true, nil
		}

		// An error here means there was either an issue connecting or the API returned an error.
		// If no error occurs, reset the unhealthy counter.
		_, err := restClient.Get().AbsPath(in.path).Timeout(in.requestTimeout).DoRaw(ctx)
		if err != nil {
			if apierrors.IsUnauthorized(err) {
				// Unauthorized means that the underlying kubeconfig is not authorizing properly anymore, which
				// usually is the result of automatic kubeconfig refreshes, meaning that we have to throw away the
				// clusterAccessor and rely on the creation of a new one (with a refreshed kubeconfig)
				return false, err
			}
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
	// NB. we are ignoring ErrWaitTimeout because this error happens when the channel is close, that in this case
	// happens when the cache is explicitly stopped.
	if err != nil && err != wait.ErrWaitTimeout {
		t.log.Error(err, "Error health checking cluster", "Cluster", klog.KRef(in.cluster.Namespace, in.cluster.Name))
		t.deleteAccessor(ctx, in.cluster)
	}
}
