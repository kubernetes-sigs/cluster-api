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
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
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
	timeout = time.Second * 30
)

var (
	testEnv           *helpers.TestEnvironment
	clusterReconciler *ClusterReconciler
	ctx               = context.Background()
)

func TestAPIs(t *testing.T) {
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(30 * time.Second)
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

	// wait for webhook port to be open prior to running tests
	testEnv.WaitForWebhooks()
	close(done)
}, 80)

var _ = AfterSuite(func() {
	if testEnv != nil {
		By("tearing down the test environment")
		Expect(testEnv.Stop()).To(Succeed())
	}
})

func ContainRefOfGroupKind(group, kind string) types.GomegaMatcher {
	return &refGroupKindMatcher{
		kind:  kind,
		group: group,
	}
}

type refGroupKindMatcher struct {
	kind  string
	group string
}

func (matcher *refGroupKindMatcher) Match(actual interface{}) (success bool, err error) {
	ownerRefs, ok := actual.([]metav1.OwnerReference)
	if !ok {
		return false, errors.Errorf("expected []metav1.OwnerReference; got %T", actual)
	}

	for _, ref := range ownerRefs {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return false, nil
		}
		if ref.Kind == matcher.kind && gv.Group == clusterv1.GroupVersion.Group {
			return true, nil
		}
	}

	return false, nil
}

func (matcher *refGroupKindMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected %+v to contain refs of Group %s and Kind %s", actual, matcher.group, matcher.kind)
}

func (matcher *refGroupKindMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected %+v not to contain refs of Group %s and Kind %s", actual, matcher.group, matcher.kind)
}
