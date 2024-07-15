/*
Copyright 2021 The Kubernetes Authors.

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

package cluster

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/clusterclass"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

var (
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
	env        *envtest.Environment
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = clusterv1.AddToScheme(fakeScheme)
	_ = apiextensionsv1.AddToScheme(fakeScheme)
	_ = expv1.AddToScheme(fakeScheme)
	_ = corev1.AddToScheme(fakeScheme)
}
func TestMain(m *testing.M) {
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
		// Set up the ClusterClassName index explicitly here. This index is ordinarily created in
		// index.AddDefaultIndexes. That doesn't happen here because the ClusterClass feature flag is not set.
		if err := index.ByClusterClassName(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}
	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		// Set up a ClusterCacheTracker and ClusterCacheReconciler to provide to controllers
		// requiring a connection to a remote cluster
		secretCachingClient, err := client.New(mgr.GetConfig(), client.Options{
			HTTPClient: mgr.GetHTTPClient(),
			Cache: &client.CacheOptions{
				Reader: mgr.GetCache(),
			},
		})
		if err != nil {
			panic(fmt.Sprintf("unable to create secretCachingClient: %v", err))
		}
		tracker, err := remote.NewClusterCacheTracker(
			mgr,
			remote.ClusterCacheTrackerOptions{
				Log:                 &ctrl.Log,
				SecretCachingClient: secretCachingClient,
			},
		)
		if err != nil {
			panic(fmt.Sprintf("unable to create cluster cache tracker: %v", err))
		}
		if err := (&remote.ClusterCacheReconciler{
			Client:  mgr.GetClient(),
			Tracker: tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start ClusterCacheReconciler: %v", err))
		}
		if err := (&Reconciler{
			Client:    mgr.GetClient(),
			APIReader: mgr.GetAPIReader(),
			Tracker:   tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("unable to create topology cluster reconciler: %v", err))
		}
		if err := (&clusterclass.Reconciler{
			Client: mgr.GetClient(),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 5}); err != nil {
			panic(fmt.Sprintf("unable to create clusterclass reconciler: %v", err))
		}
	}
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(30 * time.Second)
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:                m,
		SetupEnv:         func(e *envtest.Environment) { env = e },
		SetupIndexes:     setupIndexes,
		SetupReconcilers: setupReconcilers,
		MinK8sVersion:    "v1.22.0", // ClusterClass uses server side apply that went GA in 1.22; we do not support previous version because of bug/inconsistent behaviours in the older release.
	}))
}
