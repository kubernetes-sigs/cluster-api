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
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"sigs.k8s.io/cluster-api/util/secret"
)

type createConnectionResult struct {
	RESTConfig                *rest.Config
	KubeconfigResourceVersion string
	Kubeconfig                []byte
	TokenHolder               *tokenHolder
	RunningOnCluster          bool
	RESTClient                *rest.RESTClient
	CachedClient              client.Client
	UncachedClient            client.Client
	Cache                     *stoppableCache
}

func (ca *clusterAccessor) createConnection(ctx context.Context) (*createConnectionResult, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(6).Info("Creating connection")

	log.V(6).Info("Creating REST config")
	restConfig, kubeconfigResourceVersion, kubeconfig, tokenHolder, err := createRESTConfig(ctx, ca.config.Client, ca.config.SecretClient, ca.cluster)
	if err != nil {
		return nil, err
	}

	log.V(6).Info("Creating HTTP client and mapper")
	httpClient, mapper, restClient, err := createHTTPClientAndMapper(ctx, ca.config.HealthProbe, restConfig)
	if err != nil {
		return nil, pkgerrors.WithMessage(err, "error creating HTTP client and mapper")
	}

	log.V(6).Info("Creating uncached client")
	uncachedClient, err := createUncachedClient(ca.config.Scheme, restConfig, httpClient, mapper)
	if err != nil {
		return nil, err
	}

	log.V(6).Info("Detect if controller is running on the cluster")
	// This function uses an uncached client to ensure pods aren't cached by the long-lived client.
	runningOnCluster, err := runningOnWorkloadCluster(ctx, ca.config.ControllerPodMetadata, uncachedClient)
	if err != nil {
		return nil, err
	}

	// If the controller runs on the workload cluster, access the apiserver directly by using the
	// CA and Host from the in-cluster configuration.
	if runningOnCluster {
		log.V(6).Info("Controller is running on the cluster, updating REST config with in-cluster config")

		inClusterConfig, err := ctrl.GetConfig()
		if err != nil {
			return nil, pkgerrors.WithMessage(err, "error getting in-cluster REST config")
		}

		// Use CA and Host from in-cluster config.
		restConfig.CAData = nil
		restConfig.CAFile = inClusterConfig.CAFile
		restConfig.Host = inClusterConfig.Host

		log.V(6).Info(fmt.Sprintf("Creating HTTP client and mapper with updated REST config with host %q", restConfig.Host))
		httpClient, mapper, restClient, err = createHTTPClientAndMapper(ctx, ca.config.HealthProbe, restConfig)
		if err != nil {
			return nil, pkgerrors.WithMessage(err, "error creating HTTP client and mapper (using in-cluster config)")
		}

		log.V(6).Info(fmt.Sprintf("Creating uncached client with updated REST config with host %q", restConfig.Host))
		uncachedClient, err = createUncachedClient(ca.config.Scheme, restConfig, httpClient, mapper)
		if err != nil {
			return nil, pkgerrors.WithMessage(err, "error creating uncached client (using in-cluster config)")
		}
	}

	log.V(6).Info("Creating cached client and cache")
	cachedClient, cache, err := createCachedClient(ctx, ca.cacheCtx, ca.config, restConfig, httpClient, mapper)
	if err != nil {
		return nil, err
	}

	return &createConnectionResult{
		RESTConfig:                restConfig,
		KubeconfigResourceVersion: kubeconfigResourceVersion,
		Kubeconfig:                kubeconfig,
		TokenHolder:               tokenHolder,
		RunningOnCluster:          runningOnCluster,
		RESTClient:                restClient,
		CachedClient:              cachedClient,
		UncachedClient:            uncachedClient,
		Cache:                     cache,
	}, nil
}

// createRESTConfig returns a REST config created based on the kubeconfig Secret.
func createRESTConfig(ctx context.Context, clientConfig *clusterAccessorClientConfig, c client.Reader, cluster client.ObjectKey) (*rest.Config, string, []byte, *tokenHolder, error) {
	kubeconfigSecret, kubeConfig, err := getKubeconfigSecret(ctx, c, cluster)
	if err != nil {
		return nil, "", nil, nil, pkgerrors.WithMessage(err, "error creating REST config: error getting kubeconfig secret")
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, "", nil, nil, pkgerrors.WithMessage(err, "error creating REST config: error parsing kubeconfig")
	}

	var holder *tokenHolder
	if usesStaticBearerToken(restConfig) {
		holder = newTokenHolder(restConfig.BearerToken)
		holder.originalWrapTransport = restConfig.WrapTransport
		holder.setConfigWrapTransport(restConfig)
	}

	// Note: Using ExecProvider is not supported in ClusterCache as we don't want our controllers to exec.
	if restConfig.ExecProvider != nil {
		return nil, "", nil, nil, pkgerrors.New("ExecProvider is not supported in ClusterCache")
	}
	// Note: Using AuthProvider is not supported as we are not registering plugins like e.g. oidc in Cluster API.
	if restConfig.AuthProvider != nil {
		return nil, "", nil, nil, pkgerrors.New("AuthProvider is not supported in ClusterCache")
	}

	restConfig.UserAgent = clientConfig.UserAgent
	restConfig.Timeout = clientConfig.Timeout
	restConfig.QPS = clientConfig.QPS
	restConfig.Burst = clientConfig.Burst

	return restConfig, kubeconfigSecret.ResourceVersion, kubeConfig, holder, nil
}

func getKubeconfigSecret(ctx context.Context, c client.Reader, cluster client.ObjectKey) (*corev1.Secret, []byte, error) {
	kubeconfigSecret, err := secret.Get(ctx, c, cluster, secret.Kubeconfig)
	if err != nil {
		return nil, nil, err
	}

	kubeConfig, ok := kubeconfigSecret.Data[secret.KubeconfigDataName]
	if !ok {
		return nil, nil, pkgerrors.Errorf("missing key %q in secret data", secret.KubeconfigDataName)
	}

	return kubeconfigSecret, kubeConfig, nil
}

// usesStaticBearerToken returns true if the rest.Config authenticates via a static bearer token,
// i.e. a token that client-go cannot refresh on its own (as opposed to e.g. a token file or exec plugin).
func usesStaticBearerToken(config *rest.Config) bool {
	return config.BearerToken != "" &&
		config.BearerTokenFile == "" &&
		config.ExecProvider == nil &&
		config.AuthProvider == nil
}

// tokenHolder holds the current bearer token and the original transport wrapper so the
// rest.Config wrapper can be rebuilt after a token rotation without stacking wrappers.
type tokenHolder struct {
	// value is the current bearer token used by all transports created for the connection.
	value atomic.Pointer[string]

	// originalWrapTransport is preserved when rebuilding the dynamic bearer-token wrapper after a token rotation.
	originalWrapTransport func(http.RoundTripper) http.RoundTripper
}

func newTokenHolder(token string) *tokenHolder {
	holder := &tokenHolder{}
	holder.store(token)
	return holder
}

func (h *tokenHolder) store(token string) {
	h.value.Store(&token)
}

func (h *tokenHolder) token() string {
	token := h.value.Load()
	if token == nil {
		return ""
	}
	return *token
}

// setConfigWrapTransport configures the rest.Config to replace its static bearer token with the current
// token from the tokenHolder while preserving request-specific Authorization headers.
func (h *tokenHolder) setConfigWrapTransport(config *rest.Config) {
	configuredToken := config.BearerToken
	config.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		if h.originalWrapTransport != nil {
			rt = h.originalWrapTransport(rt)
		}
		return &dynamicBearerTokenRoundTripper{
			holder:          h,
			configuredToken: configuredToken,
			rt:              rt,
		}
	}
}

// dynamicBearerTokenRoundTripper replaces the static bearer token configured for the transport
// with the current token from the tokenHolder, so a rotated token is picked up without re-creating
// transports. Request-specific Authorization headers are preserved.
type dynamicBearerTokenRoundTripper struct {
	holder *tokenHolder

	// configuredToken is the static token client-go adds to requests for this transport.
	// Only this token is replaced so request-specific Authorization headers are preserved.
	configuredToken string

	rt http.RoundTripper
}

var _ utilnet.RoundTripperWrapper = &dynamicBearerTokenRoundTripper{}

func (rt *dynamicBearerTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	authorizationHeader := req.Header.Get("Authorization")
	if authorizationHeader != "" && authorizationHeader != "Bearer "+rt.configuredToken {
		return rt.rt.RoundTrip(req)
	}

	token := rt.holder.token()
	if token != "" {
		req = utilnet.CloneRequest(req)
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return rt.rt.RoundTrip(req)
}

func (rt *dynamicBearerTokenRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return rt.rt
}

// runningOnWorkloadCluster detects if the current controller runs on the workload cluster.
func runningOnWorkloadCluster(ctx context.Context, controllerPodMetadata *metav1.ObjectMeta, c client.Client) (bool, error) {
	// Controller Pod metadata was not found, so we can't detect if we run on the workload cluster.
	if controllerPodMetadata == nil {
		return false, nil
	}

	// Try to get the controller pod.
	var pod corev1.Pod
	if err := c.Get(ctx, client.ObjectKey{
		Namespace: controllerPodMetadata.Namespace,
		Name:      controllerPodMetadata.Name,
	}, &pod); err != nil {
		// If the controller Pod is not found, we assume we are not running on the workload cluster.
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		// If we got another error, we return the error so that this will be retried later.
		return false, pkgerrors.WithMessage(err, "error checking if we're running on workload cluster")
	}

	// If the uid is the same we found the controller pod on the workload cluster.
	return controllerPodMetadata.UID == pod.UID, nil
}

// createHTTPClientAndMapper creates a http client and a dynamic REST mapper for the given cluster, based on the rest.Config.
func createHTTPClientAndMapper(ctx context.Context, healthProbeConfig *clusterAccessorHealthProbeConfig, config *rest.Config) (*http.Client, meta.RESTMapper, *rest.RESTClient, error) {
	// Create a http client for the cluster.
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, nil, nil, pkgerrors.WithMessage(err, "error creating HTTP client")
	}

	// Create a dynamic REST mapper for the cluster.
	mapper, err := apiutil.NewDynamicRESTMapper(config, httpClient)
	if err != nil {
		return nil, nil, nil, pkgerrors.WithMessage(err, "error creating dynamic REST mapper")
	}

	// Create a REST client for the cluster (this is later used for health checking as well).
	codec := runtime.NoopEncoder{Decoder: scheme.Codecs.UniversalDecoder()}
	restClientConfig := rest.CopyConfig(config)
	restClientConfig.NegotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{Serializer: codec})
	restClient, err := rest.UnversionedRESTClientForConfigAndClient(restClientConfig, httpClient)
	if err != nil {
		return nil, nil, nil, pkgerrors.WithMessage(err, "error creating REST client")
	}

	// Note: This checks if the apiserver is up. We do this already here to produce a clearer error message if the cluster is unreachable.
	if _, err := restClient.Get().AbsPath("/").Timeout(healthProbeConfig.Timeout).DoRaw(ctx); err != nil {
		return nil, nil, nil, pkgerrors.WithMessage(err, "cluster is not reachable")
	}

	// Verify if we can get a REST mapping from the workload cluster apiserver.
	_, err = mapper.RESTMapping(corev1.SchemeGroupVersion.WithKind("Node").GroupKind(), corev1.SchemeGroupVersion.Version)
	if err != nil {
		return nil, nil, nil, pkgerrors.WithMessage(err, "error getting REST mapping")
	}

	return httpClient, mapper, restClient, nil
}

// createUncachedClient creates an uncached client for the given cluster, based on the rest.Config.
func createUncachedClient(scheme *runtime.Scheme, config *rest.Config, httpClient *http.Client, mapper meta.RESTMapper) (client.Client, error) {
	// Create the uncached client for the cluster.
	uncachedClient, err := client.New(config, client.Options{
		Scheme:     scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, pkgerrors.WithMessage(err, "error creating uncached client")
	}

	return newClientWithTimeout(uncachedClient, config.Timeout), nil
}

// createCachedClient creates a cached client for the given cluster, based on the rest.Config.
func createCachedClient(ctx, cacheCtx context.Context, clusterAccessorConfig *clusterAccessorConfig, config *rest.Config, httpClient *http.Client, mapper meta.RESTMapper) (client.Client, *stoppableCache, error) {
	// This config will only be used for List and Watch calls of informers
	// because we don't want these requests to time out after the regular timeout
	// of Options.Client.Timeout (default 10s).
	// Lists of informers have no timeouts set.
	// Watches of informers are timing out per default after [5m, 2*5m].
	// https://github.com/kubernetes/client-go/blob/v0.32.0/tools/cache/reflector.go#L53-L55
	// We are setting 11m to set a timeout for List calls without influencing Watch calls.
	configWith11mTimeout := rest.CopyConfig(config)
	configWith11mTimeout.Timeout = 11 * time.Minute
	httpClientWith11mTimeout, err := rest.HTTPClientFor(configWith11mTimeout)
	if err != nil {
		return nil, nil, pkgerrors.WithMessage(err, "error creating cache: error creating HTTP client")
	}

	// Create the cache for the cluster.
	cacheOptions := cache.Options{
		HTTPClient:       httpClientWith11mTimeout,
		Scheme:           clusterAccessorConfig.Scheme,
		Mapper:           mapper,
		SyncPeriod:       clusterAccessorConfig.Cache.SyncPeriod,
		DefaultTransform: clusterAccessorConfig.Cache.DefaultTransform,
		ByObject:         clusterAccessorConfig.Cache.ByObject,
	}
	remoteCache, err := cache.New(configWith11mTimeout, cacheOptions)
	if err != nil {
		return nil, nil, pkgerrors.WithMessage(err, "error creating cache")
	}

	// Use a context that is independent of the passed in context, so the cache doesn't get stopped
	// when the passed in context is canceled.
	cacheCtx, cacheCtxCancel := context.WithCancelCause(cacheCtx)

	// We need to be able to stop the cache's shared informers, so wrap this in a stoppableCache.
	cache := &stoppableCache{
		Cache:      remoteCache,
		cancelFunc: cacheCtxCancel,
	}

	for _, index := range clusterAccessorConfig.Cache.Indexes {
		if err := cache.IndexField(ctx, index.Object, index.Field, index.ExtractValue); err != nil {
			return nil, nil, pkgerrors.WithMessagef(err, "error adding index for field %q to cache", index.Field)
		}
	}

	// Create the client for the cluster.
	cachedClient, err := client.New(config, client.Options{
		Scheme:     clusterAccessorConfig.Scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
		Cache: &client.CacheOptions{
			Reader:       cache,
			DisableFor:   clusterAccessorConfig.Client.Cache.DisableFor,
			Unstructured: true,
		},
	})
	if err != nil {
		return nil, nil, pkgerrors.WithMessage(err, "error creating cached client")
	}

	// Start the cache!
	go cache.Start(cacheCtx) //nolint:errcheck

	// Wait until the cache is initially synced.
	cacheSyncCtx, cacheSyncCtxCancel := context.WithTimeoutCause(ctx, clusterAccessorConfig.Cache.InitialSyncTimeout, pkgerrors.New("initial sync timeout expired"))
	defer cacheSyncCtxCancel()
	if !cache.WaitForCacheSync(cacheSyncCtx) {
		cache.Stop()
		return nil, nil, fmt.Errorf("error when waiting for cache to sync: %w", cacheSyncCtx.Err())
	}

	// Wrap the cached client with a client that sets timeouts on all Get and List calls
	// If we don't set timeouts here Get and List calls can get stuck if they lazily create a new informer
	// and the informer than doesn't sync because the workload cluster apiserver is not reachable.
	// An alternative would be to set timeouts in the contexts we pass into all Get and List calls.
	// It should be reasonable to have Get and List calls timeout within the duration configured in the restConfig.
	cachedClient = newClientWithTimeout(cachedClient, config.Timeout)

	return cachedClient, cache, nil
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
	ctx, cancel := context.WithTimeoutCause(ctx, c.timeout, pkgerrors.New("call timeout expired"))
	defer cancel()
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c clientWithTimeout) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	ctx, cancel := context.WithTimeoutCause(ctx, c.timeout, pkgerrors.New("call timeout expired"))
	defer cancel()
	return c.Client.List(ctx, list, opts...)
}

// stoppableCache embeds cache.Cache and combines it with a stop channel.
type stoppableCache struct {
	cache.Cache

	lock       sync.Mutex
	stopped    bool
	cancelFunc context.CancelCauseFunc
}

// Stop cancels the cache.Cache's context, unless it has already been stopped.
func (cc *stoppableCache) Stop() {
	cc.lock.Lock()
	defer cc.lock.Unlock()

	if cc.stopped {
		return
	}

	cc.stopped = true
	cc.cancelFunc(pkgerrors.New("cache stopped"))
}
