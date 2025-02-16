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
	"net/http"
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
//
// Deprecated: This will be removed in Cluster API v1.10, use clustercache.ErrClusterNotConnected instead.
var ErrClusterLocked = errors.New("cluster is locked already")

// ClusterCacheTracker manages client caches for workload clusters.
//
// Deprecated: This will be removed in Cluster API v1.10, use clustercache.ClusterCache instead.
type ClusterCacheTracker struct {
	log logr.Logger

	cacheByObject   map[client.Object]cache.ByObject
	cacheSyncPeriod *time.Duration

	clientUncachedObjects []client.Object
	clientQPS             float32
	clientBurst           int

	client client.Client

	// SecretCachingClient is a client which caches secrets.
	// If set it will be used to read the kubeconfig secret.
	// Otherwise the default client from the manager will be used.
	secretCachingClient client.Client

	scheme *runtime.Scheme

	// clusterAccessorsLock is used to lock the access to the clusterAccessors map.
	clusterAccessorsLock sync.RWMutex
	// clusterAccessors is the map of clusterAccessors by cluster.
	clusterAccessors map[client.ObjectKey]*clusterAccessor
	// clusterLock is a per-cluster lock used whenever we're locking for a specific cluster.
	// E.g. for actions like creating a client or adding watches.
	clusterLock *keyedMutex

	indexes []Index

	// controllerName is the name of the controller.
	// This is used to calculate the user agent string.
	controllerName string

	// controllerPodMetadata is the Pod metadata of the controller using this ClusterCacheTracker.
	// This is only set when the POD_NAMESPACE, POD_NAME and POD_UID environment variables are set.
	// This information will be used to detected if the controller is running on a workload cluster, so
	// that we can then access the apiserver directly.
	controllerPodMetadata *metav1.ObjectMeta
}

// ClusterCacheTrackerOptions defines options to configure
// a ClusterCacheTracker.
//
// Deprecated: This will be removed in Cluster API v1.10, use clustercache.ClusterCache instead.
type ClusterCacheTrackerOptions struct {
	// SecretCachingClient is a client which caches secrets.
	// If set it will be used to read the kubeconfig secret.
	// Otherwise the default client from the manager will be used.
	SecretCachingClient client.Client

	// Log is the logger used throughout the lifecycle of caches.
	// Defaults to a no-op logger if it's not set.
	Log *logr.Logger

	// CacheByObject restricts the cache's ListWatch to the desired fields per GVK at the specified object.
	CacheByObject map[client.Object]cache.ByObject

	// CacheSyncPeriod is the syncPeriod used by the remote cluster cache.
	CacheSyncPeriod *time.Duration

	// ClientUncachedObjects instructs the Client to never cache the following objects,
	// it'll instead query the API server directly.
	// Defaults to never caching ConfigMap and Secret if not set.
	ClientUncachedObjects []client.Object

	// ClientQPS is the maximum queries per second from the controller client
	// to the Kubernetes API server of workload clusters.
	// Defaults to 20.
	ClientQPS float32

	// ClientBurst is the maximum number of queries that should be allowed in
	// one burst from the controller client to the Kubernetes API server of workload clusters.
	// Default 30.
	ClientBurst int

	Indexes []Index

	// ControllerName is the name of the controller.
	// This is used to calculate the user agent string.
	// If not set, it defaults to "cluster-cache-tracker".
	ControllerName string
}

func setDefaultOptions(opts *ClusterCacheTrackerOptions) {
	if opts.Log == nil {
		l := logr.New(log.NullLogSink{})
		opts.Log = &l
	}

	l := opts.Log.WithValues("component", "remote/clustercachetracker")
	opts.Log = &l

	if len(opts.ClientUncachedObjects) == 0 {
		opts.ClientUncachedObjects = []client.Object{
			&corev1.ConfigMap{},
			&corev1.Secret{},
		}
	}

	if opts.ClientQPS == 0 {
		opts.ClientQPS = 20
	}
	if opts.ClientBurst == 0 {
		opts.ClientBurst = 30
	}
}

// NewClusterCacheTracker creates a new ClusterCacheTracker.
//
// Deprecated: This will be removed in Cluster API v1.10, use clustercache.SetupWithManager instead.
func NewClusterCacheTracker(manager ctrl.Manager, options ClusterCacheTrackerOptions) (*ClusterCacheTracker, error) {
	setDefaultOptions(&options)

	controllerName := options.ControllerName
	if controllerName == "" {
		controllerName = clusterCacheControllerName
	}

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
		controllerName:        controllerName,
		controllerPodMetadata: controllerPodMetadata,
		log:                   *options.Log,
		clientUncachedObjects: options.ClientUncachedObjects,
		cacheByObject:         options.CacheByObject,
		cacheSyncPeriod:       options.CacheSyncPeriod,
		clientQPS:             options.ClientQPS,
		clientBurst:           options.ClientBurst,
		client:                manager.GetClient(),
		secretCachingClient:   options.SecretCachingClient,
		scheme:                manager.GetScheme(),
		clusterAccessors:      make(map[client.ObjectKey]*clusterAccessor),
		clusterLock:           newKeyedMutex(),
		indexes:               options.Indexes,
	}, nil
}

// GetClient returns a cached client for the given cluster.
func (t *ClusterCacheTracker) GetClient(ctx context.Context, cluster client.ObjectKey) (client.Client, error) {
	accessor, err := t.getClusterAccessor(ctx, cluster)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get client")
	}

	return accessor.client, nil
}

// GetReader returns a cached read-only client for the given cluster.
func (t *ClusterCacheTracker) GetReader(ctx context.Context, cluster client.ObjectKey) (client.Reader, error) {
	return t.GetClient(ctx, cluster)
}

// GetRESTConfig returns a cached REST config for the given cluster.
func (t *ClusterCacheTracker) GetRESTConfig(ctc context.Context, cluster client.ObjectKey) (*rest.Config, error) {
	accessor, err := t.getClusterAccessor(ctc, cluster)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get REST config")
	}

	return accessor.config, nil
}

// GetEtcdClientCertificateKey returns a cached certificate key to be used for generating certificates for accessing etcd in the given cluster.
func (t *ClusterCacheTracker) GetEtcdClientCertificateKey(ctx context.Context, cluster client.ObjectKey) (*rsa.PrivateKey, error) {
	accessor, err := t.getClusterAccessor(ctx, cluster)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get etcd client certificate key")
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
func (t *ClusterCacheTracker) getClusterAccessor(ctx context.Context, cluster client.ObjectKey) (*clusterAccessor, error) {
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
		return nil, errors.Wrapf(ErrClusterLocked, "failed to create cluster accessor: failed to get lock for cluster (probably because another worker is trying to create the client at the moment)")
	}
	defer t.clusterLock.Unlock(cluster)

	// Until we got the cluster lock a different goroutine might have initialized the clusterAccessor
	// for this cluster successfully already. If this is the case we return it.
	if accessor, ok := t.loadAccessor(cluster); ok {
		return accessor, nil
	}

	// We are the go routine who has to initialize the clusterAccessor.
	log.V(4).Info("Creating new cluster accessor")
	accessor, err := t.newClusterAccessor(ctx, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cluster accessor")
	}

	log.V(4).Info("Storing new cluster accessor")
	t.storeAccessor(cluster, accessor)
	return accessor, nil
}

// newClusterAccessor creates a new clusterAccessor.
func (t *ClusterCacheTracker) newClusterAccessor(ctx context.Context, cluster client.ObjectKey) (*clusterAccessor, error) {
	log := ctrl.LoggerFrom(ctx)

	// Get a rest config for the remote cluster.
	// Use the secretCachingClient if set.
	secretClient := t.client
	if t.secretCachingClient != nil {
		secretClient = t.secretCachingClient
	}
	config, err := RESTConfig(ctx, t.controllerName, secretClient, cluster)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching REST client config for remote cluster %q", cluster.String())
	}
	config.QPS = t.clientQPS
	config.Burst = t.clientBurst

	// Create a http client and a mapper for the cluster.
	httpClient, mapper, restClient, err := t.createHTTPClientAndMapper(ctx, config, cluster)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating http client and mapper for remote cluster %q", cluster.String())
	}

	// Create an uncached client for the cluster.
	uncachedClient, err := t.createUncachedClient(config, cluster, httpClient, mapper)
	if err != nil {
		return nil, err
	}

	// Detect if the controller is running on the workload cluster.
	// This function uses an uncached client to ensure pods aren't cached by the long-lived client.
	runningOnCluster, err := t.runningOnWorkloadCluster(ctx, uncachedClient, cluster)
	if err != nil {
		return nil, err
	}

	// If the controller runs on the workload cluster, access the apiserver directly by using the
	// CA and Host from the in-cluster configuration.
	if runningOnCluster {
		inClusterConfig, err := ctrl.GetConfig()
		if err != nil {
			return nil, errors.Wrapf(err, "error creating client for self-hosted cluster %q", cluster.String())
		}

		// Use CA and Host from in-cluster config.
		config.CAData = nil
		config.CAFile = inClusterConfig.CAFile
		config.Host = inClusterConfig.Host

		// Update the http client and the mapper to use in-cluster config.
		httpClient, mapper, restClient, err = t.createHTTPClientAndMapper(ctx, config, cluster)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating http client and mapper (using in-cluster config) for remote cluster %q", cluster.String())
		}

		log.Info(fmt.Sprintf("Creating cluster accessor for cluster %q with in-cluster service %q", cluster.String(), config.Host))
	} else {
		log.Info(fmt.Sprintf("Creating cluster accessor for cluster %q with the regular apiserver endpoint %q", cluster.String(), config.Host))
	}

	// Create a client and a cache for the cluster.
	cachedClient, err := t.createCachedClient(ctx, config, cluster, httpClient, restClient, mapper)
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
		cache:                    cachedClient.Cache,
		config:                   config,
		client:                   cachedClient.Client,
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

// createHTTPClientAndMapper creates a http client and a dynamic rest mapper for the given cluster, based on the rest.Config.
func (t *ClusterCacheTracker) createHTTPClientAndMapper(ctx context.Context, config *rest.Config, cluster client.ObjectKey) (*http.Client, meta.RESTMapper, *rest.RESTClient, error) {
	// Create a http client for the cluster.
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error creating client for remote cluster %q: error creating http client", cluster.String())
	}

	// Create a mapper for it
	mapper, err := apiutil.NewDynamicRESTMapper(config, httpClient)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error creating client for remote cluster %q: error creating dynamic rest mapper", cluster.String())
	}

	// Create a REST client for the cluster (this is later used for health checking as well).
	codec := runtime.NoopEncoder{Decoder: scheme.Codecs.UniversalDecoder()}
	restClientConfig := rest.CopyConfig(config)
	restClientConfig.NegotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{Serializer: codec})
	restClient, err := rest.UnversionedRESTClientForConfigAndClient(restClientConfig, httpClient)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error creating client for remote cluster %q: error creating REST client", cluster.String())
	}

	// Note: This checks if the apiserver is up. We do this already here to produce a clearer error message if the cluster is unreachable.
	if _, err := restClient.Get().AbsPath("/").Timeout(healthCheckRequestTimeout).DoRaw(ctx); err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error creating client for remote cluster %q: cluster is not reachable", cluster.String())
	}

	// Verify if we can get a rest mapping from the workload cluster apiserver.
	_, err = mapper.RESTMapping(corev1.SchemeGroupVersion.WithKind("Node").GroupKind(), corev1.SchemeGroupVersion.Version)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error creating client for remote cluster %q: error getting rest mapping", cluster.String())
	}

	return httpClient, mapper, restClient, nil
}

// createUncachedClient creates an uncached client for the given cluster, based on the rest.Config.
func (t *ClusterCacheTracker) createUncachedClient(config *rest.Config, cluster client.ObjectKey, httpClient *http.Client, mapper meta.RESTMapper) (client.Client, error) {
	// Create the uncached client for the remote cluster
	uncachedClient, err := client.New(config, client.Options{
		Scheme:     t.scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating uncached client for remote cluster %q", cluster.String())
	}

	return uncachedClient, nil
}

type cachedClientOutput struct {
	Client client.Client
	Cache  *stoppableCache
}

// createCachedClient creates a cached client for the given cluster, based on a rest.Config.
func (t *ClusterCacheTracker) createCachedClient(ctx context.Context, config *rest.Config, cluster client.ObjectKey, httpClient *http.Client, restClient *rest.RESTClient, mapper meta.RESTMapper) (*cachedClientOutput, error) {
	// Create the cache for the remote cluster
	cacheOptions := cache.Options{
		HTTPClient: httpClient,
		Scheme:     t.scheme,
		Mapper:     mapper,
		ByObject:   t.cacheByObject,
		SyncPeriod: t.cacheSyncPeriod,
	}
	remoteCache, err := cache.New(config, cacheOptions)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating cached client for remote cluster %q: error creating cache", cluster.String())
	}

	// Use a context that is independent of the passed in context, so the cache doesn't get stopped
	// when the passed in context is canceled.
	cacheCtx, cacheCtxCancel := context.WithCancelCause(context.Background())

	// We need to be able to stop the cache's shared informers, so wrap this in a stoppableCache.
	cache := &stoppableCache{
		Cache:      remoteCache,
		cancelFunc: cacheCtxCancel,
	}

	for _, index := range t.indexes {
		if err := cache.IndexField(ctx, index.Object, index.Field, index.ExtractValue); err != nil {
			return nil, errors.Wrapf(err, "error creating cached client for remote cluster %q: error adding index for field %q to cache", cluster.String(), index.Field)
		}
	}

	// Create the client for the remote cluster
	cachedClient, err := client.New(config, client.Options{
		Scheme:     t.scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
		Cache: &client.CacheOptions{
			Reader:       cache,
			DisableFor:   t.clientUncachedObjects,
			Unstructured: true,
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating cached client for remote cluster %q", cluster.String())
	}

	// Start the cache!!!
	go cache.Start(cacheCtx) //nolint:errcheck

	// Wait until the cache is initially synced
	cacheSyncCtx, cacheSyncCtxCancel := context.WithTimeoutCause(ctx, initialCacheSyncTimeout, errors.New("initial sync timeout expired"))
	defer cacheSyncCtxCancel()
	if !cache.WaitForCacheSync(cacheSyncCtx) {
		cache.Stop()
		return nil, fmt.Errorf("failed waiting for cache for remote cluster %v to sync: %w", cluster, cacheSyncCtx.Err())
	}

	// Wrap the cached client with a client that sets timeouts on all Get and List calls
	// If we don't set timeouts here Get and List calls can get stuck if they lazily create a new informer
	// and the informer than doesn't sync because the workload cluster apiserver is not reachable.
	// An alternative would be to set timeouts in the contexts we pass into all Get and List calls.
	// It should be reasonable to have Get and List calls timeout within the duration configured in the restConfig.
	cachedClient = newClientWithTimeout(cachedClient, config.Timeout)

	// Start cluster healthcheck!!!
	go t.healthCheckCluster(cacheCtx, &healthCheckInput{
		cluster:    cluster,
		restClient: restClient,
	})

	return &cachedClientOutput{
		Client: cachedClient,
		Cache:  cache,
	}, nil
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
	Watch(src source.Source) error
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

	accessor, err := t.getClusterAccessor(ctx, input.Cluster)
	if err != nil {
		return errors.Wrapf(err, "failed to add %T watch on cluster %s", input.Kind, klog.KRef(input.Cluster.Namespace, input.Cluster.Name))
	}

	// We have to lock the cluster, so that the watch is not created multiple times in parallel.
	ok := t.clusterLock.TryLock(input.Cluster)
	if !ok {
		return errors.Wrapf(ErrClusterLocked, "failed to add %T watch on cluster %s: failed to get lock for cluster (probably because another worker is trying to create the client at the moment)", input.Kind, klog.KRef(input.Cluster.Namespace, input.Cluster.Name))
	}
	defer t.clusterLock.Unlock(input.Cluster)

	if accessor.watches.Has(input.Name) {
		log := ctrl.LoggerFrom(ctx)
		log.V(6).Info(fmt.Sprintf("Watch %s already exists", input.Name), "Cluster", klog.KRef(input.Cluster.Namespace, input.Cluster.Name))
		return nil
	}

	// Need to create the watch
	if err := input.Watcher.Watch(source.Kind(accessor.cache, input.Kind, input.EventHandler, input.Predicates...)); err != nil {
		return errors.Wrapf(err, "failed to add %T watch on cluster %s: failed to create watch", input.Kind, klog.KRef(input.Cluster.Namespace, input.Cluster.Name))
	}

	accessor.watches.Insert(input.Name)

	return nil
}

// healthCheckInput provides the input for the healthCheckCluster method.
type healthCheckInput struct {
	cluster            client.ObjectKey
	restClient         *rest.RESTClient
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

	runHealthCheckWithThreshold := func(ctx context.Context) (bool, error) {
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
		_, err := in.restClient.Get().AbsPath(in.path).Timeout(in.requestTimeout).DoRaw(ctx)
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

	err := wait.PollUntilContextCancel(ctx, in.interval, true, runHealthCheckWithThreshold)
	// An error returned implies the health check has failed a sufficient number of times for the cluster
	// to be considered unhealthy or the cache was stopped and thus the cache context canceled (we pass the
	// cache context into wait.PollUntilContextCancel).
	// NB. Log all errors that occurred even if this error might just be from a cancel of the cache context
	// when the cache is stopped. Logging an error in this case is not a problem and makes debugging easier.
	if err != nil {
		t.log.Error(err, "Error health checking cluster", "Cluster", klog.KRef(in.cluster.Namespace, in.cluster.Name))
	}
	// Ensure in any case that the accessor is deleted (even if it is a no-op).
	// NB. It is crucial to ensure the accessor was deleted, so it can be later recreated when the
	// cluster is reachable again
	t.deleteAccessor(ctx, in.cluster)
}

// newClientWithTimeout returns a new client which sets the specified timeout on all Get and List calls.
// If we don't set timeouts here Get and List calls can get stuck if they lazily create a new informer
// and the informer than doesn't sync because the workload cluster apiserver is not reachable.
// An alternative would be to set timeouts in the contexts we pass into all Get and List calls.
func newClientWithTimeout(client client.Client, timeout time.Duration) client.Client {
	return clientWithTimeout{
		Client:  client,
		timeout: timeout,
	}
}

type clientWithTimeout struct {
	client.Client
	timeout time.Duration
}

var _ client.Client = &clientWithTimeout{}

func (c clientWithTimeout) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	ctx, cancel := context.WithTimeoutCause(ctx, c.timeout, errors.New("call timeout expired"))
	defer cancel()
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c clientWithTimeout) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	ctx, cancel := context.WithTimeoutCause(ctx, c.timeout, errors.New("call timeout expired"))
	defer cancel()
	return c.Client.List(ctx, list, opts...)
}
