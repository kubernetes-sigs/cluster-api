/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"fmt"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

var (
	env                 *envtest.Environment
	ctx                 = ctrl.SetupSignalHandler()
	secretCachingClient client.Client
)

func TestMain(m *testing.M) {
	setupReconcilers := func(_ context.Context, mgr ctrl.Manager) {
		var err error
		secretCachingClient, err = client.New(mgr.GetConfig(), client.Options{
			HTTPClient: mgr.GetHTTPClient(),
			Cache: &client.CacheOptions{
				Reader: mgr.GetCache(),
			},
		})
		if err != nil {
			panic(fmt.Sprintf("unable to create secretCachingClient: %v", err))
		}
	}
	req, _ := labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Exists, nil)
	clusterSecretCacheSelector := labels.NewSelector().Add(*req)
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M: m,
		CacheOptionsModifier: func(o *cache.Options) {
			o.ByObject = map[client.Object]cache.ByObject{
				// Only cache Secrets with the cluster name label.
				// This is similar to the real world.
				&corev1.Secret{}: {
					Label: clusterSecretCacheSelector,
				},
			}
		},
		ManagerUncachedObjs: []client.Object{
			&corev1.ConfigMap{},
			&corev1.Secret{},
		},
		SetupEnv:         func(e *envtest.Environment) { env = e },
		SetupReconcilers: setupReconcilers,
	}))
}
