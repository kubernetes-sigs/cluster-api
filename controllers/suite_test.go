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
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

func init() {
	klog.InitFlags(nil)
	logf.SetLogger(klogr.New())
}

const (
	timeout = time.Second * 10
)

var (
	cfg               *rest.Config
	k8sClient         client.Client
	testEnv           *envtest.Environment
	mgr               manager.Manager
	clusterReconciler *ClusterReconciler
	doneMgr           = make(chan struct{})
	ctx               = context.Background()
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{envtest.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDs: []*apiextensionsv1beta1.CustomResourceDefinition{
			external.TestGenericBootstrapCRD,
			external.TestGenericBootstrapTemplateCRD,
			external.TestGenericInfrastructureCRD,
			external.TestGenericInfrastructureTemplateCRD,
		},
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

	// +kubebuilder:scaffold:scheme

	By("setting up a new manager")
	mgr, err = manager.New(cfg, manager.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0",
		NewCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
			syncPeriod := 1 * time.Second
			opts.Resync = &syncPeriod
			return cache.New(config, opts)
		},
	})
	Expect(err).NotTo(HaveOccurred())
	k8sClient = mgr.GetClient()
	clusterReconciler = &ClusterReconciler{
		Client:   k8sClient,
		Log:      log.Log,
		recorder: mgr.GetEventRecorderFor("cluster-controller"),
	}
	Expect(clusterReconciler.SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())
	Expect((&MachineReconciler{
		Client:   k8sClient,
		Log:      log.Log,
		recorder: mgr.GetEventRecorderFor("machine-controller"),
	}).SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())
	Expect((&MachineSetReconciler{
		Client:         k8sClient,
		Log:            log.Log,
		TemplateCloner: &external.TemplateCloner{},
		recorder:       mgr.GetEventRecorderFor("machineset-controller"),
	}).SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())
	Expect((&MachineDeploymentReconciler{
		Client:   k8sClient,
		Log:      log.Log,
		recorder: mgr.GetEventRecorderFor("machinedeployment-controller"),
	}).SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())

	By("starting the manager")
	go func() {
		Expect(mgr.Start(doneMgr)).To(Succeed())
	}()

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("closing the manager")
	close(doneMgr)
	By("tearing down the test environment")
	Expect(testEnv.Stop()).To(Succeed())
})
