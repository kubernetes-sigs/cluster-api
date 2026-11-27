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
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/rest/fake"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestConnect(t *testing.T) {
	g := NewWithT(t)

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, testCluster)).To(Succeed()) }()

	config := buildClusterAccessorConfig(env.GetScheme(), Options{
		SecretCachingClient: env.GetClient(),
		Client: ClientOptions{
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Timeout:   10 * time.Second,
		},
		Cache: CacheOptions{
			Indexes: []CacheOptionsIndex{NodeProviderIDIndex},
		},
	}, nil)
	accessor := newClusterAccessor(context.Background(), clusterKey, config)

	// Before connect, getting the uncached client should fail with ErrClusterNotConnected
	_, err := accessor.GetUncachedClient(ctx)
	g.Expect(err).To(HaveOccurred())
	g.Expect(errors.Is(err, ErrClusterNotConnected)).To(BeTrue())

	// Connect when kubeconfig Secret doesn't exist (should fail)
	err = accessor.Connect(ctx)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(Equal("error getting kubeconfig secret: Secret \"test-cluster-kubeconfig\" not found"))
	g.Expect(accessor.Connected(ctx)).To(BeFalse())
	g.Expect(accessor.lockedState).To(Equal(clusterAccessorLockedState{}))

	// Create invalid kubeconfig Secret
	kubeconfigBytes := kubeconfig.FromEnvTestConfig(env.Config, testCluster)
	cmdConfig, err := clientcmd.Load(kubeconfigBytes)
	g.Expect(err).ToNot(HaveOccurred())
	cmdConfig.Clusters[testCluster.Name].Server += "invalid-context-path" // breaks the server URL.
	kubeconfigBytes, err = clientcmd.Write(*cmdConfig)
	g.Expect(err).ToNot(HaveOccurred())
	kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfigBytes)
	g.Expect(env.CreateAndWait(ctx, kubeconfigSecret)).To(Succeed())

	// Connect with invalid kubeconfig Secret
	err = accessor.Connect(ctx)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(Equal("error creating HTTP client and mapper: cluster is not reachable: the server could not find the requested resource"))
	g.Expect(accessor.Connected(ctx)).To(BeFalse())
	g.Expect(accessor.lockedState.lastConnectionCreationErrorTime.IsZero()).To(BeFalse())
	g.Expect(accessor.lockedState).To(Equal(clusterAccessorLockedState{
		lastConnectionCreationErrorTime: accessor.lockedState.lastConnectionCreationErrorTime,
		healthChecking: clusterAccessorLockedHealthCheckingState{
			lastProbeTime:       accessor.lockedState.healthChecking.lastProbeTime,
			consecutiveFailures: 1,
		},
	}))

	// Cleanup invalid kubeconfig Secret
	g.Expect(env.CleanupAndWait(ctx, kubeconfigSecret)).To(Succeed())

	// Create kubeconfig Secret
	kubeconfigSecret = kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
	g.Expect(env.CreateAndWait(ctx, kubeconfigSecret)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, kubeconfigSecret)).To(Succeed()) }()

	// Connect
	g.Expect(accessor.Connect(ctx)).To(Succeed())
	g.Expect(accessor.Connected(ctx)).To(BeTrue())

	g.Expect(accessor.lockedState.connection.restConfig).ToNot(BeNil())
	g.Expect(accessor.lockedState.connection.restClient).ToNot(BeNil())
	g.Expect(accessor.lockedState.connection.cachedClient).ToNot(BeNil())
	g.Expect(accessor.lockedState.connection.cache).ToNot(BeNil())
	g.Expect(accessor.lockedState.connection.watches).ToNot(BeNil())

	// Check if cache was started / synced
	cacheSyncCtx, cacheSyncCtxCancel := context.WithTimeout(ctx, 5*time.Second)
	defer cacheSyncCtxCancel()
	g.Expect(accessor.lockedState.connection.cache.WaitForCacheSync(cacheSyncCtx)).To(BeTrue())

	g.Expect(accessor.lockedState.healthChecking.lastProbeTime.IsZero()).To(BeFalse())
	g.Expect(accessor.lockedState.healthChecking.lastProbeSuccessTime.IsZero()).To(BeFalse())
	g.Expect(accessor.lockedState.healthChecking.consecutiveFailures).To(Equal(0))

	// After connect, getting the uncached client should succeed
	r, err := accessor.GetUncachedClient(ctx)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(r).ToNot(BeNil())

	// List Nodes via the uncached client
	nodeListUncached := &corev1.NodeList{}
	g.Expect(r.List(ctx, nodeListUncached)).To(Succeed())
	g.Expect(nodeListUncached.Items).To(BeEmpty())

	// Get client and test Get & List
	c, err := accessor.GetClient(ctx)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(c.Get(ctx, client.ObjectKey{Name: metav1.NamespaceDefault}, &corev1.Namespace{})).To(Succeed())
	nodeList := &corev1.NodeList{}
	g.Expect(c.List(ctx, nodeList)).To(Succeed())
	g.Expect(nodeList.Items).To(BeEmpty())

	// Connect again (no-op)
	g.Expect(accessor.Connect(ctx)).To(Succeed())

	// Disconnect
	accessor.Disconnect(ctx)
	g.Expect(accessor.Connected(ctx)).To(BeFalse())

	// After disconnect, getting the uncached client should fail with ErrClusterNotConnected
	_, err = accessor.GetUncachedClient(ctx)
	g.Expect(err).To(HaveOccurred())
	g.Expect(errors.Is(err, ErrClusterNotConnected)).To(BeTrue())
}

func TestDisconnect(t *testing.T) {
	g := NewWithT(t)

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, testCluster)).To(Succeed()) }()

	// Create kubeconfig Secret
	kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
	g.Expect(env.CreateAndWait(ctx, kubeconfigSecret)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, kubeconfigSecret)).To(Succeed()) }()

	config := buildClusterAccessorConfig(env.GetScheme(), Options{
		SecretCachingClient: env.GetClient(),
		Client: ClientOptions{
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Timeout:   10 * time.Second,
		},
	}, nil)
	accessor := newClusterAccessor(context.Background(), clusterKey, config)

	// Connect (so we can disconnect afterward)
	g.Expect(accessor.Connect(ctx)).To(Succeed())
	g.Expect(accessor.Connected(ctx)).To(BeTrue())

	cache := accessor.lockedState.connection.cache
	g.Expect(cache.stopped).To(BeFalse())

	// Disconnect
	accessor.Disconnect(ctx)
	g.Expect(accessor.Connected(ctx)).To(BeFalse())
	g.Expect(accessor.lockedState.connection).To(BeNil())
	g.Expect(cache.stopped).To(BeTrue())

	// Disconnect again (no-op)
	accessor.Disconnect(ctx)
	g.Expect(accessor.Connected(ctx)).To(BeFalse())

	g.Expect(accessor.lockedState.healthChecking.lastProbeTime.IsZero()).To(BeFalse())
	g.Expect(accessor.lockedState.healthChecking.lastProbeSuccessTime.IsZero()).To(BeFalse())
}

func TestHealthCheck(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)

	tests := []struct {
		name                           string
		connected                      bool
		restClientHTTPResponse         *http.Response
		initialConsecutiveFailures     int
		wantTooManyConsecutiveFailures bool
		wantUnauthorizedErrorOccurred  bool
		wantConsecutiveFailures        int
	}{
		{
			name:      "Health probe failed with unauthorized error",
			connected: true,
			restClientHTTPResponse: &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     header(),
				Body:       objBody(&apierrors.NewUnauthorized("authorization failed").ErrStatus),
			},
			wantTooManyConsecutiveFailures: false,
			wantUnauthorizedErrorOccurred:  true,
			wantConsecutiveFailures:        1,
		},
		{
			name:      "Health probe failed with other error",
			connected: true,
			restClientHTTPResponse: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     header(),
				Body:       objBody(&apierrors.NewBadRequest("bad request").ErrStatus),
			},
			wantTooManyConsecutiveFailures: false,
			wantUnauthorizedErrorOccurred:  false,
			wantConsecutiveFailures:        1,
		},
		{
			name:      "Health probe failed with other error (failure threshold met)",
			connected: true,
			restClientHTTPResponse: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     header(),
				Body:       objBody(&apierrors.NewBadRequest("bad request").ErrStatus),
			},
			initialConsecutiveFailures:     4, // failure threshold is 5
			wantTooManyConsecutiveFailures: true,
			wantUnauthorizedErrorOccurred:  false,
			wantConsecutiveFailures:        5,
		},
		{
			name:      "Health probe succeeded",
			connected: true,
			restClientHTTPResponse: &http.Response{
				StatusCode: http.StatusOK,
			},
			wantTooManyConsecutiveFailures: false,
			wantUnauthorizedErrorOccurred:  false,
			wantConsecutiveFailures:        0,
		},
		{
			name:      "Health probe skipped (not connected)",
			connected: false,
			restClientHTTPResponse: &http.Response{
				StatusCode: http.StatusOK,
			},
			wantTooManyConsecutiveFailures: false,
			wantUnauthorizedErrorOccurred:  false,
			wantConsecutiveFailures:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			accessor := newClusterAccessor(context.Background(), clusterKey, &clusterAccessorConfig{
				HealthProbe: &clusterAccessorHealthProbeConfig{
					Timeout:          5 * time.Second,
					FailureThreshold: 5,
				},
			})
			accessor.lockedState.connection = &clusterAccessorLockedConnectionState{
				restClient: &fake.RESTClient{
					NegotiatedSerializer: scheme.Codecs,
					Resp:                 tt.restClientHTTPResponse,
					Err:                  nil,
				},
			}
			accessor.lockedState.healthChecking = clusterAccessorLockedHealthCheckingState{
				consecutiveFailures: tt.initialConsecutiveFailures,
			}

			if !tt.connected {
				accessor.lockedState.connection = nil
			}

			gotTooManyConsecutiveFailures, gotUnauthorizedErrorOccurred := accessor.HealthCheck(ctx)
			g.Expect(gotTooManyConsecutiveFailures).To(Equal(tt.wantTooManyConsecutiveFailures))
			g.Expect(gotUnauthorizedErrorOccurred).To(Equal(tt.wantUnauthorizedErrorOccurred))
		})
	}
}

func TestSyncKubeconfig(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)

	// The apiservers only accept old-token and new-token and serve the discovery information required to create a connection.
	// Note: TLS is required, as client-go ignores the auth info of a kubeconfig for non-TLS servers.
	newAPIServer := func() *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Header.Get("Authorization") {
			case "Bearer old-token", "Bearer new-token":
			default:
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/api":
				_, _ = w.Write([]byte(`{"kind":"APIVersions","versions":["v1"]}`))
			case "/api/v1":
				_, _ = w.Write([]byte(`{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"nodes","kind":"Node","namespaced":false,"verbs":["get","list","watch"]}]}`))
			case "/apis":
				_, _ = w.Write([]byte(`{"kind":"APIGroupList","groups":[]}`))
			default:
				w.WriteHeader(http.StatusOK)
			}
		}))
	}
	oldServer := newAPIServer()
	defer oldServer.Close()
	newServer := newAPIServer()
	defer newServer.Close()
	unreachableServer := newAPIServer()
	unreachableServer.Close()
	// All httptest servers use the same certificate, which is valid for 127.0.0.1 and example.com.
	caData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: oldServer.Certificate().Raw})

	oldServerKubeconfig := newTokenKubeconfig("test-cluster", oldServer.URL, "old-token", caData)
	oldServerRotatedTokenKubeconfig := newTokenKubeconfig("test-cluster", oldServer.URL, "new-token", caData)
	oldServerRejectedTokenKubeconfig := newTokenKubeconfig("test-cluster", oldServer.URL, "rejected-token", caData)
	newServerKubeconfig := newTokenKubeconfig("test-cluster", newServer.URL, "old-token", caData)
	newServerRotatedTokenKubeconfig := newTokenKubeconfig("test-cluster", newServer.URL, "new-token", caData)
	unreachableServerKubeconfig := newTokenKubeconfig("test-cluster", unreachableServer.URL, "old-token", caData)
	oldServerKubeconfigWithInactiveToken := withAuthInfoToken(oldServerKubeconfig, "inactive-user", "old-inactive-token")
	oldServerKubeconfigWithNewInactiveToken := withAuthInfoToken(oldServerKubeconfig, "inactive-user", "new-inactive-token")
	oldServerWithoutCurrentContextKubeconfig := withoutCurrentContext(oldServerKubeconfig)

	tests := []struct {
		name                   string
		initialResourceVersion string
		initialKubeconfig      []byte
		initialTokenHolder     *tokenHolder
		secret                 *corev1.Secret
		secretReaderError      error
		wantErr                bool
		wantReconnected        bool
		wantResourceVersion    string
		wantKubeconfig         []byte
		wantToken              string
	}{
		{
			name:                   "resourceVersion unchanged returns no-op",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "1", oldServerRotatedTokenKubeconfig),
			wantResourceVersion:    "1",
			wantKubeconfig:         oldServerKubeconfig,
			wantToken:              "old-token",
		},
		{
			name:                   "same kubeconfig bytes with new resourceVersion only updates resourceVersion",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "2", oldServerKubeconfig),
			wantResourceVersion:    "2",
			wantKubeconfig:         oldServerKubeconfig,
			wantToken:              "old-token",
		},
		{
			name:                   "same kubeconfig bytes with new resourceVersion without token holder only updates resourceVersion",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			secret:                 newKubeconfigSecret(testCluster, "2", oldServerKubeconfig),
			wantResourceVersion:    "2",
			wantKubeconfig:         oldServerKubeconfig,
		},
		{
			name:                   "token-only change refreshes token in place",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "2", oldServerRotatedTokenKubeconfig),
			wantResourceVersion:    "2",
			wantKubeconfig:         oldServerRotatedTokenKubeconfig,
			wantToken:              "new-token",
		},
		{
			name:                   "token-only change with rejected token keeps current token",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "2", oldServerRejectedTokenKubeconfig),
			wantErr:                true,
			wantResourceVersion:    "1",
			wantKubeconfig:         oldServerKubeconfig,
			wantToken:              "old-token",
		},
		{
			name:                   "token-only change without token holder re-creates connection",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			secret:                 newKubeconfigSecret(testCluster, "2", oldServerRotatedTokenKubeconfig),
			wantReconnected:        true,
			wantResourceVersion:    "2",
			wantKubeconfig:         oldServerRotatedTokenKubeconfig,
			wantToken:              "new-token",
		},
		{
			name:                   "server change re-creates connection",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "2", newServerKubeconfig),
			wantReconnected:        true,
			wantResourceVersion:    "2",
			wantKubeconfig:         newServerKubeconfig,
			wantToken:              "old-token",
		},
		{
			name:                   "server change to unreachable server keeps connection",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "2", unreachableServerKubeconfig),
			wantErr:                true,
			wantResourceVersion:    "1",
			wantKubeconfig:         oldServerKubeconfig,
			wantToken:              "old-token",
		},
		{
			name:                   "server and token change re-creates connection",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "2", newServerRotatedTokenKubeconfig),
			wantReconnected:        true,
			wantResourceVersion:    "2",
			wantKubeconfig:         newServerRotatedTokenKubeconfig,
			wantToken:              "new-token",
		},
		{
			name:                   "inactive AuthInfo token change re-creates connection",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfigWithInactiveToken,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "2", oldServerKubeconfigWithNewInactiveToken),
			wantReconnected:        true,
			wantResourceVersion:    "2",
			wantKubeconfig:         oldServerKubeconfigWithNewInactiveToken,
			wantToken:              "old-token",
		},
		{
			name:                   "secret missing keeps connection",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secretReaderError:      apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "test-cluster-kubeconfig"),
			wantErr:                true,
			wantResourceVersion:    "1",
			wantKubeconfig:         oldServerKubeconfig,
			wantToken:              "old-token",
		},
		{
			name:                   "unparsable new kubeconfig keeps connection",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "2", []byte("not a kubeconfig")),
			wantErr:                true,
			wantResourceVersion:    "1",
			wantKubeconfig:         oldServerKubeconfig,
			wantToken:              "old-token",
		},
		{
			name:                   "new kubeconfig without current context keeps connection",
			initialResourceVersion: "1",
			initialKubeconfig:      oldServerKubeconfig,
			initialTokenHolder:     newTokenHolder("old-token"),
			secret:                 newKubeconfigSecret(testCluster, "2", oldServerWithoutCurrentContextKubeconfig),
			wantErr:                true,
			wantResourceVersion:    "1",
			wantKubeconfig:         oldServerKubeconfig,
			wantToken:              "old-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			accessorConfig := buildClusterAccessorConfig(env.GetScheme(), Options{
				SecretCachingClient: &testSecretReader{
					secret: tt.secret,
					err:    tt.secretReaderError,
				},
				Client: ClientOptions{
					UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
					Timeout:   5 * time.Second,
				},
			}, nil)
			accessor := newClusterAccessor(context.Background(), clusterKey, accessorConfig)
			oldCache := &stoppableCache{cancelFunc: func(error) {}}
			accessor.lockedState.connection = &clusterAccessorLockedConnectionState{
				restConfig:                &rest.Config{Host: oldServer.URL},
				kubeconfigResourceVersion: tt.initialResourceVersion,
				kubeconfig:                mustLoadKubeconfig(tt.initialKubeconfig),
				tokenHolder:               tt.initialTokenHolder,
				cache:                     oldCache,
			}
			defer accessor.Disconnect(ctx)

			reconnected, err := accessor.SyncKubeconfig(ctx)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(reconnected).To(Equal(tt.wantReconnected))

			g.Expect(accessor.Connected(ctx)).To(BeTrue())
			connection := accessor.lockedState.connection
			g.Expect(connection.kubeconfigResourceVersion).To(Equal(tt.wantResourceVersion))
			g.Expect(connection.kubeconfig).To(Equal(mustLoadKubeconfig(tt.wantKubeconfig)))
			if tt.wantReconnected {
				g.Expect(oldCache.stopped).To(BeTrue())
				g.Expect(connection.cache).ToNot(BeIdenticalTo(oldCache))
				g.Expect(connection.tokenHolder.Token()).To(HaveField("AccessToken", tt.wantToken))
				return
			}
			g.Expect(oldCache.stopped).To(BeFalse())
			g.Expect(connection.cache).To(BeIdenticalTo(oldCache))
			if tt.wantErr {
				g.Expect(accessor.lockedState.lastConnectionCreationErrorTime.IsZero()).To(BeTrue())
				g.Expect(accessor.lockedState.healthChecking).To(Equal(clusterAccessorLockedHealthCheckingState{}))
			}
			if tt.initialTokenHolder != nil {
				g.Expect(tt.initialTokenHolder.Token()).To(HaveField("AccessToken", tt.wantToken))
			}
		})
	}
}

func TestWatch(t *testing.T) {
	g := NewWithT(t)

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, testCluster)).To(Succeed()) }()

	// Create kubeconfig Secret
	kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
	g.Expect(env.CreateAndWait(ctx, kubeconfigSecret)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, kubeconfigSecret)).To(Succeed()) }()

	config := buildClusterAccessorConfig(env.GetScheme(), Options{
		SecretCachingClient: env.GetClient(),
		Client: ClientOptions{
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Timeout:   10 * time.Second,
		},
	}, nil)
	accessor := newClusterAccessor(context.Background(), clusterKey, config)

	tw := &testWatcher{}
	wi := WatcherOptions{
		Name:         "test-watch",
		Watcher:      tw,
		Kind:         &corev1.Node{},
		EventHandler: &handler.EnqueueRequestForObject{},
	}

	// Add watch when not connected (fails)
	err := accessor.Watch(ctx, NewWatcher(wi))
	g.Expect(err).To(HaveOccurred())
	g.Expect(errors.Is(err, ErrClusterNotConnected)).To(BeTrue())

	// Connect
	g.Expect(accessor.Connect(ctx)).To(Succeed())
	g.Expect(accessor.Connected(ctx)).To(BeTrue())

	g.Expect(accessor.lockedState.connection.watches).To(BeEmpty())

	// Add watch
	g.Expect(accessor.Watch(ctx, NewWatcher(wi))).To(Succeed())
	g.Expect(accessor.lockedState.connection.watches.Has("test-watch")).To(BeTrue())
	g.Expect(accessor.lockedState.connection.watches.Len()).To(Equal(1))

	// Add watch again (no-op as watch already exists)
	g.Expect(accessor.Watch(ctx, NewWatcher(wi))).To(Succeed())
	g.Expect(accessor.lockedState.connection.watches.Has("test-watch")).To(BeTrue())
	g.Expect(accessor.lockedState.connection.watches.Len()).To(Equal(1))

	// Disconnect
	accessor.Disconnect(ctx)
	g.Expect(accessor.Connected(ctx)).To(BeFalse())
}

func TestConnectWithDefaultTransform(t *testing.T) {
	g := NewWithT(t)

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-transform",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, testCluster)).To(Succeed()) }()

	kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
	g.Expect(env.CreateAndWait(ctx, kubeconfigSecret)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, kubeconfigSecret)).To(Succeed()) }()

	config := buildClusterAccessorConfig(env.GetScheme(), Options{
		SecretCachingClient: env.GetClient(),
		Client: ClientOptions{
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Timeout:   10 * time.Second,
		},
		Cache: CacheOptions{
			DefaultTransform: ctrlcache.TransformStripManagedFields(),
		},
	}, nil)
	accessor := newClusterAccessor(context.Background(), clusterKey, config)
	g.Expect(accessor.Connect(ctx)).To(Succeed())
	defer accessor.Disconnect(ctx)

	// Wait for the per-cluster cache to sync.
	cacheSyncCtx, cacheSyncCtxCancel := context.WithTimeout(ctx, 5*time.Second)
	defer cacheSyncCtxCancel()
	g.Expect(accessor.lockedState.connection.cache.WaitForCacheSync(cacheSyncCtx)).To(BeTrue())

	// Get the default Namespace via the uncached client: managedFields should be present
	// because the uncached client fetches directly from the API server.
	uncachedClient, err := accessor.GetUncachedClient(ctx)
	g.Expect(err).ToNot(HaveOccurred())
	nsUncached := &corev1.Namespace{}
	g.Expect(uncachedClient.Get(ctx, client.ObjectKey{Name: metav1.NamespaceDefault}, nsUncached)).To(Succeed())
	g.Expect(nsUncached.ManagedFields).ToNot(BeEmpty())

	// Get the same Namespace via the cached client: managedFields should be empty
	// because DefaultTransform: cache.TransformStripManagedFields() was set.
	cachedClient, err := accessor.GetClient(ctx)
	g.Expect(err).ToNot(HaveOccurred())
	nsCached := &corev1.Namespace{}
	g.Expect(cachedClient.Get(ctx, client.ObjectKey{Name: metav1.NamespaceDefault}, nsCached)).To(Succeed())
	g.Expect(nsCached.ManagedFields).To(BeEmpty())
}

type testWatcher struct {
	watchedSources []source.Source
}

func (w *testWatcher) Watch(src source.Source) error {
	w.watchedSources = append(w.watchedSources, src)
	return nil
}

func header() http.Header {
	header := http.Header{}
	header.Set("Content-Type", runtime.ContentTypeJSON)
	return header
}

func objBody(obj runtime.Object) io.ReadCloser {
	corev1GV := schema.GroupVersion{Version: "v1"}
	corev1Codec := scheme.Codecs.CodecForVersions(scheme.Codecs.LegacyCodec(corev1GV), scheme.Codecs.UniversalDecoder(corev1GV), corev1GV, corev1GV)
	return io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(corev1Codec, obj))))
}

type testSecretReader struct {
	secret *corev1.Secret
	err    error
}

var _ client.Reader = &testSecretReader{}

func (r *testSecretReader) Get(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	if r.err != nil {
		return r.err
	}

	r.secret.DeepCopyInto(obj.(*corev1.Secret))
	return nil
}

func (r *testSecretReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return errors.New("List is not implemented")
}

func newKubeconfigSecret(cluster *clusterv1.Cluster, resourceVersion string, data []byte) *corev1.Secret {
	kubeconfigSecret := kubeconfig.GenerateSecret(cluster, data)
	kubeconfigSecret.ResourceVersion = resourceVersion
	return kubeconfigSecret
}

func newTokenKubeconfig(clusterName, server, token string, certificateAuthorityData []byte) []byte {
	const authInfoName = "test-user"

	data, err := clientcmd.Write(clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   server,
				CertificateAuthorityData: certificateAuthorityData,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			clusterName: {
				Cluster:  clusterName,
				AuthInfo: authInfoName,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			authInfoName: {
				Token: token,
			},
		},
		CurrentContext: clusterName,
	})
	if err != nil {
		panic(err)
	}
	return data
}

func withAuthInfoToken(kubeconfig []byte, name, token string) []byte {
	config, err := clientcmd.Load(kubeconfig)
	if err != nil {
		panic(err)
	}
	config.AuthInfos[name] = &clientcmdapi.AuthInfo{Token: token}

	data, err := clientcmd.Write(*config)
	if err != nil {
		panic(err)
	}
	return data
}

func withoutCurrentContext(kubeconfig []byte) []byte {
	config, err := clientcmd.Load(kubeconfig)
	if err != nil {
		panic(err)
	}
	config.CurrentContext = ""

	data, err := clientcmd.Write(*config)
	if err != nil {
		panic(err)
	}
	return data
}

func mustLoadKubeconfig(kubeconfig []byte) *clientcmdapi.Config {
	config, err := clientcmd.Load(kubeconfig)
	if err != nil {
		panic(err)
	}
	return config
}
