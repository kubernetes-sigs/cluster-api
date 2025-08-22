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

package predicates

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/labels"
)

// All returns a predicate that returns true only if all given predicates return true.
func All(scheme *runtime.Scheme, logger logr.Logger, predicates ...predicate.Funcs) predicate.Funcs {
	return TypedAll(scheme, logger, predicates...)
}

// TypedAll returns a predicate that returns true only if all given predicates return true.
func TypedAll[T client.Object](scheme *runtime.Scheme, logger logr.Logger, predicates ...predicate.TypedFuncs[T]) predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			log := logger.WithValues("predicateAggregation", "All")
			if gvk, err := apiutil.GVKForObject(e.ObjectNew, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.ObjectNew))
			}
			for _, p := range predicates {
				if !p.UpdateFunc(e) {
					log.V(6).Info("One of the provided predicates returned false, blocking further processing")
					return false
				}
			}
			log.V(6).Info("All provided predicates returned true, allowing further processing")
			return true
		},
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			log := logger.WithValues("predicateAggregation", "All")
			if gvk, err := apiutil.GVKForObject(e.Object, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.Object))
			}
			for _, p := range predicates {
				if !p.CreateFunc(e) {
					log.V(6).Info("One of the provided predicates returned false, blocking further processing")
					return false
				}
			}
			log.V(6).Info("All provided predicates returned true, allowing further processing")
			return true
		},
		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			log := logger.WithValues("predicateAggregation", "All")
			if gvk, err := apiutil.GVKForObject(e.Object, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.Object))
			}
			for _, p := range predicates {
				if !p.DeleteFunc(e) {
					log.V(6).Info("One of the provided predicates returned false, blocking further processing")
					return false
				}
			}
			log.V(6).Info("All provided predicates returned true, allowing further processing")
			return true
		},
		GenericFunc: func(e event.TypedGenericEvent[T]) bool {
			log := logger.WithValues("predicateAggregation", "All")
			if gvk, err := apiutil.GVKForObject(e.Object, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.Object))
			}
			for _, p := range predicates {
				if !p.GenericFunc(e) {
					log.V(6).Info("One of the provided predicates returned false, blocking further processing")
					return false
				}
			}
			log.V(6).Info("All provided predicates returned true, allowing further processing")
			return true
		},
	}
}

// Any returns a predicate that returns true only if any given predicate returns true.
func Any(scheme *runtime.Scheme, logger logr.Logger, predicates ...predicate.Funcs) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicateAggregation", "Any")
			if gvk, err := apiutil.GVKForObject(e.ObjectNew, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.ObjectNew))
			}
			for _, p := range predicates {
				if p.UpdateFunc(e) {
					log.V(6).Info("One of the provided predicates returned true, allowing further processing")
					return true
				}
			}
			log.V(6).Info("All of the provided predicates returned false, blocking further processing")
			return false
		},
		CreateFunc: func(e event.CreateEvent) bool {
			log := logger.WithValues("predicateAggregation", "Any")
			if gvk, err := apiutil.GVKForObject(e.Object, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.Object))
			}
			for _, p := range predicates {
				if p.CreateFunc(e) {
					log.V(6).Info("One of the provided predicates returned true, allowing further processing")
					return true
				}
			}
			log.V(6).Info("All of the provided predicates returned false, blocking further processing")
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log := logger.WithValues("predicateAggregation", "Any")
			if gvk, err := apiutil.GVKForObject(e.Object, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.Object))
			}
			for _, p := range predicates {
				if p.DeleteFunc(e) {
					log.V(6).Info("One of the provided predicates returned true, allowing further processing")
					return true
				}
			}
			log.V(6).Info("All of the provided predicates returned false, blocking further processing")
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			log := logger.WithValues("predicateAggregation", "Any")
			if gvk, err := apiutil.GVKForObject(e.Object, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.Object))
			}
			for _, p := range predicates {
				if p.GenericFunc(e) {
					log.V(6).Info("One of the provided predicates returned true, allowing further processing")
					return true
				}
			}
			log.V(6).Info("All of the provided predicates returned false, blocking further processing")
			return false
		},
	}
}

// ResourceHasFilterLabel returns a predicate that returns true only if the provided resource contains
// a label with the WatchLabel key and the configured label value exactly.
func ResourceHasFilterLabel(scheme *runtime.Scheme, logger logr.Logger, labelValue string) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfLabelMatch(scheme, logger.WithValues("predicate", "ResourceHasFilterLabel", "eventType", "update"), e.ObjectNew, labelValue)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfLabelMatch(scheme, logger.WithValues("predicate", "ResourceHasFilterLabel", "eventType", "create"), e.Object, labelValue)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfLabelMatch(scheme, logger.WithValues("predicate", "ResourceHasFilterLabel", "eventType", "delete"), e.Object, labelValue)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfLabelMatch(scheme, logger.WithValues("predicate", "ResourceHasFilterLabel", "eventType", "generic"), e.Object, labelValue)
		},
	}
}

// ResourceNotPaused returns a Predicate that returns true only if the provided resource does not contain the
// paused annotation.
// This implements a common requirement for all cluster-api and provider controllers skip reconciliation when the paused
// annotation is present for a resource.
// Example use:
//
//	func (r *MyReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
//		controller, err := ctrl.NewControllerManagedBy(mgr).
//			For(&v1.MyType{}).
//			WithOptions(options).
//			WithEventFilter(util.ResourceNotPaused(mgr.GetScheme(), r.Log)).
//			Build(r)
//		return err
//	}
func ResourceNotPaused(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfNotPaused(scheme, logger.WithValues("predicate", "ResourceNotPaused", "eventType", "update"), e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfNotPaused(scheme, logger.WithValues("predicate", "ResourceNotPaused", "eventType", "create"), e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfNotPaused(scheme, logger.WithValues("predicate", "ResourceNotPaused", "eventType", "delete"), e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfNotPaused(scheme, logger.WithValues("predicate", "ResourceNotPaused", "eventType", "generic"), e.Object)
		},
	}
}

// ResourceNotPausedAndHasFilterLabel returns a predicate that returns true only if the
// ResourceNotPaused and ResourceHasFilterLabel predicates return true.
func ResourceNotPausedAndHasFilterLabel(scheme *runtime.Scheme, logger logr.Logger, labelValue string) predicate.Funcs {
	return All(scheme, logger, ResourceNotPaused(scheme, logger), ResourceHasFilterLabel(scheme, logger, labelValue))
}

func processIfNotPaused(scheme *runtime.Scheme, logger logr.Logger, obj client.Object) bool {
	if gvk, err := apiutil.GVKForObject(obj, scheme); err == nil {
		logger = logger.WithValues(gvk.Kind, klog.KObj(obj))
	}
	if annotations.HasPaused(obj) {
		logger.V(4).Info("Resource is paused, will not attempt to map resource")
		return false
	}
	logger.V(6).Info("Resource is not paused, will attempt to map resource")
	return true
}

func processIfLabelMatch(scheme *runtime.Scheme, logger logr.Logger, obj client.Object, labelValue string) bool {
	// Return early if no labelValue was set.
	if labelValue == "" {
		return true
	}

	if gvk, err := apiutil.GVKForObject(obj, scheme); err == nil {
		logger = logger.WithValues(gvk.Kind, klog.KObj(obj))
	}
	if labels.HasWatchLabel(obj, labelValue) {
		logger.V(6).Info("Resource matches label, will attempt to map resource")
		return true
	}
	logger.V(4).Info("Resource does not match label, will not attempt to map resource")
	return false
}

// ResourceIsNotExternallyManaged returns a predicate that returns true only if the resource does not contain
// the externally managed annotation.
// This implements a requirement for InfraCluster providers to be able to ignore externally managed
// cluster infrastructure.
func ResourceIsNotExternallyManaged(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfNotExternallyManaged(scheme, logger.WithValues("predicate", "ResourceIsNotExternallyManaged", "eventType", "update"), e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfNotExternallyManaged(scheme, logger.WithValues("predicate", "ResourceIsNotExternallyManaged", "eventType", "create"), e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfNotExternallyManaged(scheme, logger.WithValues("predicate", "ResourceIsNotExternallyManaged", "eventType", "delete"), e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfNotExternallyManaged(scheme, logger.WithValues("predicate", "ResourceIsNotExternallyManaged", "eventType", "generic"), e.Object)
		},
	}
}

func processIfNotExternallyManaged(scheme *runtime.Scheme, logger logr.Logger, obj client.Object) bool {
	if gvk, err := apiutil.GVKForObject(obj, scheme); err == nil {
		logger = logger.WithValues(gvk.Kind, klog.KObj(obj))
	}
	if annotations.IsExternallyManaged(obj) {
		logger.V(4).Info("Resource is externally managed, will not attempt to map resource")
		return false
	}
	logger.V(6).Info("Resource is managed, will attempt to map resource")
	return true
}

// ResourceIsTopologyOwned returns a predicate that returns true only if the resource has
// the `topology.cluster.x-k8s.io/owned` label.
func ResourceIsTopologyOwned(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfTopologyOwned(scheme, logger.WithValues("predicate", "ResourceIsTopologyOwned", "eventType", "update"), e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfTopologyOwned(scheme, logger.WithValues("predicate", "ResourceIsTopologyOwned", "eventType", "create"), e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfTopologyOwned(scheme, logger.WithValues("predicate", "ResourceIsTopologyOwned", "eventType", "delete"), e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfTopologyOwned(scheme, logger.WithValues("predicate", "ResourceIsTopologyOwned", "eventType", "generic"), e.Object)
		},
	}
}

func processIfTopologyOwned(scheme *runtime.Scheme, logger logr.Logger, obj client.Object) bool {
	if gvk, err := apiutil.GVKForObject(obj, scheme); err == nil {
		logger = logger.WithValues(gvk.Kind, klog.KObj(obj))
	}
	if labels.IsTopologyOwned(obj) {
		logger.V(6).Info("Resource is topology owned, will attempt to map resource")
		return true
	}
	// We intentionally log this line only on level 6, because it will be very frequently
	// logged for MachineDeployments and MachineSets not owned by a topology.
	logger.V(6).Info("Resource is not topology owned, will not attempt to map resource")
	return false
}

// ResourceIsChanged returns a predicate that returns true only if the resource
// has changed. This predicate allows to drop resync events on additionally watched objects.
func ResourceIsChanged(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return TypedResourceIsChanged[client.Object](scheme, logger)
}

// TypedResourceIsChanged returns a predicate that returns true only if the resource
// has changed. This predicate allows to drop resync events on additionally watched objects.
func TypedResourceIsChanged[T client.Object](scheme *runtime.Scheme, logger logr.Logger) predicate.TypedFuncs[T] {
	log := logger.WithValues("predicate", "ResourceIsChanged")
	return predicate.TypedFuncs[T]{
		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			// Ensure we don't modify log from above.
			log := log
			if gvk, err := apiutil.GVKForObject(e.ObjectNew, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.ObjectNew))
			}
			if e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion() {
				log.WithValues("eventType", "update").V(6).Info("Resource is not changed, will not attempt to map resource")
				return false
			}
			log.WithValues("eventType", "update").V(6).Info("Resource is changed, will attempt to map resource")
			return true
		},
		CreateFunc:  func(event.TypedCreateEvent[T]) bool { return true },
		DeleteFunc:  func(event.TypedDeleteEvent[T]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[T]) bool { return true },
	}
}
