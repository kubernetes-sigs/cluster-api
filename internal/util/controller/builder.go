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
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/cache"
	predicatesutil "sigs.k8s.io/cluster-api/util/predicates"
)

const (
	// defaultReconciliationTimeout is the default ReconciliationTimeout that is set.
	// This means that the context of a Reconcile will time out after 1m.
	defaultReconciliationTimeout = 1 * time.Minute
)

// Builder is a wrapper around controller-runtime's builder.Builder.
type Builder struct {
	builder           *builder.Builder
	mgr               manager.Manager
	predicateLog      logr.Logger
	options           controller.TypedOptions[reconcile.Request]
	forObject         client.Object
	controllerName    string
	rateLimitInterval time.Duration
}

// NewControllerManagedBy returns a new controller builder that will be started by the provided Manager.
func NewControllerManagedBy(m manager.Manager, predicateLog logr.Logger) *Builder {
	return &Builder{
		builder:      builder.ControllerManagedBy(m),
		mgr:          m,
		predicateLog: predicateLog,
	}
}

// For defines the type of Object being *reconciled*, and configures the ControllerManagedBy to respond to create / delete /
// update events by *reconciling the object*.
func (blder *Builder) For(object client.Object, opts ...builder.ForOption) *Builder {
	blder.forObject = object
	blder.builder.For(object, opts...)
	return blder
}

// Owns defines types of Objects being *generated* by the ControllerManagedBy, and configures the ControllerManagedBy to respond to
// create / delete / update events by *reconciling the owner object*.
func (blder *Builder) Owns(object client.Object, predicates ...predicate.Predicate) *Builder {
	// Note: Prepend a ResourceIsChanged predicate to all "secondary" watches, this will filter out resync events,
	//       because resync from the primary object is enough.
	// Note: Prepending it to avoid calling (potentially) more resource intensive predicates for resyncs.
	predicates = append([]predicate.Predicate{predicatesutil.ResourceIsChanged(blder.mgr.GetScheme(), blder.predicateLog)}, predicates...)
	blder.builder.Owns(object, builder.WithPredicates(predicates...))
	return blder
}

// Watches defines the type of Object to watch, and configures the ControllerManagedBy to respond to create / delete /
// update events by *reconciling the object* with the given EventHandler.
func (blder *Builder) Watches(object client.Object, eventHandler handler.TypedEventHandler[client.Object, reconcile.Request], predicates ...predicate.Predicate) *Builder {
	// Note: Prepend a ResourceIsChanged predicate to all "secondary" watches, this will filter out resync events,
	//       because resync from the primary object is enough.
	// Note: Prepending it to avoid calling (potentially) more resource intensive predicates for resyncs.
	predicates = append([]predicate.Predicate{predicatesutil.ResourceIsChanged(blder.mgr.GetScheme(), blder.predicateLog)}, predicates...)
	blder.builder.Watches(object, eventHandler, builder.WithPredicates(predicates...))
	return blder
}

// WatchesMetadata is the same as Watches, but forces the internal cache to only watch PartialObjectMetadata.
func (blder *Builder) WatchesMetadata(object client.Object, eventHandler handler.TypedEventHandler[client.Object, reconcile.Request],
	predicates ...predicate.Predicate) *Builder {
	// Note: Prepend a ResourceIsChanged predicate to all "secondary" watches, this will filter out resync events,
	//       because resync from the primary object is enough.
	// Note: Prepending it to avoid calling (potentially) more resource intensive predicates for resyncs.
	predicates = append([]predicate.Predicate{predicatesutil.ResourceIsChanged(blder.mgr.GetScheme(), blder.predicateLog)}, predicates...)
	blder.builder.Watches(object, eventHandler, builder.WithPredicates(predicates...), builder.OnlyMetadata)
	return blder
}

// WatchesRawSource exposes the lower-level ControllerManagedBy Watches functions through the builder.
func (blder *Builder) WatchesRawSource(src source.TypedSource[reconcile.Request]) *Builder {
	blder.builder.WatchesRawSource(src)
	return blder
}

// WithOptions overrides the controller options used in doController. Defaults to empty.
func (blder *Builder) WithOptions(options controller.TypedOptions[reconcile.Request]) *Builder {
	blder.options = options
	return blder
}

// WithRateLimitInterval overrides the default rate limit interval, 1s.
// Note: intervals lower than 1s will be ignored.
func (blder *Builder) WithRateLimitInterval(interval time.Duration) *Builder {
	blder.rateLimitInterval = interval
	return blder
}

// WithEventFilter sets the event filters, to filter which create/update/delete/generic events eventually
// trigger reconciliations. For example, filtering on whether the resource version has changed.
func (blder *Builder) WithEventFilter(p predicate.Predicate) *Builder {
	blder.builder.WithEventFilter(p)
	return blder
}

// Named sets the name of the controller to the given name. The name shows up
// in metrics, among other things, and thus should be a prometheus compatible name
// (underscores and alphanumeric characters only).
func (blder *Builder) Named(name string) *Builder {
	blder.controllerName = name
	blder.builder.Named(name)
	return blder
}

// Controller is the controller-runtime Controller interface with
// additional methods to defer the next reconcile for a request / object.
type Controller interface {
	controller.Controller
	DeferNextReconcile(req reconcile.Request, reconcileAfter time.Time)
	DeferNextReconcileForObject(obj metav1.Object, reconcileAfter time.Time)
}

// Complete builds the Application Controller.
func (blder *Builder) Complete(r reconcile.TypedReconciler[reconcile.Request]) error {
	_, err := blder.Build(r)
	return err
}

// Build builds the Application Controller and returns the Controller it created.
func (blder *Builder) Build(r reconcile.TypedReconciler[reconcile.Request]) (Controller, error) {
	if feature.Gates.Enabled(feature.ReconcilerRateLimiting) && !feature.Gates.Enabled(feature.PriorityQueue) {
		return nil, errors.New("if feature gate ReconcilerRateLimiting is enabled, feature gate PriorityQueue must be enabled as well")
	}

	// Get GVK of the for object.
	var gvk schema.GroupVersionKind
	hasGVK := blder.forObject != nil
	if hasGVK {
		var err error
		gvk, err = apiutil.GVKForObject(blder.forObject, blder.mgr.GetScheme())
		if err != nil {
			return nil, err
		}
	}

	// Get controllerName.
	controllerName := blder.controllerName
	if controllerName == "" {
		controllerName = strings.ToLower(gvk.Kind)
	}

	// Default ReconciliationTimeout.
	// This means that the context of a Reconcile will time out after 1m.
	if blder.options.ReconciliationTimeout == 0 {
		blder.options.ReconciliationTimeout = defaultReconciliationTimeout
	}

	// Default LogConstructor.
	// This overwrites the LogConstructor defaulting in controller-runtime, because we do not want
	// to add additional & redundant name & namespace k/v pairs like controller-runtime.
	if blder.options.LogConstructor == nil {
		log := blder.mgr.GetLogger().WithValues(
			"controller", controllerName,
		)
		if hasGVK {
			log = log.WithValues(
				"controllerGroup", gvk.Group,
				"controllerKind", gvk.Kind,
			)
		}

		blder.options.LogConstructor = func(req *reconcile.Request) logr.Logger {
			// Note: This logic has to be inside the LogConstructor as this k/v pair
			// is different for every single reconcile.Request
			log := log
			if req != nil {
				if hasGVK {
					log = log.WithValues(gvk.Kind, klog.KRef(req.Namespace, req.Name))
				}
				// Note: Not setting additional name & namespace k/v pairs like controller-runtime
				// as they are redundant.
			}
			return log
		}
	}

	// Sets the rate limit interval.
	rateLimitInterval := time.Second
	if blder.rateLimitInterval > time.Second {
		rateLimitInterval = blder.rateLimitInterval
	}

	var queueRateLimiter *typedItemExponentialFailureRateLimiter[reconcile.Request]
	if feature.Gates.Enabled(feature.ReconcilerRateLimiting) {
		queueRateLimiter = newTypedItemExponentialFailureRateLimiter[reconcile.Request](rateLimitInterval, 5*time.Millisecond, 1000*time.Second)
		blder.options.RateLimiter = queueRateLimiter
	}

	// Passing the options to the underlying builder here because we modified them above.
	blder.builder.WithOptions(blder.options)

	// Create reconcileCache.
	reconcileCache := cache.New[reconcileCacheEntry](cache.DefaultTTL)

	c, err := blder.builder.Build(&reconcilerWrapper{
		name:              controllerName,
		reconciler:        r,
		reconcileCache:    reconcileCache,
		rateLimitInterval: rateLimitInterval,
		queueRateLimiter:  queueRateLimiter,
	})
	if err != nil {
		return nil, err
	}

	// Initialize metrics to align to what controller-runtime is doing for its metrics.
	// Note: This is not done for reconcileTime because we cannot add data to this metric
	// without skewing the data.
	reconcileTotal.WithLabelValues(controllerName, labelError).Add(0)
	reconcileTotal.WithLabelValues(controllerName, labelRequeueAfter).Add(0)
	reconcileTotal.WithLabelValues(controllerName, labelRequeue).Add(0)
	reconcileTotal.WithLabelValues(controllerName, labelSuccess).Add(0)

	return &controllerWrapper{
		TypedController: c,
		reconcileCache:  reconcileCache,
	}, nil
}

func newTypedItemExponentialFailureRateLimiter[T comparable](rateLimitInterval time.Duration, baseDelay time.Duration, maxDelay time.Duration) *typedItemExponentialFailureRateLimiter[T] {
	return &typedItemExponentialFailureRateLimiter[T]{
		failures:          map[T]int{},
		rateLimitInterval: rateLimitInterval,
		baseDelay:         baseDelay,
		maxDelay:          maxDelay,
	}
}

// typedItemExponentialFailureRateLimiter does a simple baseDelay*2^<num-failures> limit
// dealing with max failures and expiration are up to the caller.
// Note: In addition to the upstream TypedItemExponentialFailureRateLimiter it adds
// rateLimitInterval to the backoff duration and it changes the behavior of the Forget method.
// The Forget method is now a no-op and the ActualForget must be used to forget an item.
// This is done to ensure the reconcileWrapper has full control over forget and the forget calls
// in controller-runtime are no-ops.
type typedItemExponentialFailureRateLimiter[T comparable] struct {
	failuresLock sync.Mutex
	failures     map[T]int

	rateLimitInterval time.Duration
	baseDelay         time.Duration
	maxDelay          time.Duration
}

func (r *typedItemExponentialFailureRateLimiter[T]) When(item T) time.Duration {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	exp := r.failures[item]
	r.failures[item]++

	// The backoff is capped such that 'calculated' value never overflows.
	backoff := float64(r.rateLimitInterval) + float64(r.baseDelay.Nanoseconds())*math.Pow(2, float64(exp))
	if backoff > math.MaxInt64 {
		return r.maxDelay
	}

	calculated := time.Duration(backoff)
	if calculated > r.maxDelay {
		return r.maxDelay
	}

	return calculated
}

func (r *typedItemExponentialFailureRateLimiter[T]) NumRequeues(item T) int {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	return r.failures[item]
}

func (r *typedItemExponentialFailureRateLimiter[T]) Forget(_ T) {}

func (r *typedItemExponentialFailureRateLimiter[T]) ActualForget(item T) {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	delete(r.failures, item)
}
