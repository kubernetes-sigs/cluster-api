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

package topology

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/internal/envtest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
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
}
func TestMain(m *testing.M) {
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}
	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		unstructuredCachingClient, err := client.NewDelegatingClient(
			client.NewDelegatingClientInput{
				// Use the default client for write operations.
				Client: mgr.GetClient(),
				// For read operations, use the same cache used by all the controllers but ensure
				// unstructured objects will be also cached (this does not happen with the default client).
				CacheReader:       mgr.GetCache(),
				CacheUnstructured: true,
			},
		)
		if err != nil {
			panic(fmt.Sprintf("unable to create unstructuredCachineClient: %v", err))
		}
		if err := (&ClusterReconciler{
			Client:                    mgr.GetClient(),
			APIReader:                 mgr.GetAPIReader(),
			UnstructuredCachingClient: unstructuredCachingClient,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 5}); err != nil {
			panic(fmt.Sprintf("unable to create topology cluster reconciler: %v", err))
		}
		if err := (&ClusterClassReconciler{
			Client:                    mgr.GetClient(),
			UnstructuredCachingClient: unstructuredCachingClient,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 5}); err != nil {
			panic(fmt.Sprintf("unable to create clusterclass reconciler: %v", err))
		}
	}
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(30 * time.Second)
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:                   m,
		ManagerUncachedObjs: []client.Object{},
		SetupEnv:            func(e *envtest.Environment) { env = e },
		SetupIndexes:        setupIndexes,
		SetupReconcilers:    setupReconcilers,
	}))
}
