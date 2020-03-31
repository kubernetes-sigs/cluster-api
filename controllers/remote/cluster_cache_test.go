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
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestClusterCache(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	cfg, err := testEnv.Start()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cfg).ToNot(BeNil())
	defer func() {
		g.Expect(testEnv.Stop()).To(Succeed())
	}()

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	mgr, err := manager.New(cfg, manager.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	go func() {
		err := mgr.Start(ctx.Done())
		g.Expect(err).NotTo(HaveOccurred())
	}()

	// Create the cluster
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}
	err = mgr.GetClient().Create(ctx, cluster)
	g.Expect(err).ToNot(HaveOccurred())

	clusterKey := client.ObjectKey{Namespace: "default", Name: "test"}
	g.Eventually(func() error {
		return mgr.GetClient().Get(ctx, clusterKey, cluster)
	}).Should(Succeed())

	// Create our cache manager
	m, err := NewClusterCacheManager(NewClusterCacheManagerInput{
		Log:                     &log.NullLogger{},
		Manager:                 mgr,
		ManagementClusterClient: mgr.GetClient(),
		Scheme:                  scheme,
	})
	g.Expect(err).ToNot(HaveOccurred())

	m.newRESTConfig = func(ctx context.Context, cluster client.ObjectKey) (*rest.Config, error) {
		return mgr.GetConfig(), nil
	}

	g.Expect(len(m.clusterCaches)).To(Equal(0))

	// Get a cache for our cluster
	cc, err := m.ClusterCache(ctx, clusterKey)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cc).NotTo(BeNil())
	g.Expect(len(m.clusterCaches)).To(Equal(1))

	// Get it a few more times to make sure it doesn't create more than 1 cache
	for i := 0; i < 5; i++ {
		cc, err := m.ClusterCache(ctx, clusterKey)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cc).NotTo(BeNil())
		g.Expect(len(m.clusterCaches)).To(Equal(1))
	}

	// Delete the cluster
	err = mgr.GetClient().Delete(ctx, cluster)
	g.Expect(err).ToNot(HaveOccurred())

	// Make sure it's gone
	g.Eventually(func() bool {
		err := mgr.GetClient().Get(ctx, clusterKey, cluster)
		return apierrors.IsNotFound(err)
	}, 5*time.Second).Should(BeTrue())

	// Make sure the cache was removed
	g.Eventually(func() int {
		return len(m.clusterCaches)
	}, 5*time.Second).Should(Equal(0))

	// Make sure it was stopped
	typedCache := cc.(*clusterCache)
	g.Expect(typedCache.stopped).To(BeTrue())
}
