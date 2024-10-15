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
	"math"
	"math/rand/v2"
	"net/http"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest/fake"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
)

func TestReconcile(t *testing.T) {
	g := NewWithT(t)

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}
	clusterKey := client.ObjectKeyFromObject(testCluster)
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
	defer func() { g.Expect(client.IgnoreNotFound(env.CleanupAndWait(ctx, testCluster))).To(Succeed()) }()

	opts := Options{
		SecretClient: env.Manager.GetClient(),
		Client: ClientOptions{
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Timeout:   10 * time.Second,
		},
		Cache: CacheOptions{
			Indexes: []CacheOptionsIndex{NodeProviderIDIndex},
		},
	}
	accessorConfig := buildClusterAccessorConfig(env.Manager.GetScheme(), opts, nil)
	cc := &clusterCache{
		// Use APIReader to avoid cache issues when reading the Cluster object.
		client:                env.Manager.GetAPIReader(),
		clusterAccessorConfig: accessorConfig,
		clusterAccessors:      make(map[client.ObjectKey]*clusterAccessor),
	}

	// Add a Cluster source and start it (queue will be later used to verify the source works correctly)
	clusterQueue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
		workqueue.TypedRateLimitingQueueConfig[reconcile.Request]{
			Name: "test-controller",
		})
	g.Expect(cc.GetClusterSource("test-controller", func(_ context.Context, o client.Object) []ctrl.Request {
		return []ctrl.Request{{NamespacedName: client.ObjectKeyFromObject(o)}}
	}).Start(ctx, clusterQueue)).To(Succeed())

	// Reconcile, Cluster.Status.InfrastructureReady == false
	// => we expect no requeue.
	res, err := cc.Reconcile(ctx, reconcile.Request{NamespacedName: clusterKey})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.IsZero()).To(BeTrue())

	// Set Cluster.Status.InfrastructureReady == true
	patch := client.MergeFrom(testCluster.DeepCopy())
	testCluster.Status.InfrastructureReady = true
	g.Expect(env.Status().Patch(ctx, testCluster, patch)).To(Succeed())

	// Reconcile, kubeconfig Secret doesn't exist
	// => accessor.Connect will fail so we expect a retry with ConnectionCreationRetryInterval.
	res, err = cc.Reconcile(ctx, reconcile.Request{NamespacedName: clusterKey})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).To(Equal(reconcile.Result{RequeueAfter: accessorConfig.ConnectionCreationRetryInterval}))

	// Create kubeconfig Secret
	kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
	g.Expect(env.CreateAndWait(ctx, kubeconfigSecret)).To(Succeed())
	defer func() { g.Expect(env.CleanupAndWait(ctx, kubeconfigSecret)).To(Succeed()) }()

	// Reconcile again
	// => accessor.Connect just failed, so because of rate-limiting we expect a retry with
	//    slightly less than ConnectionCreationRetryInterval.
	res, err = cc.Reconcile(ctx, reconcile.Request{NamespacedName: clusterKey})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.RequeueAfter >= accessorConfig.ConnectionCreationRetryInterval-2*time.Second).To(BeTrue())
	g.Expect(res.RequeueAfter <= accessorConfig.ConnectionCreationRetryInterval).To(BeTrue())

	// Set lastConnectionCreationErrorTimestamp to now - ConnectionCreationRetryInterval to skip over rate-limiting
	cc.getClusterAccessor(clusterKey).lockedState.lastConnectionCreationErrorTimestamp = time.Now().Add(-1 * accessorConfig.ConnectionCreationRetryInterval)

	// Reconcile again, accessor.Connect works now
	// => because accessor.Connect just set the lastProbeTimestamp we expect a retry with
	//    slightly less than HealthProbe.Interval
	res, err = cc.Reconcile(ctx, reconcile.Request{NamespacedName: clusterKey})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.RequeueAfter >= accessorConfig.HealthProbe.Interval-2*time.Second).To(BeTrue())
	g.Expect(res.RequeueAfter <= accessorConfig.HealthProbe.Interval).To(BeTrue())
	g.Expect(cc.getClusterAccessor(clusterKey).Connected(ctx)).To(BeTrue())

	// Reconcile again
	// => we still expect a retry with slightly less than HealthProbe.Interval
	res, err = cc.Reconcile(ctx, reconcile.Request{NamespacedName: clusterKey})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.RequeueAfter >= accessorConfig.HealthProbe.Interval-2*time.Second).To(BeTrue())
	g.Expect(res.RequeueAfter <= accessorConfig.HealthProbe.Interval).To(BeTrue())

	// Set last probe timestamps to now - accessorConfig.HealthProbe.Interval to skip over rate-limiting
	cc.getClusterAccessor(clusterKey).lockedState.healthChecking.lastProbeTimestamp = time.Now().Add(-1 * accessorConfig.HealthProbe.Interval)
	cc.getClusterAccessor(clusterKey).lockedState.healthChecking.lastProbeSuccessTimestamp = time.Now().Add(-1 * accessorConfig.HealthProbe.Interval)

	// Reconcile again, now the health probe will be run successfully
	// => so we expect a retry with slightly less than HealthProbe.Interval
	res, err = cc.Reconcile(ctx, reconcile.Request{NamespacedName: clusterKey})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.RequeueAfter >= accessorConfig.HealthProbe.Interval-2*time.Second).To(BeTrue())
	g.Expect(res.RequeueAfter <= accessorConfig.HealthProbe.Interval).To(BeTrue())
	g.Expect(cc.getClusterAccessor(clusterKey).Connected(ctx)).To(BeTrue())
	g.Expect(cc.getClusterAccessor(clusterKey).lockedState.healthChecking.consecutiveFailures).To(Equal(0))

	// Exchange the REST client, so the next health probe will return an unauthorized error
	cc.getClusterAccessor(clusterKey).lockedState.connection.restClient = &fake.RESTClient{
		NegotiatedSerializer: scheme.Codecs,
		Resp: &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     header(),
			Body:       objBody(&apierrors.NewUnauthorized("authorization failed").ErrStatus),
		},
	}
	// Set last probe timestamps to now - accessorConfig.HealthProbe.Interval to skip over rate-limiting
	cc.getClusterAccessor(clusterKey).lockedState.healthChecking.lastProbeTimestamp = time.Now().Add(-1 * accessorConfig.HealthProbe.Interval)
	cc.getClusterAccessor(clusterKey).lockedState.healthChecking.lastProbeSuccessTimestamp = time.Now().Add(-1 * accessorConfig.HealthProbe.Interval)

	// Reconcile again, now the health probe will fail
	// => so we expect a disconnect and an immediate retry.
	res, err = cc.Reconcile(ctx, reconcile.Request{NamespacedName: clusterKey})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).To(Equal(reconcile.Result{RequeueAfter: 1 * time.Millisecond}))

	// At this point there should still be a cluster accessor, but it is disconnected
	_, ok := cc.clusterAccessors[clusterKey]
	g.Expect(ok).To(BeTrue())
	g.Expect(cc.getClusterAccessor(clusterKey).Connected(ctx)).To(BeFalse())
	// There should be one cluster source with an entry for lastEventSentTime for the current cluster.
	g.Expect(cc.clusterSources).To(HaveLen(1))
	_, ok = cc.clusterSources[0].lastEventSentTimeByCluster[clusterKey]
	g.Expect(ok).To(BeTrue())

	// Delete the Cluster
	g.Expect(env.CleanupAndWait(ctx, testCluster)).To(Succeed())

	// Reconcile again, Cluster has been deleted
	// => so we expect another Disconnect (no-op) and cleanup of the cluster accessor and cluster sources entries
	res, err = cc.Reconcile(ctx, reconcile.Request{NamespacedName: clusterKey})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.IsZero()).To(BeTrue())

	// Cluster accessor should have been removed
	_, ok = cc.clusterAccessors[clusterKey]
	g.Expect(ok).To(BeFalse())
	// lastEventSentTime in cluster source for the current cluster should have been removed.
	g.Expect(cc.clusterSources).To(HaveLen(1))
	_, ok = cc.clusterSources[0].lastEventSentTimeByCluster[clusterKey]
	g.Expect(ok).To(BeFalse())

	// Verify Cluster queue.
	g.Expect(clusterQueue.Len()).To(Equal(1))
	g.Expect(clusterQueue.Get()).To(Equal(reconcile.Request{NamespacedName: clusterKey}))
}

func TestShouldRequeue(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name             string
		now              time.Time
		lastExecution    time.Time
		interval         time.Duration
		wantRequeue      bool
		wantRequeueAfter time.Duration
	}{
		{
			name:             "Don't requeue first execution (interval: 20s)",
			now:              now,
			lastExecution:    time.Time{},
			interval:         20 * time.Second,
			wantRequeue:      false,
			wantRequeueAfter: time.Duration(0),
		},
		{
			name:             "Requeue after 15s last execution was 5s ago (interval: 20s)",
			now:              now,
			lastExecution:    now.Add(-time.Duration(5) * time.Second),
			interval:         20 * time.Second,
			wantRequeue:      true,
			wantRequeueAfter: time.Duration(15) * time.Second,
		},
		{
			name:             "Don't requeue last execution was 20s ago (interval: 20s)",
			now:              now,
			lastExecution:    now.Add(-time.Duration(20) * time.Second),
			interval:         20 * time.Second,
			wantRequeue:      false,
			wantRequeueAfter: time.Duration(0),
		},
		{
			name:             "Don't requeue last execution was 60s ago (interval: 20s)",
			now:              now,
			lastExecution:    now.Add(-time.Duration(60) * time.Second),
			interval:         20 * time.Second,
			wantRequeue:      false,
			wantRequeueAfter: time.Duration(0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotRequeueAfter, gotRequeue := shouldRequeue(tt.now, tt.lastExecution, tt.interval)
			g.Expect(gotRequeue).To(Equal(tt.wantRequeue))
			g.Expect(gotRequeueAfter).To(Equal(tt.wantRequeueAfter))
		})
	}
}

func TestMinDurationOrDefault(t *testing.T) {
	tests := []struct {
		name            string
		durations       []time.Duration
		defaultDuration time.Duration
		wantDuration    time.Duration
	}{
		{
			name:            "nil durations use default duration",
			durations:       nil,
			defaultDuration: defaultRequeueAfter,
			wantDuration:    defaultRequeueAfter,
		},
		{
			name:            "empty durations use default duration",
			durations:       []time.Duration{},
			defaultDuration: defaultRequeueAfter,
			wantDuration:    defaultRequeueAfter,
		},
		{
			name:            "use min duration",
			durations:       []time.Duration{5 * time.Second, 10 * time.Second},
			defaultDuration: defaultRequeueAfter,
			wantDuration:    5 * time.Second,
		},
		{
			name:            "use min duration even if default duration is smaller",
			durations:       []time.Duration{5 * time.Hour, 10 * time.Hour},
			defaultDuration: defaultRequeueAfter,
			wantDuration:    5 * time.Hour,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotDuration := minDurationOrDefault(tt.durations, tt.defaultDuration)
			g.Expect(gotDuration).To(Equal(tt.wantDuration))
		})
	}
}

func TestSendEventsToClusterSources(t *testing.T) {
	now := time.Now()

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	type testClusterSources struct {
		controllerName                      string
		sendEventAfterProbeFailureDurations []time.Duration
		lastEventSentTime                   time.Time
	}

	tests := []struct {
		name                    string
		now                     time.Time
		lastProbeSuccessTime    time.Time
		didConnect              bool
		didDisconnect           bool
		clusterSources          []testClusterSources
		wantEventsToControllers []string
	}{
		{
			name:                 "send event because of connect",
			now:                  now,
			lastProbeSuccessTime: now,
			didConnect:           true,
			clusterSources: []testClusterSources{
				{
					controllerName: "cluster",
				},
				{
					controllerName: "machineset",
				},
				{
					controllerName: "machine",
				},
			},
			wantEventsToControllers: []string{"cluster", "machineset", "machine"},
		},
		{
			name:                 "send event because of probe failure to some controllers",
			now:                  now,
			lastProbeSuccessTime: now.Add(-60 * time.Second), // probe last succeeded 60s ago.
			clusterSources: []testClusterSources{
				{
					controllerName:    "cluster",
					lastEventSentTime: now.Add(-40 * time.Second), // last event was sent 40s ago (20s after last successful probe)
					// cluster controller already got an event after 20s which is >= 5s, won't get another event
					sendEventAfterProbeFailureDurations: []time.Duration{5 * time.Second},
				},
				{
					controllerName:    "machineset",
					lastEventSentTime: now.Add(-40 * time.Second), // last event was sent 40s ago (20s after last successful probe)
					// machineset controller didn't get an event after >= 30s, will get an event
					sendEventAfterProbeFailureDurations: []time.Duration{30 * time.Second},
				},
				{
					controllerName:    "machine",
					lastEventSentTime: now.Add(-40 * time.Second), // last event was sent 40s ago (20s after last successful probe)
					// machine controller won't get an event as event should be sent at -60s + 120s = now + 60s
					sendEventAfterProbeFailureDurations: []time.Duration{120 * time.Second},
				},
			},
			wantEventsToControllers: []string{"machineset"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotEventsToController := []string{}
			var lock sync.Mutex
			var wg sync.WaitGroup

			cc := &clusterCache{}
			for i, cs := range tt.clusterSources {
				// Create cluster source
				opts := []GetClusterSourceOption{}
				for _, sendEventAfterProbeFailureDuration := range cs.sendEventAfterProbeFailureDurations {
					opts = append(opts, WatchForProbeFailure(sendEventAfterProbeFailureDuration))
				}
				g.Expect(cc.GetClusterSource(cs.controllerName, func(_ context.Context, o client.Object) []ctrl.Request {
					return []ctrl.Request{{NamespacedName: client.ObjectKeyFromObject(o)}}
				}, opts...)).ToNot(BeNil())

				cc.clusterSources[i].lastEventSentTimeByCluster[client.ObjectKeyFromObject(testCluster)] = cs.lastEventSentTime

				// Create go routine to read events from source and store them in gotEventsToController
				wg.Add(1)
				go func(ch <-chan event.GenericEvent) {
					for {
						_, ok := <-ch
						if !ok {
							wg.Done()
							return
						}
						lock.Lock()
						gotEventsToController = append(gotEventsToController, cs.controllerName)
						lock.Unlock()
					}
				}(cc.clusterSources[i].ch)
			}

			cc.sendEventsToClusterSources(ctx, testCluster, tt.now, tt.lastProbeSuccessTime, tt.didConnect, tt.didDisconnect)

			for _, cs := range cc.clusterSources {
				close(cs.ch)
			}
			wg.Wait()

			g.Expect(gotEventsToController).To(ConsistOf(tt.wantEventsToControllers))
		})
	}
}

func TestShouldSentEvent(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name                                string
		now                                 time.Time
		lastProbeSuccessTime                time.Time
		lastEventSentTime                   time.Time
		didConnect                          bool
		didDisconnect                       bool
		sendEventAfterProbeFailureDurations []time.Duration
		wantReasons                         []string
	}{
		{
			name:                 "send event because of connect",
			now:                  now,
			lastProbeSuccessTime: now,
			lastEventSentTime:    now.Add(-5 * time.Second),
			didConnect:           true,
			didDisconnect:        false,
			wantReasons:          []string{"connect"},
		},
		{
			name:                 "send event because of disconnect",
			now:                  now,
			lastProbeSuccessTime: now,
			lastEventSentTime:    now.Add(-5 * time.Second),
			didConnect:           false,
			didDisconnect:        true,
			wantReasons:          []string{"disconnect"},
		},
		{
			name:                                "send event, last probe success is longer ago than failure duration",
			now:                                 now,
			lastProbeSuccessTime:                now.Add(-60 * time.Second),        // probe last succeeded 60s ago.
			lastEventSentTime:                   now.Add(-40 * time.Second),        // last event was sent 40s ago (20s after last successful probe).
			sendEventAfterProbeFailureDurations: []time.Duration{21 * time.Second}, // event should be sent 21s after last successful probe
			wantReasons:                         []string{"health probe didn't succeed since more than 21s"},
		},
		{
			name:                                "don't send event, last probe success is longer ago than failure duration, but an event was already sent",
			now:                                 now,
			lastProbeSuccessTime:                now.Add(-60 * time.Second),        // probe last succeeded 60s ago.
			lastEventSentTime:                   now.Add(-39 * time.Second),        // last event was sent 39s ago (21s after last successful probe).
			sendEventAfterProbeFailureDurations: []time.Duration{21 * time.Second}, // event should be sent 21s after last successful probe
			wantReasons:                         nil,
		},
		{
			name:                                "don't send event, last probe success is not longer ago than failure duration (event should be send in 1s)",
			now:                                 now,
			lastProbeSuccessTime:                now.Add(-60 * time.Second),        // probe last succeeded 60s ago.
			lastEventSentTime:                   now.Add(-39 * time.Second),        // last event was sent 39s ago (21s after last successful probe).
			sendEventAfterProbeFailureDurations: []time.Duration{61 * time.Second}, // event should be sent 61s after last successful probe
			wantReasons:                         nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotReasons := shouldSendEvent(tt.now, tt.lastProbeSuccessTime, tt.lastEventSentTime, tt.didConnect, tt.didDisconnect, tt.sendEventAfterProbeFailureDurations)
			g.Expect(gotReasons).To(Equal(tt.wantReasons))
		})
	}
}

type testCluster struct {
	cluster          client.ObjectKey
	brokenRESTConfig bool
}

func TestClusterCacheConcurrency(t *testing.T) {
	g := NewWithT(t)

	// Scale parameters for the test.
	const (
		// ClusterCache will only be run with 10 workers (default is 100).
		// So in general 10x clusterCount should be at least possible
		clusterCacheConcurrency = 10

		// clusterCount is the number of clusters we create
		clusterCount = 100
		// brokenClusterPercentage is the percentage of clusters that are broken,
		// i.e. they'll have a broken REST config and connection creation will time out.
		brokenClusterPercentage = 20 // 20%
		// clusterCreationConcurrency is the number of workers that are creating Cluster objects in parallel.
		clusterCreationConcurrency = 10

		// clusterCacheConsumerConcurrency is the number of workers for consumers that use the ClusterCache,
		// i.e. this is the equivalent of the sum of workers of all reconcilers calling .GetClient etc.
		clusterCacheConsumerConcurrency = 100

		// clusterCacheConsumerDuration is the duration for which we let the consumers run.
		clusterCacheConsumerDuration = 1 * time.Minute
	)

	// Set up ClusterCache.
	cc, err := SetupWithManager(ctx, env.Manager, Options{
		SecretClient: env.Manager.GetClient(),
		Cache: CacheOptions{
			Indexes: []CacheOptionsIndex{NodeProviderIDIndex},
		},
		Client: ClientOptions{
			QPS:       20,
			Burst:     30,
			UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
			Timeout:   10 * time.Second,
			Cache: ClientCacheOptions{
				DisableFor: []client.Object{
					// Don't cache ConfigMaps & Secrets.
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
	}, controller.Options{MaxConcurrentReconciles: clusterCacheConcurrency})
	g.Expect(err).ToNot(HaveOccurred())
	internalClusterCache, ok := cc.(*clusterCache)
	g.Expect(ok).To(BeTrue())

	// Generate test clusters.
	testClusters := generateTestClusters(clusterCount, brokenClusterPercentage)

	// Create test clusters concurrently
	start := time.Now()
	inputChan := make(chan testCluster)
	var wg sync.WaitGroup
	for range clusterCreationConcurrency {
		wg.Add(1)
		go func() {
			for {
				testCluster, ok := <-inputChan
				if !ok {
					wg.Done()
					return
				}
				createCluster(g, testCluster)
			}
		}()
	}
	go func() {
		for _, testCluster := range testClusters {
			inputChan <- testCluster
		}
		// All clusters are requested, close the channel to shut down workers.
		close(inputChan)
	}()
	wg.Wait()
	afterClusterCreation := time.Since(start)
	t.Logf("Created %d clusters in %s", clusterCount, afterClusterCreation)

	// Ensure ClusterCache had a chance to run Connect for each Cluster at least once.
	// Otherwise we e.g. can't assume below that every working cluster is already connected.
	g.Eventually(func(g Gomega) {
		for _, tc := range testClusters {
			accessor := internalClusterCache.getClusterAccessor(tc.cluster)
			g.Expect(accessor).ToNot(BeNil())
			if tc.brokenRESTConfig {
				g.Expect(accessor.GetLastConnectionCreationErrorTimestamp(ctx).IsZero()).To(BeFalse())
			} else {
				g.Expect(accessor.Connected(ctx)).To(BeTrue())
			}
		}
	}, 3*time.Minute, 5*time.Second).Should(Succeed())

	t.Logf("ClusterCache ran Connect at least once per Cluster now")

	// Run some workers to simulate other reconcilers using the ClusterCache
	errChan := make(chan error)
	errStopChan := make(chan struct{})
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	errs := []error{}
	for range clusterCacheConsumerConcurrency {
		wg.Add(1)
		go func(ctx context.Context) {
			for {
				select {
				case <-consumerCtx.Done():
					wg.Done()
					return
				default:
				}

				// Pick a random cluster
				tc := testClusters[rand.IntN(len(testClusters))] //nolint:gosec // weak random numbers are fine

				// Try to get a client.
				c, err := cc.GetClient(ctx, tc.cluster)

				if tc.brokenRESTConfig {
					// If the cluster has a broken REST config, validate:
					// * we got an error when getting the client
					// * the accessor is not connected
					// * connection creation was tried in a reasonable timeframe (ConnectionCreationRetryInterval*1,2)
					//   As a lot of clusters are enqueued at the same time we have to give a bit of buffer (20% for now).
					if err == nil {
						errChan <- pkgerrors.Errorf("cluster %s: expected an error when getting client", tc.cluster)
						continue
					}

					accessor := internalClusterCache.getClusterAccessor(tc.cluster)
					if accessor.Connected(ctx) {
						errChan <- pkgerrors.Errorf("cluster %s: expected accessor to not be connected", tc.cluster)
						continue
					}

					lastConnectionCreationErrorTimestamp := accessor.GetLastConnectionCreationErrorTimestamp(ctx)
					if time.Since(lastConnectionCreationErrorTimestamp) > (internalClusterCache.clusterAccessorConfig.ConnectionCreationRetryInterval * 120 / 100) {
						errChan <- pkgerrors.Wrapf(err, "cluster %s: connection creation wasn't tried within the connection creation retry interval", tc.cluster)
						continue
					}
				} else {
					// If the cluster has a valid REST config, validate:
					// * we didn't get an error when getting the client
					// * the accessor is connected
					// * the client works, by listing ConfigMaps
					// * health probe was run in a reasonable timeframe (HealthProbe.Interval*1,2)
					//   As a lot of clusters are enqueued at the same time we have to give a bit of buffer (20% for now).
					if err != nil {
						errChan <- pkgerrors.Wrapf(err, "cluster %s: got unexpected error when getting client: %v", tc.cluster, err)
						continue
					}

					accessor := internalClusterCache.getClusterAccessor(tc.cluster)
					if !accessor.Connected(ctx) {
						errChan <- pkgerrors.Errorf("cluster %s: expected accessor to be connected", tc.cluster)
						continue
					}

					if err := c.List(ctx, &corev1.NodeList{}); err != nil {
						errChan <- pkgerrors.Wrapf(err, "cluster %s: unexpected error when using client to list ConfigMaps: %v", tc.cluster, err)
						continue
					}

					lastProbeSuccessTimestamp := cc.GetLastProbeSuccessTimestamp(ctx, tc.cluster)
					if time.Since(lastProbeSuccessTimestamp) > (internalClusterCache.clusterAccessorConfig.HealthProbe.Interval * 120 / 100) {
						errChan <- pkgerrors.Wrapf(err, "cluster %s: health probe wasn't run successfully within the health probe interval", tc.cluster)
						continue
					}
				}

				// Wait before the next iteration
				time.Sleep(10 * time.Millisecond)
			}
		}(consumerCtx)
	}
	go func() {
		for {
			err, ok := <-errChan
			if !ok {
				errStopChan <- struct{}{}
				return
			}
			errs = append(errs, err)
		}
	}()

	t.Logf("Consumers will now run for %s", clusterCacheConsumerDuration)
	time.Sleep(clusterCacheConsumerDuration)

	// Cancel consumer go routines.
	consumerCancel()
	// Wait until consumer go routines are done.
	wg.Wait()
	// Close error channel to stop error collection go routine.
	close(errChan)
	// Wait until error collection go routine is done.
	<-errStopChan

	// Validate that no errors occurred.
	// Note: This test is also run with the race detector to detect race conditions.
	if len(errs) > 0 {
		g.Expect(kerrors.NewAggregate(errs)).ToNot(HaveOccurred())
	}

	// The expectation is that the ClusterCache Reconciler never returns errors to avoid exponential backoff.
	errorsTotal, err := getCounterMetric("controller_runtime_reconcile_errors_total", "clustercache")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(errorsTotal).To(Equal(float64(0)))

	panicsTotal, err := getCounterMetric("controller_runtime_reconcile_panics_total", "clustercache")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(panicsTotal).To(Equal(float64(0)))
}

func generateTestClusters(clusterCount, brokenClusterPercentage int) []testCluster {
	testClusters := make([]testCluster, 0, clusterCount)

	clusterNameDigits := 1 + int(math.Log10(float64(clusterCount)))
	for i := 1; i <= clusterCount; i++ {
		// This ensures we always have the right number of leading zeros in our cluster names, e.g.
		// clusterCount=1000 will lead to cluster names like scale-0001, scale-0002, ... .
		// This makes it possible to have correct ordering of clusters in diagrams in tools like Grafana.
		name := fmt.Sprintf("%s-%0*d", "cluster", clusterNameDigits, i)
		testClusters = append(testClusters, testCluster{
			cluster: client.ObjectKey{
				Name:      name,
				Namespace: metav1.NamespaceDefault,
			},
			brokenRESTConfig: i%(100/brokenClusterPercentage) == 0,
		})
	}
	return testClusters
}

func createCluster(g Gomega, testCluster testCluster) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testCluster.cluster.Name,
			Namespace: testCluster.cluster.Namespace,
		},
	}
	g.Expect(env.CreateAndWait(ctx, cluster)).To(Succeed())

	if !testCluster.brokenRESTConfig {
		g.Expect(env.CreateKubeconfigSecret(ctx, cluster)).To(Succeed())
	} else {
		// Create invalid kubeconfig Secret
		kubeconfigBytes := kubeconfig.FromEnvTestConfig(env.Config, cluster)
		cmdConfig, err := clientcmd.Load(kubeconfigBytes)
		g.Expect(err).ToNot(HaveOccurred())
		cmdConfig.Clusters[cluster.Name].Server = "https://1.2.3.4" // breaks the server URL.
		kubeconfigBytes, err = clientcmd.Write(*cmdConfig)
		g.Expect(err).ToNot(HaveOccurred())
		kubeconfigSecret := kubeconfig.GenerateSecret(cluster, kubeconfigBytes)
		g.Expect(env.CreateAndWait(ctx, kubeconfigSecret)).To(Succeed())
	}

	patch := client.MergeFrom(cluster.DeepCopy())
	cluster.Status.InfrastructureReady = true
	g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())
}

func getCounterMetric(metricFamilyName, controllerName string) (float64, error) {
	metricFamilies, err := metrics.Registry.Gather()
	if err != nil {
		return 0, err
	}

	for _, metricFamily := range metricFamilies {
		if metricFamily.GetName() == metricFamilyName {
			for _, metric := range metricFamily.Metric {
				foundController := false
				for _, l := range metric.Label {
					if l.GetName() == "controller" && l.GetValue() == controllerName {
						foundController = true
					}
				}
				if !foundController {
					continue
				}

				if metric.GetCounter() != nil {
					return metric.GetCounter().GetValue(), nil
				}
			}
		}
	}

	return 0, fmt.Errorf("failed to find %q metric", metricFamilyName)
}
