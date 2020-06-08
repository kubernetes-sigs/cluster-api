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

	"github.com/onsi/ginkgo"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func init() {
	klog.InitFlags(nil)
	log.SetLogger(klogr.New())
	klog.SetOutput(ginkgo.GinkgoWriter)
}

var (
	env *envtest.Environment
)

func init() {
	// Calculate the scheme.
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(bootstrapv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(expv1.AddToScheme(scheme.Scheme))

	// Get the root of the current file to use in CRD paths.
	_, filename, _, _ := goruntime.Caller(0) //nolint
	root := path.Join(path.Dir(filename), "..", "..")

	// Create the test environment.
	env = &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths: []string{
			filepath.Join(root, "config", "crd", "bases"),
			filepath.Join(root, "controlplane", "kubeadm", "config", "crd", "bases"),
			filepath.Join(root, "bootstrap", "kubeadm", "config", "crd", "bases"),
		},
		CRDs: []runtime.Object{
			external.TestGenericBootstrapCRD.DeepCopy(),
			external.TestGenericBootstrapTemplateCRD.DeepCopy(),
			external.TestGenericInfrastructureCRD.DeepCopy(),
			external.TestGenericInfrastructureTemplateCRD.DeepCopy(),
		},
	}
}

// TestEnvironment encapsulates a Kubernetes local test environment.
type TestEnvironment struct {
	manager.Manager
	client.Client
	Config *rest.Config

	doneMgr chan struct{}
}

// NewTestEnvironment creates a new environment spinning up a local api-server.
//
// This function should be called only once for each package you're running tests within,
// usually the environment is initialized in a suite_test.go file within a `BeforeSuite` ginkgo block.
func NewTestEnvironment() *TestEnvironment {
	if _, err := env.Start(); err != nil {
		panic(err)
	}

	mgr, err := manager.New(env.Config, manager.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0",
		NewClient:          util.ManagerDelegatingClientFunc,
	})
	if err != nil {
		klog.Fatalf("Failed to start testenv manager: %v", err)
	}

	return &TestEnvironment{
		Manager: mgr,
		Client:  mgr.GetClient(),
		Config:  mgr.GetConfig(),
		doneMgr: make(chan struct{}),
	}
}

func (t *TestEnvironment) StartManager() error {
	return t.Manager.Start(t.doneMgr)
}

func (t *TestEnvironment) Stop() error {
	t.doneMgr <- struct{}{}
	return env.Stop()
}

func (t *TestEnvironment) CreateKubeconfigSecret(cluster *clusterv1.Cluster) error {
	return kubeconfig.CreateEnvTestSecret(t.Client, t.Config, cluster)
}
