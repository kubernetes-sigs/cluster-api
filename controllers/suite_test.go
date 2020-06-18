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
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/test/helpers"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	"sigs.k8s.io/controller-runtime/pkg/log"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	timeout = time.Second * 10
)

var (
	testEnv           *helpers.TestEnvironment
	clusterReconciler *ClusterReconciler
	ctx               = context.Background()
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	By("bootstrapping test environment")
	testEnv = helpers.NewTestEnvironment()

	// Set up a ClusterCacheTracker and ClusterCacheReconciler to provide to controllers
	// requiring a connection to a remote cluster
	tracker, err := remote.NewClusterCacheTracker(
		log.Log,
		testEnv.Manager,
	)
	Expect(err).ToNot(HaveOccurred())

	Expect((&remote.ClusterCacheReconciler{
		Client:  testEnv,
		Log:     log.Log,
		Tracker: tracker,
	}).SetupWithManager(testEnv.Manager, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())

	clusterReconciler = &ClusterReconciler{
		Client:   testEnv,
		Log:      log.Log,
		recorder: testEnv.GetEventRecorderFor("cluster-controller"),
	}
	Expect(clusterReconciler.SetupWithManager(testEnv.Manager, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())
	Expect((&MachineReconciler{
		Client:   testEnv,
		Log:      log.Log,
		Tracker:  tracker,
		recorder: testEnv.GetEventRecorderFor("machine-controller"),
	}).SetupWithManager(testEnv.Manager, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())
	Expect((&MachineSetReconciler{
		Client:   testEnv,
		Log:      log.Log,
		Tracker:  tracker,
		recorder: testEnv.GetEventRecorderFor("machineset-controller"),
	}).SetupWithManager(testEnv.Manager, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())
	Expect((&MachineDeploymentReconciler{
		Client:   testEnv,
		Log:      log.Log,
		recorder: testEnv.GetEventRecorderFor("machinedeployment-controller"),
	}).SetupWithManager(testEnv.Manager, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())
	Expect((&MachineHealthCheckReconciler{
		Client:   testEnv,
		Log:      log.Log,
		Tracker:  tracker,
		recorder: testEnv.GetEventRecorderFor("machinehealthcheck-controller"),
	}).SetupWithManager(testEnv.Manager, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())

	By("starting the manager")
	go func() {
		defer GinkgoRecover()
		Expect(testEnv.StartManager()).To(Succeed())
	}()

	close(done)
}, 60)

var _ = AfterSuite(func() {
	if testEnv != nil {
		By("tearing down the test environment")
		Expect(testEnv.Stop()).To(Succeed())
	}
})
