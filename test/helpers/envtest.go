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

package helpers

import (
	"path"
	"path/filepath"
	goruntime "runtime"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type TestEnvironment struct {
	*envtest.Environment
	manager.Manager
	client.Client

	Config  *rest.Config
	scheme  *runtime.Scheme
	doneMgr chan struct{}
}

func newTestEnvironment() *TestEnvironment {
	scheme := scheme.Scheme
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(bootstrapv1.AddToScheme(scheme))

	_, filename, _, _ := goruntime.Caller(0) //nolint
	root := path.Join(path.Dir(filename), "..", "..")

	return &TestEnvironment{
		Environment: &envtest.Environment{
			ErrorIfCRDPathMissing: true,
			CRDDirectoryPaths: []string{
				filepath.Join(root, "config", "crd", "bases"),
				filepath.Join(root, "controlplane", "kubeadm", "config", "crd", "bases"),
				filepath.Join(root, "bootstrap", "kubeadm", "config", "crd", "bases"),
			},
			CRDs: []runtime.Object{
				external.TestGenericBootstrapCRD,
				external.TestGenericBootstrapTemplateCRD,
				external.TestGenericInfrastructureCRD,
				external.TestGenericInfrastructureTemplateCRD,
			},
		},
		scheme:  scheme,
		doneMgr: make(chan struct{}),
	}
}

func NewTestEnvironment() (*TestEnvironment, error) {
	env := newTestEnvironment()

	cfg, err := env.Environment.Start()
	if err != nil {
		return env, err
	}

	mgr, err := manager.New(cfg, manager.Options{
		Scheme:             env.scheme,
		MetricsBindAddress: "0",
		NewCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
			syncPeriod := 1 * time.Second
			opts.Resync = &syncPeriod
			return cache.New(config, opts)
		},
	})
	if err != nil {
		if stopErr := env.Stop(); err != nil {
			return nil, kerrors.NewAggregate([]error{err, stopErr})
		}
		return env, err
	}

	env.Config = cfg
	env.Manager = mgr
	env.Client = mgr.GetClient()

	return env, nil
}

func (t *TestEnvironment) StartManager() error {
	return t.Manager.Start(t.doneMgr)
}

func (t *TestEnvironment) Stop() error {
	close(t.doneMgr)
	return t.Environment.Stop()
}

func (t *TestEnvironment) CreateKubeconfigSecret(cluster *clusterv1.Cluster) error {
	return kubeconfig.CreateEnvTestSecret(t.Client, t.Config, cluster)
}
