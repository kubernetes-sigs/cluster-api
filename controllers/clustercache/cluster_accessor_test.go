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
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest/fake"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
)

func TestConnect(t *testing.T) {
	g := NewWithT(t)

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, testCluster)).To(Succeed()) }()

	config := buildClusterAccessorConfig(env.Manager.GetScheme(), Options{
		SecretClient: env.Manager.GetClient(),
		Client: ClientOptions{
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Timeout:   10 * time.Second,
		},
		Cache: CacheOptions{
			Indexes: []CacheOptionsIndex{NodeProviderIDIndex},
		},
	}, nil)
	accessor := newClusterAccessor(clusterKey, config)

	// Connect when kubeconfig Secret doesn't exist (should fail)
	err := accessor.Connect(ctx)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(Equal("error creating REST config: error getting kubeconfig secret: Secret \"test-cluster-kubeconfig\" not found"))
	g.Expect(accessor.Connected(ctx)).To(BeFalse())
	g.Expect(accessor.lockedState.lastConnectionCreationErrorTimestamp.IsZero()).To(BeFalse())
	accessor.lockedState.lastConnectionCreationErrorTimestamp = time.Time{} // so we can compare in the next line
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
	g.Expect(accessor.lockedState.lastConnectionCreationErrorTimestamp.IsZero()).To(BeFalse())

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

	g.Expect(accessor.lockedState.clientCertificatePrivateKey).ToNot(BeNil())

	g.Expect(accessor.lockedState.healthChecking.lastProbeTimestamp.IsZero()).To(BeFalse())
	g.Expect(accessor.lockedState.healthChecking.lastProbeSuccessTimestamp.IsZero()).To(BeFalse())
	g.Expect(accessor.lockedState.healthChecking.consecutiveFailures).To(Equal(0))

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
}

func TestDisconnect(t *testing.T) {
	g := NewWithT(t)

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, testCluster)).To(Succeed()) }()

	// Create kubeconfig Secret
	kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
	g.Expect(env.CreateAndWait(ctx, kubeconfigSecret)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, kubeconfigSecret)).To(Succeed()) }()

	config := buildClusterAccessorConfig(env.Manager.GetScheme(), Options{
		SecretClient: env.Manager.GetClient(),
		Client: ClientOptions{
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Timeout:   10 * time.Second,
		},
	}, nil)
	accessor := newClusterAccessor(clusterKey, config)

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

	// Verify health checking state was preserved
	g.Expect(accessor.lockedState.clientCertificatePrivateKey).ToNot(BeNil())

	g.Expect(accessor.lockedState.healthChecking.lastProbeTimestamp.IsZero()).To(BeFalse())
	g.Expect(accessor.lockedState.healthChecking.lastProbeSuccessTimestamp.IsZero()).To(BeFalse())
}

func TestHealthCheck(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
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

			accessor := newClusterAccessor(clusterKey, &clusterAccessorConfig{
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

func TestWatch(t *testing.T) {
	g := NewWithT(t)

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, testCluster)).To(Succeed()) }()

	// Create kubeconfig Secret
	kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
	g.Expect(env.CreateAndWait(ctx, kubeconfigSecret)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, kubeconfigSecret)).To(Succeed()) }()

	config := buildClusterAccessorConfig(env.Manager.GetScheme(), Options{
		SecretClient: env.Manager.GetClient(),
		Client: ClientOptions{
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Timeout:   10 * time.Second,
		},
	}, nil)
	accessor := newClusterAccessor(clusterKey, config)

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
