/*
Copyright 2025 The Kubernetes Authors.

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
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/predicates"
)

func TestBuilder(t *testing.T) {
	g := NewWithT(t)

	predicateLog := ctrl.LoggerFrom(t.Context()).WithValues("controller", "test/cluster")

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	realManager, err := manager.New(&rest.Config{}, manager.Options{})
	g.Expect(err).ToNot(HaveOccurred())
	fakeSink := &fakeSink{}
	fakeManager := &fakeManager{
		Manager: realManager,
		Scheme:  scheme,
		Sink:    fakeSink,
	}

	fakeSource := source.Channel(nil, handler.EnqueueRequestsFromMapFunc(nil))

	reconciler := reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		ctrl.LoggerFrom(ctx).Info("Reconciling", "request", req)
		return reconcile.Result{}, nil
	})

	c, err := NewControllerManagedBy(fakeManager, predicateLog).
		For(&clusterv1.Cluster{}).
		Named("test/cluster").
		WatchesRawSource(fakeSource).
		Watches(&clusterv1.ClusterClass{}, handler.EnqueueRequestsFromMapFunc(nil)).
		Watches(&clusterv1.MachineDeployment{}, handler.EnqueueRequestsFromMapFunc(nil), predicates.ResourceIsTopologyOwned(fakeManager.GetScheme(), predicateLog)).
		Watches(&clusterv1.MachinePool{}, handler.EnqueueRequestsFromMapFunc(nil)).
		WithOptions(controller.Options{}).
		WithEventFilter(predicates.ResourceHasFilterLabel(fakeManager.GetScheme(), predicateLog, "labelValue")).
		Build(reconciler)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(c).ToNot(BeNil())

	typedController := (c.(*controllerWrapper)).TypedController

	// Verify ReconciliationTimeout.
	var timeDurationType time.Duration
	reconciliationTimeout := reflect.NewAt(
		reflect.TypeOf(timeDurationType),
		unsafe.Pointer(reflect.ValueOf(typedController).Elem().FieldByName("ReconciliationTimeout").UnsafeAddr()), //nolint:gosec // Using unsafe here is ~ fine.
	).Elem().Interface().(time.Duration)
	g.Expect(reconciliationTimeout).To(Equal(defaultReconciliationTimeout))

	// Verify LogConstructor.
	g.Expect(fakeSink.keysAndValues).To(Equal([]string{
		"controller", "test/cluster",
		"controllerGroup", "cluster.x-k8s.io",
		"controllerKind", "Cluster",
	}))
}

func TestTypedItemExponentialFailureRateLimiter(t *testing.T) {
	g := NewWithT(t)

	rateLimitInterval := 1 * time.Second
	rateLimiter := newTypedItemExponentialFailureRateLimiter[reconcile.Request](rateLimitInterval, 5*time.Millisecond, 1000*time.Second)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "cluster-1",
		},
	}

	durations := make([]time.Duration, 0, 100)
	for range 100 {
		durations = append(durations, rateLimiter.When(req))
	}
	// rateLimitInterval + exp. backoff
	expectedDurations := []time.Duration{
		1*time.Second + 5*time.Millisecond,
		1*time.Second + 10*time.Millisecond,
		1*time.Second + 20*time.Millisecond,
		1*time.Second + 40*time.Millisecond,
		1*time.Second + 80*time.Millisecond,
		1*time.Second + 160*time.Millisecond,
		1*time.Second + 320*time.Millisecond,
		1*time.Second + 640*time.Millisecond,
		1*time.Second + 1280*time.Millisecond,
		1*time.Second + 2560*time.Millisecond,
		1*time.Second + 5120*time.Millisecond,
		1*time.Second + 10240*time.Millisecond,
		1*time.Second + 20480*time.Millisecond,
		1*time.Second + 40960*time.Millisecond,
		1*time.Second + 1*time.Minute + 21*time.Second + 920*time.Millisecond,
		1*time.Second + 2*time.Minute + 43*time.Second + 840*time.Millisecond,
		1*time.Second + 5*time.Minute + 27*time.Second + 680*time.Millisecond,
		1*time.Second + 10*time.Minute + 55*time.Second + 360*time.Millisecond,
		16*time.Minute + 40*time.Second, // max: 1000 seconds
	}
	for i := len(expectedDurations); i < len(durations); i++ {
		expectedDurations = append(expectedDurations, 16*time.Minute+40*time.Second) // max: 1000 seconds
	}

	g.Expect(durations).To(Equal(expectedDurations))
}

type fakeManager struct {
	manager.Manager
	Scheme *runtime.Scheme
	Sink   *fakeSink
}

func (m *fakeManager) Add(_ manager.Runnable) error {
	return nil
}

func (m *fakeManager) GetScheme() *runtime.Scheme {
	return m.Scheme
}

func (m *fakeManager) GetLogger() logr.Logger {
	return logr.New(m.Sink)
}

func (m *fakeManager) GetControllerOptions() config.Controller {
	return config.Controller{}
}

type fakeSink struct {
	logr.LogSink
	keysAndValues []string
}

func (s *fakeSink) Init(_ logr.RuntimeInfo) {
}

func (s *fakeSink) WithValues(keysAndValues ...any) logr.LogSink {
	for _, item := range keysAndValues {
		s.keysAndValues = append(s.keysAndValues, item.(string))
	}
	return s
}
