/*
Copyright 2023 The Kubernetes Authors.

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

package cloud

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
	creconciler "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/reconcile"
)

var scheme = runtime.NewScheme()

func init() {
	_ = cloudv1.AddToScheme(scheme)
}

type simpleReconciler struct {
	mgr Manager
}

func (r *simpleReconciler) SetupWithManager(_ context.Context, mgr Manager) error {
	return NewControllerManagedBy(mgr).
		For(&cloudv1.CloudMachine{}).
		Complete(r)
}

func (r *simpleReconciler) Reconcile(ctx context.Context, req creconciler.Request) (creconciler.Result, error) {
	client := r.mgr.GetResourceGroup(req.ResourceGroup).GetClient()

	obj := &cloudv1.CloudMachine{}
	if err := client.Get(ctx, req.NamespacedName, obj); err != nil {
		return creconciler.Result{}, err
	}

	obj.Annotations = map[string]string{"reconciled": "true"}
	if err := client.Update(ctx, obj); err != nil {
		return creconciler.Result{}, err
	}
	return creconciler.Result{}, nil
}

func TestController_Queue(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.TODO())

	mgr := NewManager(scheme)
	mgr.AddResourceGroup("foo")

	c := mgr.GetResourceGroup("foo").GetClient()

	err := (&simpleReconciler{
		mgr: mgr,
	}).SetupWithManager(ctx, mgr)
	g.Expect(err).ToNot(HaveOccurred())

	err = mgr.Start(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	obj := &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar-reconcile",
		},
	}
	err = c.Create(ctx, obj)
	g.Expect(err).ToNot(HaveOccurred())

	g.Eventually(func() bool {
		obj2 := &cloudv1.CloudMachine{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj2); err != nil {
			return false
		}
		return obj2.Annotations["reconciled"] == "true"
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

	cancelFn()
}
