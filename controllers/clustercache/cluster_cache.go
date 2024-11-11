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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// Options defines the options to configure a ClusterCache.
type Options struct {
	// SecretClient is the client used to access secrets of type secret.Kubeconfig, i.e. Secrets
	// with the following name format: "<cluster-name>-kubeconfig".
	// Ideally this is a client that caches only kubeconfig secrets, it is highly recommended to avoid caching all secrets.
	// An example on how to create an ideal secret caching client can be found in the core Cluster API controller main.go file.
	SecretClient client.Reader

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	// If a filter excludes a cluster from reconciliation, the accessors for this cluster
	// will never be created.
	WatchFilterValue string

	// Cache are the cache options for the caches that are created per cluster.
	Cache CacheOptions

	// Client are the client options for the clients that are created per cluster.
	Client ClientOptions
}

// CacheOptions are the cache options for the caches that are created per cluster.
type CacheOptions struct {
	// SyncPeriod is the sync period of the cache.
	SyncPeriod *time.Duration

	// ByObject restricts the cache's ListWatch to the desired fields per GVK at the specified object.
	ByObject map[client.Object]cache.ByObject

	// Indexes are the indexes added to the cache.
	Indexes []CacheOptionsIndex
}

// CacheOptionsIndex is a index that is added to the cache.
type CacheOptionsIndex struct {
	// Object is the object for which the index is created.
	Object client.Object

	// Field is the field of the index that can later be used when selecting
	// objects with a field selector.
	Field string

	// ExtractValue is a func that extracts the index value from an object.
	ExtractValue client.IndexerFunc
}

// ClientOptions are the client options for the clients that are created per cluster.
type ClientOptions struct {
	// Timeout is the timeout used for the REST config, client and cache.
	// Defaults to 10s.
	Timeout time.Duration

	// QPS is the maximum queries per second from the controller client
	// to the Kubernetes API server of workload clusters.
	// It is used for the REST config, client and cache.
	// Defaults to 20.
	QPS float32

	// Burst is the maximum number of queries that should be allowed in
	// one burst from the controller client to the Kubernetes API server of workload clusters.
	// It is used for the REST config, client and cache.
	// Default 30.
	Burst int

	// UserAgent is the user agent used for the REST config, client and cache.
	UserAgent string

	// Cache are the cache options defining how clients should interact with the underlying cache.
	Cache ClientCacheOptions
}

// ClientCacheOptions are the cache options for the clients that are created per cluster.
type ClientCacheOptions struct {
	// DisableFor is a list of objects that should never be read from the cache.
	// Get & List calls for objects configured here always result in a live lookup.
	DisableFor []client.Object
}

// ClusterCache is a component that caches clients, caches etc. for workload clusters.
type ClusterCache interface {
	// GetClient returns a cached client for the given cluster.
	// If there is no connection to the workload cluster ErrClusterNotConnected will be returned.
	GetClient(ctx context.Context, cluster client.ObjectKey) (client.Client, error)

	// GetReader returns a cached read-only client for the given cluster.
	// If there is no connection to the workload cluster ErrClusterNotConnected will be returned.
	GetReader(ctx context.Context, cluster client.ObjectKey) (client.Reader, error)

	// GetRESTConfig returns a REST config for the given cluster.
	// If there is no connection to the workload cluster ErrClusterNotConnected will be returned.
	GetRESTConfig(ctx context.Context, cluster client.ObjectKey) (*rest.Config, error)

	// GetClientCertificatePrivateKey returns a private key that is generated once for a cluster
	// and can then be used to generate client certificates. This is e.g. used in KCP to generate a client
	// cert to communicate with etcd.
	// This private key is stored and cached in the ClusterCache because it's expensive to generate a new
	// private key in every single Reconcile.
	GetClientCertificatePrivateKey(ctx context.Context, cluster client.ObjectKey) (*rsa.PrivateKey, error)

	// Watch watches a workload cluster for events.
	// Each unique watch (by input.Name) is only added once after a Connect (otherwise we return early).
	// During a disconnect existing watches (i.e. informers) are shutdown when stopping the cache.
	// After a re-connect watches will be re-added (assuming the Watch method is called again).
	// If there is no connection to the workload cluster ErrClusterNotConnected will be returned.
	Watch(ctx context.Context, cluster client.ObjectKey, watcher Watcher) error

	// GetLastProbeSuccessTimestamp returns the time when the health probe was successfully executed last.
	GetLastProbeSuccessTimestamp(ctx context.Context, cluster client.ObjectKey) time.Time

	// GetClusterSource returns a Source of Cluster events.
	// The mapFunc will be used to map from Cluster to reconcile.Request.
	// reconcile.Requests will always be enqueued on connect and disconnect.
	// Additionally the WatchForProbeFailure(time.Duration) option can be used to enqueue reconcile.Requests
	// if the health probe didn't succeed for the configured duration.
	// Note: GetClusterSource would ideally take a mapFunc that has a *clusterv1.Cluster instead of a client.Object
	// as a parameter, but then the existing mapFuncs we already use in our Reconcilers wouldn't work and we would
	// have to implement new ones.
	GetClusterSource(controllerName string, mapFunc func(ctx context.Context, cluster client.Object) []ctrl.Request, opts ...GetClusterSourceOption) source.Source
}

// ErrClusterNotConnected is returned by the ClusterCache when e.g. a Client cannot be returned
// because there is no connection to the workload cluster.
var ErrClusterNotConnected = errors.New("connection to the workload cluster is down")

// Watcher is an interface that can start a Watch.
type Watcher interface {
	Name() string
	Object() client.Object
	Watch(cache cache.Cache) error
}

// SourceWatcher is a scoped-down interface from Controller that only has the Watch func.
type SourceWatcher[request comparable] interface {
	Watch(src source.TypedSource[request]) error
}

// WatcherOptions specifies the parameters used to establish a new watch for a workload cluster.
// A source.TypedKind source (configured with Kind, TypedEventHandler and Predicates) will be added to the Watcher.
// To watch for events, the source.TypedKind will create an informer on the Cache that we have created and cached
// for the given Cluster.
type WatcherOptions = TypedWatcherOptions[client.Object, ctrl.Request]

// TypedWatcherOptions specifies the parameters used to establish a new watch for a workload cluster.
// A source.TypedKind source (configured with Kind, TypedEventHandler and Predicates) will be added to the Watcher.
// To watch for events, the source.TypedKind will create an informer on the Cache that we have created and cached
// for the given Cluster.
type TypedWatcherOptions[object client.Object, request comparable] struct {
	// Name represents a unique Watch request for the specified Cluster.
	// The name is used to track that a specific watch is only added once to a cache.
	// After a connection (and thus also the cache) has been re-created, watches have to be added
	// again by calling the Watch method again.
	Name string

	// Watcher is the watcher (controller) whose Reconcile() function will be called for events.
	Watcher SourceWatcher[request]

	// Kind is the type of resource to watch.
	Kind object

	// EventHandler contains the event handlers to invoke for resource events.
	EventHandler handler.TypedEventHandler[object, request]

	// Predicates is used to filter resource events.
	Predicates []predicate.TypedPredicate[object]
}

// NewWatcher creates a Watcher for the workload cluster.
// A source.TypedKind source (configured with Kind, TypedEventHandler and Predicates) will be added to the SourceWatcher.
// To watch for events, the source.TypedKind will create an informer on the Cache that we have created and cached
// for the given Cluster.
func NewWatcher[object client.Object, request comparable](options TypedWatcherOptions[object, request]) Watcher {
	return &watcher[object, request]{
		name:         options.Name,
		kind:         options.Kind,
		eventHandler: options.EventHandler,
		predicates:   options.Predicates,
		watcher:      options.Watcher,
	}
}

type watcher[object client.Object, request comparable] struct {
	name         string
	kind         object
	eventHandler handler.TypedEventHandler[object, request]
	predicates   []predicate.TypedPredicate[object]
	watcher      SourceWatcher[request]
}

func (tw *watcher[object, request]) Name() string          { return tw.name }
func (tw *watcher[object, request]) Object() client.Object { return tw.kind }
func (tw *watcher[object, request]) Watch(cache cache.Cache) error {
	return tw.watcher.Watch(source.TypedKind[object, request](cache, tw.kind, tw.eventHandler, tw.predicates...))
}

// GetClusterSourceOption is an option that modifies GetClusterSourceOptions for a GetClusterSource call.
type GetClusterSourceOption interface {
	// ApplyToGetClusterSourceOptions applies this option to the given GetClusterSourceOptions.
	ApplyToGetClusterSourceOptions(option *GetClusterSourceOptions)
}

// GetClusterSourceOptions allows to set options for the GetClusterSource method.
type GetClusterSourceOptions struct {
	watchForProbeFailures []time.Duration
}

// ApplyOptions applies the passed list of options to GetClusterSourceOptions,
// and then returns itself for convenient chaining.
func (o *GetClusterSourceOptions) ApplyOptions(opts []GetClusterSourceOption) *GetClusterSourceOptions {
	for _, opt := range opts {
		opt.ApplyToGetClusterSourceOptions(o)
	}
	return o
}

// WatchForProbeFailure will configure the Cluster source to enqueue reconcile.Requests if the health probe
// didn't succeed for the configured duration.
// For example if WatchForProbeFailure is set to 5m, an event will be sent if LastProbeSuccessTimestamp
// is 5m in the past (i.e. health probes didn't succeed in the last 5m).
type WatchForProbeFailure time.Duration

// ApplyToGetClusterSourceOptions applies WatchForProbeFailure to the given GetClusterSourceOptions.
func (n WatchForProbeFailure) ApplyToGetClusterSourceOptions(opts *GetClusterSourceOptions) {
	opts.watchForProbeFailures = append(opts.watchForProbeFailures, time.Duration(n))
}

// SetupWithManager sets up a ClusterCache with the given Manager and Options.
// This will add a reconciler to the Manager and returns a ClusterCache which can be used
// to retrieve e.g. Clients for a given Cluster.
func SetupWithManager(ctx context.Context, mgr manager.Manager, options Options, controllerOptions controller.Options) (ClusterCache, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("controller", "clustercache")

	if err := validateAndDefaultOptions(&options); err != nil {
		return nil, err
	}

	var controllerPodMetadata *metav1.ObjectMeta
	podNamespace := os.Getenv("POD_NAMESPACE")
	podName := os.Getenv("POD_NAME")
	podUID := os.Getenv("POD_UID")
	if podNamespace != "" && podName != "" && podUID != "" {
		log.Info("Found controller Pod metadata, the ClusterCache will try to access the cluster it is running on directly if possible")
		controllerPodMetadata = &metav1.ObjectMeta{
			Namespace: podNamespace,
			Name:      podName,
			UID:       types.UID(podUID),
		}
	} else {
		log.Info("Couldn't find controller Pod metadata, the ClusterCache will always access the cluster it is running on using the regular apiserver endpoint")
	}

	cc := &clusterCache{
		client:                mgr.GetClient(),
		clusterAccessorConfig: buildClusterAccessorConfig(mgr.GetScheme(), options, controllerPodMetadata),
		clusterAccessors:      make(map[client.ObjectKey]*clusterAccessor),
	}

	err := ctrl.NewControllerManagedBy(mgr).
		Named("clustercache").
		For(&clusterv1.Cluster{}).
		WithOptions(controllerOptions).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), log, options.WatchFilterValue)).
		Complete(cc)
	if err != nil {
		return nil, errors.Wrap(err, "failed setting up ClusterCache with a controller manager")
	}

	return cc, nil
}

type clusterCache struct {
	client client.Reader

	// clusterAccessorConfig is the config for clusterAccessors.
	clusterAccessorConfig *clusterAccessorConfig

	// clusterAccessorsLock is used to synchronize access to clusterAccessors.
	clusterAccessorsLock sync.RWMutex
	// clusterAccessors is the map of clusterAccessors by cluster.
	clusterAccessors map[client.ObjectKey]*clusterAccessor

	// clusterSourcesLock is used to synchronize access to clusterSources.
	clusterSourcesLock sync.RWMutex
	// clusterSources is used to store information about cluster sources.
	// This information is necessary so we can enqueue reconcile.Requests for reconcilers that
	// got a cluster source via GetClusterSource.
	clusterSources []clusterSource
}

// clusterSource stores the necessary information so we can enqueue reconcile.Requests for reconcilers that
// got a cluster source via GetClusterSource.
type clusterSource struct {
	// controllerName is the name of the controller that will watch this source.
	controllerName string

	// ch is the channel on which to send events.
	ch chan event.GenericEvent

	// sendEventAfterProbeFailureDurations are the durations after LastProbeSuccessTimestamp
	// after which we have to send events.
	sendEventAfterProbeFailureDurations []time.Duration

	// lastEventSentTimeByCluster are the timestamps when we last sent an event for a cluster.
	lastEventSentTimeByCluster map[client.ObjectKey]time.Time
}

func (cc *clusterCache) GetClient(ctx context.Context, cluster client.ObjectKey) (client.Client, error) {
	accessor := cc.getClusterAccessor(cluster)
	if accessor == nil {
		return nil, errors.Wrapf(ErrClusterNotConnected, "error getting client")
	}
	return accessor.GetClient(ctx)
}

func (cc *clusterCache) GetReader(ctx context.Context, cluster client.ObjectKey) (client.Reader, error) {
	accessor := cc.getClusterAccessor(cluster)
	if accessor == nil {
		return nil, errors.Wrapf(ErrClusterNotConnected, "error getting client reader")
	}
	return accessor.GetReader(ctx)
}

func (cc *clusterCache) GetRESTConfig(ctx context.Context, cluster client.ObjectKey) (*rest.Config, error) {
	accessor := cc.getClusterAccessor(cluster)
	if accessor == nil {
		return nil, errors.Wrapf(ErrClusterNotConnected, "error getting REST config")
	}
	return accessor.GetRESTConfig(ctx)
}

func (cc *clusterCache) GetClientCertificatePrivateKey(ctx context.Context, cluster client.ObjectKey) (*rsa.PrivateKey, error) {
	accessor := cc.getClusterAccessor(cluster)
	if accessor == nil {
		return nil, errors.New("error getting client certificate private key: private key was not generated yet")
	}
	return accessor.GetClientCertificatePrivateKey(ctx), nil
}

func (cc *clusterCache) Watch(ctx context.Context, cluster client.ObjectKey, watcher Watcher) error {
	accessor := cc.getClusterAccessor(cluster)
	if accessor == nil {
		return errors.Wrapf(ErrClusterNotConnected, "error creating watch %s for %T", watcher.Name(), watcher.Object())
	}
	return accessor.Watch(ctx, watcher)
}

func (cc *clusterCache) GetLastProbeSuccessTimestamp(ctx context.Context, cluster client.ObjectKey) time.Time {
	accessor := cc.getClusterAccessor(cluster)
	if accessor == nil {
		return time.Time{}
	}
	return accessor.GetLastProbeSuccessTimestamp(ctx)
}

const (
	// defaultRequeueAfter is used as a fallback if no other duration should be used.
	defaultRequeueAfter = 10 * time.Second
)

// Reconcile reconciles Clusters and manages corresponding clusterAccessors.
func (cc *clusterCache) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	clusterKey := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}

	accessor := cc.getOrCreateClusterAccessor(clusterKey)

	cluster := &clusterv1.Cluster{}
	if err := cc.client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Cluster has been deleted, disconnecting")
			accessor.Disconnect(ctx)
			cc.deleteClusterAccessor(clusterKey)
			cc.cleanupClusterSourcesForCluster(clusterKey)
			return ctrl.Result{}, nil
		}

		// Requeue, error getting the object.
		log.Error(err, fmt.Sprintf("Requeuing after %s (error getting Cluster object)", defaultRequeueAfter))
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Return if infrastructure is not ready yet to avoid trying to open a connection when it cannot succeed.
	// Requeue is not needed as there will be a new reconcile.Request when Cluster.status.infrastructureReady is set.
	if !cluster.Status.InfrastructureReady {
		log.V(6).Info("Can't connect yet, Cluster infrastructure is not ready")
		return reconcile.Result{}, nil
	}

	// Track if the current Reconcile is doing a connect or disconnect.
	didConnect := false
	didDisconnect := false

	requeueAfterDurations := []time.Duration{}

	// Try to connect, if not connected.
	connected := accessor.Connected(ctx)
	if !connected {
		lastConnectionCreationErrorTimestamp := accessor.GetLastConnectionCreationErrorTimestamp(ctx)

		// Requeue, if connection creation failed within the ConnectionCreationRetryInterval.
		if requeueAfter, requeue := shouldRequeue(time.Now(), lastConnectionCreationErrorTimestamp, accessor.config.ConnectionCreationRetryInterval); requeue {
			log.V(6).Info(fmt.Sprintf("Requeuing after %s as connection creation already failed within the last %s",
				requeueAfter.Truncate(time.Second/10), accessor.config.ConnectionCreationRetryInterval))
			requeueAfterDurations = append(requeueAfterDurations, requeueAfter)
		} else {
			if err := accessor.Connect(ctx); err != nil {
				// Requeue, if the connect failed.
				log.V(6).Info(fmt.Sprintf("Requeuing after %s (connection creation failed)",
					accessor.config.ConnectionCreationRetryInterval))
				requeueAfterDurations = append(requeueAfterDurations, accessor.config.ConnectionCreationRetryInterval)
			} else {
				// Store that connect was done successfully.
				didConnect = true
				connected = true
			}
		}
	}

	// Run the health probe, if connected.
	if connected {
		lastProbeTimestamp := accessor.GetLastProbeTimestamp(ctx)

		// Requeue, if health probe was already run within the HealthProbe.Interval.
		if requeueAfter, requeue := shouldRequeue(time.Now(), lastProbeTimestamp, accessor.config.HealthProbe.Interval); requeue {
			log.V(6).Info(fmt.Sprintf("Requeuing after %s as health probe was already run within the last %s",
				requeueAfter.Truncate(time.Second/10), accessor.config.HealthProbe.Interval))
			requeueAfterDurations = append(requeueAfterDurations, requeueAfter)
		} else {
			// Run the health probe
			tooManyConsecutiveFailures, unauthorizedErrorOccurred := accessor.HealthCheck(ctx)
			if tooManyConsecutiveFailures || unauthorizedErrorOccurred {
				// Disconnect if the health probe failed (either with unauthorized or consecutive failures >= HealthProbe.FailureThreshold).
				accessor.Disconnect(ctx)

				// Store that disconnect was done.
				didDisconnect = true
				connected = false //nolint:ineffassign // connected is *currently* not used below, let's update it anyway
			}
			switch {
			case unauthorizedErrorOccurred:
				// Requeue for connection creation immediately.
				// If we got an unauthorized error, it could be because the kubeconfig was rotated
				// and in that case we want to immediately try to create the connection again.
				log.V(6).Info("Requeuing immediately (disconnected after unauthorized error occurred)")
				requeueAfterDurations = append(requeueAfterDurations, 1*time.Millisecond)
			case tooManyConsecutiveFailures:
				// Requeue for connection creation with the regular ConnectionCreationRetryInterval.
				log.V(6).Info(fmt.Sprintf("Requeuing after %s (disconnected after consecutive failure threshold met)",
					accessor.config.ConnectionCreationRetryInterval))
				requeueAfterDurations = append(requeueAfterDurations, accessor.config.ConnectionCreationRetryInterval)
			default:
				// Requeue for next health probe.
				log.V(6).Info(fmt.Sprintf("Requeuing after %s (health probe succeeded)",
					accessor.config.HealthProbe.Interval))
				requeueAfterDurations = append(requeueAfterDurations, accessor.config.HealthProbe.Interval)
			}
		}
	}

	// Send events to cluster sources.
	lastProbeSuccessTime := accessor.GetLastProbeSuccessTimestamp(ctx)
	cc.sendEventsToClusterSources(ctx, cluster, time.Now(), lastProbeSuccessTime, didConnect, didDisconnect)

	// Requeue based on requeueAfterDurations (fallback to defaultRequeueAfter).
	return reconcile.Result{RequeueAfter: minDurationOrDefault(requeueAfterDurations, defaultRequeueAfter)}, nil
}

// getOrCreateClusterAccessor returns a clusterAccessor and creates it if it doesn't exist already.
// Note: This intentionally does not already create a client and cache. This is later done
// via clusterAccessor.Connect() by the ClusterCache reconciler.
// Note: This method should only be called in the ClusterCache Reconcile method. Otherwise it could happen
// that a new clusterAccessor is created even after ClusterCache Reconcile deleted the clusterAccessor after
// Cluster object deletion.
func (cc *clusterCache) getOrCreateClusterAccessor(cluster client.ObjectKey) *clusterAccessor {
	cc.clusterAccessorsLock.Lock()
	defer cc.clusterAccessorsLock.Unlock()

	accessor, ok := cc.clusterAccessors[cluster]
	if !ok {
		accessor = newClusterAccessor(cluster, cc.clusterAccessorConfig)
		cc.clusterAccessors[cluster] = accessor
	}

	return accessor
}

// getClusterAccessor returns a clusterAccessor if it exists, otherwise nil.
func (cc *clusterCache) getClusterAccessor(cluster client.ObjectKey) *clusterAccessor {
	cc.clusterAccessorsLock.RLock()
	defer cc.clusterAccessorsLock.RUnlock()

	return cc.clusterAccessors[cluster]
}

// deleteClusterAccessor deletes the clusterAccessor for the given cluster in the clusterAccessors map.
func (cc *clusterCache) deleteClusterAccessor(cluster client.ObjectKey) {
	cc.clusterAccessorsLock.Lock()
	defer cc.clusterAccessorsLock.Unlock()

	delete(cc.clusterAccessors, cluster)
}

// shouldRequeue calculates if we should requeue based on the lastExecutionTime and the interval.
// Note: We can implement a more sophisticated backoff mechanism later if really necessary.
func shouldRequeue(now, lastExecutionTime time.Time, interval time.Duration) (time.Duration, bool) {
	if lastExecutionTime.IsZero() {
		return time.Duration(0), false
	}

	timeSinceLastExecution := now.Sub(lastExecutionTime)
	if timeSinceLastExecution < interval {
		return interval - timeSinceLastExecution, true
	}

	return time.Duration(0), false
}

func minDurationOrDefault(durations []time.Duration, defaultDuration time.Duration) time.Duration {
	if len(durations) == 0 {
		return defaultDuration
	}

	d := durations[0]
	for i := 1; i < len(durations); i++ {
		if durations[i] < d {
			d = durations[i]
		}
	}
	return d
}

func (cc *clusterCache) cleanupClusterSourcesForCluster(cluster client.ObjectKey) {
	cc.clusterSourcesLock.Lock()
	defer cc.clusterSourcesLock.Unlock()

	for _, cs := range cc.clusterSources {
		delete(cs.lastEventSentTimeByCluster, cluster)
	}
}

func (cc *clusterCache) GetClusterSource(controllerName string, mapFunc func(ctx context.Context, cluster client.Object) []ctrl.Request, opts ...GetClusterSourceOption) source.Source {
	cc.clusterSourcesLock.Lock()
	defer cc.clusterSourcesLock.Unlock()

	getClusterSourceOptions := &GetClusterSourceOptions{}
	getClusterSourceOptions.ApplyOptions(opts)

	cs := clusterSource{
		controllerName:                      controllerName,
		ch:                                  make(chan event.GenericEvent),
		sendEventAfterProbeFailureDurations: getClusterSourceOptions.watchForProbeFailures,
		lastEventSentTimeByCluster:          map[client.ObjectKey]time.Time{},
	}
	cc.clusterSources = append(cc.clusterSources, cs)

	return source.Channel(cs.ch, handler.TypedEnqueueRequestsFromMapFunc(mapFunc))
}

func (cc *clusterCache) sendEventsToClusterSources(ctx context.Context, cluster *clusterv1.Cluster, now, lastProbeSuccessTime time.Time, didConnect, didDisconnect bool) {
	log := ctrl.LoggerFrom(ctx)

	cc.clusterSourcesLock.Lock()
	defer cc.clusterSourcesLock.Unlock()

	clusterKey := client.ObjectKeyFromObject(cluster)

	for _, cs := range cc.clusterSources {
		lastEventSentTime := cs.lastEventSentTimeByCluster[clusterKey]

		if reasons := shouldSendEvent(now, lastProbeSuccessTime, lastEventSentTime, didConnect, didDisconnect, cs.sendEventAfterProbeFailureDurations); len(reasons) > 0 {
			log.V(6).Info("Sending Cluster event", "targetController", cs.controllerName, "reasons", strings.Join(reasons, ", "))
			cs.ch <- event.GenericEvent{
				Object: cluster,
			}
			cs.lastEventSentTimeByCluster[clusterKey] = now
		}
	}
}

func shouldSendEvent(now, lastProbeSuccessTime, lastEventSentTime time.Time, didConnect, didDisconnect bool, sendEventAfterProbeFailureDurations []time.Duration) []string {
	var reasons []string
	if didConnect {
		// Send an event if connect was done.
		reasons = append(reasons, "connect")
	}
	if didDisconnect {
		// Send an event if disconnect was done.
		reasons = append(reasons, "disconnect")
	}

	if len(sendEventAfterProbeFailureDurations) > 0 {
		for _, failureDuration := range sendEventAfterProbeFailureDurations {
			shouldSendEventTime := lastProbeSuccessTime.Add(failureDuration)

			// Send an event if:
			// * lastEventSentTime is before shouldSendEventTime and
			// * shouldSendEventTime is in the past
			if lastEventSentTime.Before(shouldSendEventTime) && shouldSendEventTime.Before(now) {
				reasons = append(reasons, fmt.Sprintf("health probe didn't succeed since more than %s", failureDuration))
			}
		}
	}

	return reasons
}

// SetConnectionCreationRetryInterval can be used to overwrite the ConnectionCreationRetryInterval.
// This method should only be used for tests and is not part of the public ClusterCache interface.
func (cc *clusterCache) SetConnectionCreationRetryInterval(interval time.Duration) {
	cc.clusterAccessorConfig.ConnectionCreationRetryInterval = interval
}

func validateAndDefaultOptions(opts *Options) error {
	if opts.SecretClient == nil {
		return errors.New("options.SecretClient must be set")
	}

	if opts.Client.Timeout.Nanoseconds() == 0 {
		opts.Client.Timeout = 10 * time.Second
	}
	if opts.Client.QPS == 0 {
		opts.Client.QPS = 20
	}
	if opts.Client.Burst == 0 {
		opts.Client.Burst = 30
	}
	if opts.Client.UserAgent == "" {
		return errors.New("options.Client.UserAgent must be set")
	}

	return nil
}

func buildClusterAccessorConfig(scheme *runtime.Scheme, options Options, controllerPodMetadata *metav1.ObjectMeta) *clusterAccessorConfig {
	return &clusterAccessorConfig{
		Scheme:                          scheme,
		SecretClient:                    options.SecretClient,
		ControllerPodMetadata:           controllerPodMetadata,
		ConnectionCreationRetryInterval: 30 * time.Second,
		Cache: &clusterAccessorCacheConfig{
			InitialSyncTimeout: 5 * time.Minute,
			SyncPeriod:         options.Cache.SyncPeriod,
			ByObject:           options.Cache.ByObject,
			Indexes:            options.Cache.Indexes,
		},
		Client: &clusterAccessorClientConfig{
			Timeout:   options.Client.Timeout,
			QPS:       options.Client.QPS,
			Burst:     options.Client.Burst,
			UserAgent: options.Client.UserAgent,
			Cache: clusterAccessorClientCacheConfig{
				DisableFor: options.Client.Cache.DisableFor,
			},
		},
		HealthProbe: &clusterAccessorHealthProbeConfig{
			Timeout:          5 * time.Second,
			Interval:         10 * time.Second,
			FailureThreshold: 5,
		},
	}
}
