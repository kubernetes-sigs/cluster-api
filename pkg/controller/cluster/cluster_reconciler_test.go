/*
Copyright 2018 The Kubernetes Authors.

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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1alpha2 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.TODO()

	instance := &clusterv1alpha2.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec:       clusterv1alpha2.ClusterSpec{},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	c := mgr.GetClient()

	reconciler := newReconciler(mgr)
	controller, err := addController(mgr, reconciler)
	Expect(err).To(BeNil())
	reconciler.controller = controller
	defer close(StartTestManager(mgr, t))

	// Create the Cluster object and expect the Reconcile and Deployment to be created
	Expect(c.Create(ctx, instance)).To(BeNil())
	defer c.Delete(ctx, instance)

	// Make sure the Cluster exists.
	Eventually(func() bool {
		key := client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}
		if err := c.Get(ctx, key, instance); err != nil {
			return false
		}
		return true
	}, timeout).Should(BeTrue())
}
