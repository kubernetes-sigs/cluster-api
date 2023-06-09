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

package controller

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
	ccache "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/cache"
	chandler "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/handler"
	creconciler "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/reconcile"
	csource "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/source"
)

var scheme = runtime.NewScheme()

func init() {
	_ = cloudv1.AddToScheme(scheme)
}

type simpleReconciler struct {
	processed chan creconciler.Request
}

func (r *simpleReconciler) Reconcile(_ context.Context, req creconciler.Request) (_ creconciler.Result, err error) {
	r.processed <- req
	return creconciler.Result{}, nil
}

func TestController_Queue(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.TODO())

	r := &simpleReconciler{
		processed: make(chan creconciler.Request),
	}

	c, err := New("foo", Options{
		Concurrency: 1,
		Reconciler:  r,
	})
	g.Expect(err).ToNot(HaveOccurred())

	i := &fakeInformer{}

	err = c.Watch(&csource.Informer{
		Informer: i,
	}, &chandler.EnqueueRequestForObject{})
	g.Expect(err).ToNot(HaveOccurred())

	err = c.Start(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	i.InformCreate("foo", &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar-create",
		},
	})

	g.Eventually(func() bool {
		r := <-r.processed
		return r == creconciler.Request{
			ResourceGroup:  "foo",
			NamespacedName: types.NamespacedName{Name: "bar-create"},
		}
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

	i.InformUpdate("foo", &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar-update",
		},
	}, &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar-update",
		},
	})

	g.Eventually(func() bool {
		r := <-r.processed
		return r == creconciler.Request{
			ResourceGroup:  "foo",
			NamespacedName: types.NamespacedName{Name: "bar-update"},
		}
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

	i.InformDelete("foo", &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar-delete",
		},
	})

	g.Eventually(func() bool {
		r := <-r.processed
		return r == creconciler.Request{
			ResourceGroup:  "foo",
			NamespacedName: types.NamespacedName{Name: "bar-delete"},
		}
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

	i.InformGeneric("foo", &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar-generic",
		},
	})

	g.Eventually(func() bool {
		r := <-r.processed
		return r == creconciler.Request{
			ResourceGroup:  "foo",
			NamespacedName: types.NamespacedName{Name: "bar-generic"},
		}
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

	close(r.processed)
	cancelFn()
}

var _ ccache.Informer = &fakeInformer{}

type fakeInformer struct {
	handler ccache.InformEventHandler
}

func (i *fakeInformer) AddEventHandler(handler ccache.InformEventHandler) error {
	i.handler = handler
	return nil
}

func (i *fakeInformer) RemoveEventHandler(_ ccache.InformEventHandler) error {
	i.handler = nil
	return nil
}

func (i *fakeInformer) InformCreate(resourceGroup string, obj client.Object) {
	i.handler.OnCreate(resourceGroup, obj)
}

func (i *fakeInformer) InformUpdate(resourceGroup string, oldObj, newObj client.Object) {
	i.handler.OnUpdate(resourceGroup, oldObj, newObj)
}

func (i *fakeInformer) InformDelete(resourceGroup string, res client.Object) {
	i.handler.OnDelete(resourceGroup, res)
}

func (i *fakeInformer) InformGeneric(resourceGroup string, res client.Object) {
	i.handler.OnGeneric(resourceGroup, res)
}
