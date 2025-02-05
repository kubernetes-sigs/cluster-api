/*
Copyright 2022 The Kubernetes Authors.

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

package structuredmerge

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
}

func TestMain(m *testing.M) {
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(30 * time.Second)
	req, _ := labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Exists, nil)
	clusterSecretCacheSelector := labels.NewSelector().Add(*req)
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M: m,
		ManagerCacheOptions: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				// Only cache Secrets with the cluster name label.
				// This is similar to the real world.
				&corev1.Secret{}: {
					Label: clusterSecretCacheSelector,
				},
			},
		},
		ManagerUncachedObjs: []client.Object{
			&corev1.ConfigMap{},
			&corev1.Secret{},
		},
		SetupEnv: func(e *envtest.Environment) { env = e },
		// We are testing the patch helper against a real API Server, no need of additional indexes/reconcilers.
		MinK8sVersion: "v1.22.0", // ClusterClass uses server side apply that went GA in 1.22; we do not support previous version because of bug/inconsistent behaviours in the older release.
	}))
}
