/*
Copyright 2024 The Kubernetes Authors.

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

package clustercache

import (
	"context"
	"crypto/rsa"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/util/certs"
)

// clusterAccessor is the object used to create and manage connections to a specific workload cluster.
type clusterAccessor struct {
	cluster client.ObjectKey

	// config is the config of the clusterAccessor.
	config *clusterAccessorConfig

	// lockedStateLock is used to synchronize access to lockedState.
	// It should *never* be held for an extended period of time (e.g. during connection creation) to ensure
	// regular controllers are not blocked when calling e.g. GetClient().
	// lockedStateLock should never be used directly, use the following methods instead: lock, unlock, rLock, rUnlock.
	// All usages of these functions can be found with the following regexp: "\.lock\(|\.unlock\(|\.rLock\(|\.rUnlock\("
	lockedStateLock sync.RWMutex

	// lockedState is the state of the clusterAccessor. This includes the connection (e.g. client, cache)
	// and health checking information (e.g. lastProbeSuccessTimestamp, consecutiveFailures).
	// lockedStateLock must be *always* held (via lock or rLock) before accessing this field.
	lockedState clusterAccessorLockedState

	// cacheCtx is the ctx used when starting the cache.
	// This ctx can be used by the ClusterCache to stop the cache.
	cacheCtx context.Context //nolint:containedctx
}

// clusterAccessorConfig is the config of the clusterAccessor.
type clusterAccessorConfig struct {
	// Scheme is the scheme used for the client and the cache.
	Scheme *runtime.Scheme

	// SecretClient is the client used to access secrets of type secret.Kubeconfig, i.e. Secrets
	// with the following name format: "<cluster-name>-kubeconfig".
	// Ideally this is a client that caches only kubeconfig secrets, it is highly recommended to avoid caching all secrets.
	// An example on how to create an ideal secret caching client can be found in the core Cluster API controller main.go file.
	SecretClient client.Reader

	// ControllerPodMetadata is the Pod metadata of the controller using this ClusterCache.
	// This is only set when the POD_NAMESPACE, POD_NAME and POD_UID environment variables are set.
	// This information will be used to detected if the controller is running on a workload cluster, so
	// that for the Cluster we're running on we can then access the apiserver directly instead of going
	// through the apiserver loadbalancer.
	ControllerPodMetadata *metav1.ObjectMeta

	// ConnectionCreationRetryInterval is the interval after which to retry to create a
	// connection after creating a connection failed.
	ConnectionCreationRetryInterval time.Duration

	// Cache is the config used for the cache that the clusterAccessor creates.
	Cache *clusterAccessorCacheConfig

	// Client is the config used for the client that the clusterAccessor creates.
	Client *clusterAccessorClientConfig

	// HealthProbe is the configuration for the health probe.
	HealthProbe *clusterAccessorHealthProbeConfig
}

// clusterAccessorCacheConfig is the config used for the cache that the clusterAccessor creates.
type clusterAccessorCacheConfig struct {
	// InitialSyncTimeout is the timeout used when waiting for the cache to sync after cache start.
	InitialSyncTimeout time.Duration

	// SyncPeriod is the sync period of the cache.
	SyncPeriod *time.Duration

	// ByObject restricts the cache's ListWatch to the desired fields per GVK at the specified object.
	ByObject map[client.Object]cache.ByObject

	// Indexes are the indexes added to the cache.
	Indexes []CacheOptionsIndex
}

// clusterAccessorClientConfig is the config used for the client that the clusterAccessor creates.
type clusterAccessorClientConfig struct {
	// Timeout is the timeout used for the rest.Config.
	// The rest.Config is also used to create the client and the cache.
	Timeout time.Duration

	// QPS is the qps used for the rest.Config.
	// The rest.Config is also used to create the client and the cache.
	QPS float32

	// Burst is the burst used for the rest.Config.
	// The rest.Config is also used to create the client and the cache.
	Burst int

	// UserAgent is the user agent used for the rest.Config.
	// The rest.Config is also used to create the client and the cache.
	UserAgent string

	// Cache is the cache config defining how the clients that clusterAccessor creates
	// should interact with the underlying cache.
	Cache clusterAccessorClientCacheConfig
}

// clusterAccessorClientCacheConfig is the cache config used for the client that the clusterAccessor creates.
type clusterAccessorClientCacheConfig struct {
	// DisableFor is a list of objects that should never be read from the cache.
	// Get & List calls for objects configured here always result in a live lookup.
	DisableFor []client.Object
}

// clusterAccessorHealthProbeConfig is the configuration for the health probe.
type clusterAccessorHealthProbeConfig struct {
	// Timeout is the timeout after which the health probe times out.
	Timeout time.Duration

	// Interval is the interval in which the probe should be run.
	Interval time.Duration

	// FailureThreshold is the number of consecutive failures after which
	// the health probe is considered failed.
	FailureThreshold int
}

// clusterAccessorLockedState is the state of the clusterAccessor. This includes the connection (e.g. client, cache)
// and health checking information (e.g. lastProbeSuccessTimestamp, consecutiveFailures).
// lockedStateLock must be *always* held (via lock or rLock) before accessing this field.
type clusterAccessorLockedState struct {
	// lastConnectionCreationErrorTimestamp is the timestamp when connection creation failed the last time.
	lastConnectionCreationErrorTimestamp time.Time

	// connection holds the connection state (e.g. client, cache) of the clusterAccessor.
	connection *clusterAccessorLockedConnectionState

	// clientCertificatePrivateKey is a private key that is generated once for a clusterAccessor
	// and can then be used to generate client certificates. This is e.g. used in KCP to generate a client
	// cert to communicate with etcd.
	// This private key is stored and cached in the ClusterCache because it's expensive to generate a new
	// private key in every single Reconcile.
	clientCertificatePrivateKey *rsa.PrivateKey

	// healthChecking holds the health checking state (e.g. lastProbeSuccessTimestamp, consecutiveFailures)
	// of the clusterAccessor.
	healthChecking clusterAccessorLockedHealthCheckingState
}

// RESTClient is an interface only containing the methods of *rest.RESTClient that we use.
// Using this interface instead of *rest.RESTClient makes it possible to use the fake.RESTClient in unit tests.
type RESTClient interface {
	Get() *rest.Request
}

// clusterAccessorLockedConnectionState holds the connection state (e.g. client, cache) of the clusterAccessor.
type clusterAccessorLockedConnectionState struct {
	// restConfig to communicate with the workload cluster.
	restConfig *rest.Config

	// restClient to communicate with the workload cluster.
	restClient RESTClient

	// cachedClient to communicate with the workload cluster.
	// It uses cache for Get & List calls for all Unstructured objects and
	// all typed objects except the ones for which caching has been disabled via DisableFor.
	cachedClient client.Client

	// cache is the cache used by the client.
	// It manages informers that have been created e.g. by adding indexes to the cache,
	// Get & List calls from the client or via the Watch method of the clusterAccessor.
	cache *stoppableCache

	// watches is used to track the watches that have been added through the Watch method
	// of the clusterAccessor. This is important to avoid adding duplicate watches.
	watches sets.Set[string]
}

// clusterAccessorLockedHealthCheckingState holds the health checking state (e.g. lastProbeSuccessTimestamp,
// consecutiveFailures) of the clusterAccessor.
type clusterAccessorLockedHealthCheckingState struct {
	// lastProbeTimestamp is the time when the health probe was executed last.
	lastProbeTimestamp time.Time

	// lastProbeSuccessTimestamp is the time when the health probe was successfully executed last.
	lastProbeSuccessTimestamp time.Time

	// consecutiveFailures is the number of consecutive health probe failures.
	consecutiveFailures int
}

// newClusterAccessor creates a new clusterAccessor.
func newClusterAccessor(cacheCtx context.Context, cluster client.ObjectKey, clusterAccessorConfig *clusterAccessorConfig) *clusterAccessor {
	return &clusterAccessor{
		cacheCtx: cacheCtx,
		cluster:  cluster,
		config:   clusterAccessorConfig,
	}
}

// Connected returns true if there is a connection to the workload cluster, i.e. the clusterAccessor has a
// client, cache, etc.
func (ca *clusterAccessor) Connected(ctx context.Context) bool {
	ca.rLock(ctx)
	defer ca.rUnlock(ctx)

	return ca.lockedState.connection != nil
}

// Connect creates a connection to the workload cluster, i.e. it creates a client, cache, etc.
//
// This method will only be called by the ClusterCache reconciler and not by other regular reconcilers.
// Controller-runtime guarantees that the ClusterCache reconciler will never reconcile the same Cluster concurrently.
// Thus, we can rely on this method not being called concurrently for the same Cluster / clusterAccessor.
// This means we don't have to hold the lock when we create the client, cache, etc.
// It is extremely important that we don't lock during connection creation, so that other methods like GetClient (that
// use a read lock) can return immediately even when we currently create a client. This is necessary to avoid blocking
// other reconcilers (e.g. the Cluster or Machine reconciler).
func (ca *clusterAccessor) Connect(ctx context.Context) (retErr error) {
	log := ctrl.LoggerFrom(ctx)

	if ca.Connected(ctx) {
		log.V(6).Info("Skipping connect, already connected")
		return nil
	}

	log.Info("Connecting")

	// Creating clients, cache etc. is intentionally done without a lock to avoid blocking other reconcilers.
	connection, err := ca.createConnection(ctx)

	ca.lock(ctx)
	defer ca.unlock(ctx)

	defer func() {
		if retErr != nil {
			log.Error(retErr, "Connect failed")
			connectionUp.WithLabelValues(ca.cluster.Name, ca.cluster.Namespace).Set(0)
			ca.lockedState.lastConnectionCreationErrorTimestamp = time.Now()
		} else {
			connectionUp.WithLabelValues(ca.cluster.Name, ca.cluster.Namespace).Set(1)
		}
	}()

	if err != nil {
		return err
	}

	log.Info("Connected")

	// Only generate the clientCertificatePrivateKey once as there is no need to regenerate it after disconnect/connect.
	// Note: This has to be done before setting connection, because otherwise this code wouldn't be re-entrant if the
	// private key generation fails because we check Connected above.
	if ca.lockedState.clientCertificatePrivateKey == nil {
		log.V(6).Info("Generating client certificate private key")
		clientCertificatePrivateKey, err := certs.NewPrivateKey()
		if err != nil {
			return errors.Wrapf(err, "error creating client certificate private key")
		}
		ca.lockedState.clientCertificatePrivateKey = clientCertificatePrivateKey
	}

	now := time.Now()
	ca.lockedState.healthChecking = clusterAccessorLockedHealthCheckingState{
		// A client was just created successfully, so let's set the last probe times.
		lastProbeTimestamp:        now,
		lastProbeSuccessTimestamp: now,
		consecutiveFailures:       0,
	}
	ca.lockedState.connection = &clusterAccessorLockedConnectionState{
		restConfig:   connection.RESTConfig,
		restClient:   connection.RESTClient,
		cachedClient: connection.CachedClient,
		cache:        connection.Cache,
		watches:      sets.Set[string]{},
	}

	return nil
}

// Disconnect disconnects a connection to the workload cluster.
func (ca *clusterAccessor) Disconnect(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx)
	if !ca.Connected(ctx) {
		log.V(6).Info("Skipping disconnect, already disconnected")
		return
	}

	ca.lock(ctx)

	defer func() {
		ca.unlock(ctx)
		connectionUp.WithLabelValues(ca.cluster.Name, ca.cluster.Namespace).Set(0)
	}()
	log.Info("Disconnecting")

	// Stopping the cache is non-blocking, so it's okay to do it while holding the lock.
	// Note: Stopping the cache will also trigger shutdown of all informers that have been added to the cache.
	log.V(6).Info("Stopping cache")
	ca.lockedState.connection.cache.Stop()

	log.Info("Disconnected")

	ca.lockedState.connection = nil
}

// HealthCheck will run a health probe against the cluster's apiserver (a "GET /" call).
func (ca *clusterAccessor) HealthCheck(ctx context.Context) (bool, bool) {
	log := ctrl.LoggerFrom(ctx)

	if !ca.Connected(ctx) {
		log.V(6).Info("Skipping health check, not connected")
		return false, false
	}

	ca.rLock(ctx)
	restClient := ca.lockedState.connection.restClient
	ca.rUnlock(ctx)

	log.V(6).Info("Run health probe")

	// Executing the health probe is intentionally done without a lock to avoid blocking other reconcilers.
	_, err := restClient.Get().AbsPath("/").Timeout(ca.config.HealthProbe.Timeout).DoRaw(ctx)

	ca.lock(ctx)
	defer ca.unlock(ctx)

	ca.lockedState.healthChecking.lastProbeTimestamp = time.Now()

	unauthorizedErrorOccurred := false
	switch {
	case err != nil && apierrors.IsUnauthorized(err):
		// Unauthorized means that the underlying kubeconfig is not authorizing properly anymore, which
		// usually is the result of automatic kubeconfig refreshes, meaning that we have to disconnect the
		// clusterAccessor and re-connect without waiting for further failed health probes.
		unauthorizedErrorOccurred = true
		ca.lockedState.healthChecking.consecutiveFailures++
		log.V(6).Info(fmt.Sprintf("Health probe failed (unauthorized error occurred): %v", err))
		healthCheck.WithLabelValues(ca.cluster.Name, ca.cluster.Namespace).Set(0)
		healthChecksTotal.WithLabelValues(ca.cluster.Name, ca.cluster.Namespace, "error").Inc()
	case err != nil:
		ca.lockedState.healthChecking.consecutiveFailures++
		log.V(6).Info(fmt.Sprintf("Health probe failed (%d/%d): %v",
			ca.lockedState.healthChecking.consecutiveFailures, ca.config.HealthProbe.FailureThreshold, err))
		healthCheck.WithLabelValues(ca.cluster.Name, ca.cluster.Namespace).Set(0)
		healthChecksTotal.WithLabelValues(ca.cluster.Name, ca.cluster.Namespace, "error").Inc()
	default:
		ca.lockedState.healthChecking.consecutiveFailures = 0
		ca.lockedState.healthChecking.lastProbeSuccessTimestamp = ca.lockedState.healthChecking.lastProbeTimestamp
		log.V(6).Info("Health probe succeeded")
		healthCheck.WithLabelValues(ca.cluster.Name, ca.cluster.Namespace).Set(1)
		healthChecksTotal.WithLabelValues(ca.cluster.Name, ca.cluster.Namespace, "success").Inc()
	}

	tooManyConsecutiveFailures := ca.lockedState.healthChecking.consecutiveFailures >= ca.config.HealthProbe.FailureThreshold
	return tooManyConsecutiveFailures, unauthorizedErrorOccurred
}

func (ca *clusterAccessor) GetClient(ctx context.Context) (client.Client, error) {
	ca.rLock(ctx)
	defer ca.rUnlock(ctx)

	if ca.lockedState.connection == nil {
		return nil, errors.Wrapf(ErrClusterNotConnected, "error getting client")
	}

	return ca.lockedState.connection.cachedClient, nil
}

func (ca *clusterAccessor) GetReader(ctx context.Context) (client.Reader, error) {
	ca.rLock(ctx)
	defer ca.rUnlock(ctx)

	if ca.lockedState.connection == nil {
		return nil, errors.Wrapf(ErrClusterNotConnected, "error getting client reader")
	}

	return ca.lockedState.connection.cachedClient, nil
}

func (ca *clusterAccessor) GetRESTConfig(ctx context.Context) (*rest.Config, error) {
	ca.rLock(ctx)
	defer ca.rUnlock(ctx)

	if ca.lockedState.connection == nil {
		return nil, errors.Wrapf(ErrClusterNotConnected, "error getting REST config")
	}

	return ca.lockedState.connection.restConfig, nil
}

func (ca *clusterAccessor) GetClientCertificatePrivateKey(ctx context.Context) *rsa.PrivateKey {
	ca.rLock(ctx)
	defer ca.rUnlock(ctx)

	return ca.lockedState.clientCertificatePrivateKey
}

// Watch watches a workload cluster for events.
// Each unique watch (by watcher.Name()) is only added once after a Connect (otherwise we return early).
// During a disconnect existing watches (i.e. informers) are shutdown when stopping the cache.
// After a re-connect watches will be re-added (assuming the Watch method is called again).
func (ca *clusterAccessor) Watch(ctx context.Context, watcher Watcher) error {
	if watcher.Name() == "" {
		return errors.New("watcher.Name() cannot be empty")
	}

	if !ca.Connected(ctx) {
		return errors.Wrapf(ErrClusterNotConnected, "error creating watch %s for %T", watcher.Name(), watcher.Object())
	}

	log := ctrl.LoggerFrom(ctx)

	// Calling Watch on a controller is non-blocking because it only calls Start on the Kind source.
	// Start on the Kind source starts an additional go routine to create an informer.
	// Because it is non-blocking we can use a full lock here without blocking other reconcilers too long.
	ca.lock(ctx)
	defer ca.unlock(ctx)

	// Checking connection again while holding the lock, because maybe Disconnect was called since checking above.
	if ca.lockedState.connection == nil {
		return errors.Wrapf(ErrClusterNotConnected, "error creating watch %s for %T", watcher.Name(), watcher.Object())
	}

	// Return early if the watch was already added.
	if ca.lockedState.connection.watches.Has(watcher.Name()) {
		log.V(6).Info(fmt.Sprintf("Skip creation of watch %s for %T because it already exists", watcher.Name(), watcher.Object()))
		return nil
	}

	log.Info(fmt.Sprintf("Creating watch %s for %T", watcher.Name(), watcher.Object()))
	if err := watcher.Watch(ca.lockedState.connection.cache); err != nil {
		return errors.Wrapf(err, "error creating watch %s for %T", watcher.Name(), watcher.Object())
	}

	ca.lockedState.connection.watches.Insert(watcher.Name())
	return nil
}

func (ca *clusterAccessor) GetLastProbeSuccessTimestamp(ctx context.Context) time.Time {
	ca.rLock(ctx)
	defer ca.rUnlock(ctx)

	return ca.lockedState.healthChecking.lastProbeSuccessTimestamp
}

func (ca *clusterAccessor) GetLastProbeTimestamp(ctx context.Context) time.Time {
	ca.rLock(ctx)
	defer ca.rUnlock(ctx)

	return ca.lockedState.healthChecking.lastProbeTimestamp
}

func (ca *clusterAccessor) GetLastConnectionCreationErrorTimestamp(ctx context.Context) time.Time {
	ca.rLock(ctx)
	defer ca.rUnlock(ctx)

	return ca.lockedState.lastConnectionCreationErrorTimestamp
}

func (ca *clusterAccessor) rLock(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx).WithCallDepth(1)
	log.V(10).Info("Getting read lock for ClusterAccessor")
	ca.lockedStateLock.RLock()
	log.V(10).Info("Got read lock for ClusterAccessor")
}

func (ca *clusterAccessor) rUnlock(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx).WithCallDepth(1)
	log.V(10).Info("Removing read lock for ClusterAccessor")
	ca.lockedStateLock.RUnlock()
	log.V(10).Info("Removed read lock for ClusterAccessor")
}

func (ca *clusterAccessor) lock(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx).WithCallDepth(1)
	log.V(10).Info("Getting lock for ClusterAccessor")
	ca.lockedStateLock.Lock()
	log.V(10).Info("Got lock for ClusterAccessor")
}

func (ca *clusterAccessor) unlock(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx).WithCallDepth(1)
	log.V(10).Info("Removing lock for ClusterAccessor")
	ca.lockedStateLock.Unlock()
	log.V(10).Info("Removed lock for ClusterAccessor")
}
