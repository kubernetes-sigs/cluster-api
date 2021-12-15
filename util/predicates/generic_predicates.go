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
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/labels"
)

// All returns a predicate that returns true only if all given predicates return true.
func All(logger logr.Logger, predicates ...predicate.Funcs) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicateAggregation", "All")
			for _, p := range predicates {
				if !p.UpdateFunc(e) {
					log.V(6).Info("One of the provided predicates returned false, blocking further processing")
					return false
				}
			}
			log.V(6).Info("All provided predicates returned true, allowing further processing")
			return true
		},
		CreateFunc: func(e event.CreateEvent) bool {
			log := logger.WithValues("predicateAggregation", "All")
			for _, p := range predicates {
				if !p.CreateFunc(e) {
					log.V(6).Info("One of the provided predicates returned false, blocking further processing")
					return false
				}
			}
			log.V(6).Info("All provided predicates returned true, allowing further processing")
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log := logger.WithValues("predicateAggregation", "All")
			for _, p := range predicates {
				if !p.DeleteFunc(e) {
					log.V(6).Info("One of the provided predicates returned false, blocking further processing")
					return false
				}
			}
			log.V(6).Info("All provided predicates returned true, allowing further processing")
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			log := logger.WithValues("predicateAggregation", "All")
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
func Any(logger logr.Logger, predicates ...predicate.Funcs) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicateAggregation", "Any")
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
func ResourceHasFilterLabel(logger logr.Logger, labelValue string) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfLabelMatch(logger.WithValues("predicate", "ResourceHasFilterLabel", "eventType", "update"), e.ObjectNew, labelValue)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfLabelMatch(logger.WithValues("predicate", "ResourceHasFilterLabel", "eventType", "create"), e.Object, labelValue)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfLabelMatch(logger.WithValues("predicate", "ResourceHasFilterLabel", "eventType", "delete"), e.Object, labelValue)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfLabelMatch(logger.WithValues("predicate", "ResourceHasFilterLabel", "eventType", "generic"), e.Object, labelValue)
		},
	}
}

// ResourceNotPaused returns a Predicate that returns true only if the provided resource does not contain the
// paused annotation.
// This implements a common requirement for all cluster-api and provider controllers skip reconciliation when the paused
// annotation is present for a resource.
// Example use:
//	func (r *MyReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
//		controller, err := ctrl.NewControllerManagedBy(mgr).
//			For(&v1.MyType{}).
//			WithOptions(options).
//			WithEventFilter(util.ResourceNotPaused(r.Log)).
//			Build(r)
//		return err
//	}
func ResourceNotPaused(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfNotPaused(logger.WithValues("predicate", "ResourceNotPaused", "eventType", "update"), e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfNotPaused(logger.WithValues("predicate", "ResourceNotPaused", "eventType", "create"), e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfNotPaused(logger.WithValues("predicate", "ResourceNotPaused", "eventType", "delete"), e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfNotPaused(logger.WithValues("predicate", "ResourceNotPaused", "eventType", "generic"), e.Object)
		},
	}
}

// ResourceNotPausedAndHasFilterLabel returns a predicate that returns true only if the
// ResourceNotPaused and ResourceHasFilterLabel predicates return true.
func ResourceNotPausedAndHasFilterLabel(logger logr.Logger, labelValue string) predicate.Funcs {
	return All(logger, ResourceNotPaused(logger), ResourceHasFilterLabel(logger, labelValue))
}

func processIfNotPaused(logger logr.Logger, obj client.Object) bool {
	kind := strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind)
	log := logger.WithValues("namespace", obj.GetNamespace(), kind, obj.GetName())
	if annotations.HasPaused(obj) {
		log.V(4).Info("Resource is paused, will not attempt to map resource")
		return false
	}
	log.V(6).Info("Resource is not paused, will attempt to map resource")
	return true
}

func processIfLabelMatch(logger logr.Logger, obj client.Object, labelValue string) bool {
	// Return early if no labelValue was set.
	if labelValue == "" {
		return true
	}

	kind := strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind)
	log := logger.WithValues("namespace", obj.GetNamespace(), kind, obj.GetName())
	if labels.HasWatchLabel(obj, labelValue) {
		log.V(6).Info("Resource matches label, will attempt to map resource")
		return true
	}
	log.V(4).Info("Resource does not match label, will not attempt to map resource")
	return false
}

// ResourceIsNotExternallyManaged returns a predicate that returns true only if the resource does not contain
// the externally managed annotation.
// This implements a requirement for InfraCluster providers to be able to ignore externally managed
// cluster infrastructure.
func ResourceIsNotExternallyManaged(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfNotExternallyManaged(logger.WithValues("predicate", "ResourceIsNotExternallyManaged", "eventType", "update"), e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfNotExternallyManaged(logger.WithValues("predicate", "ResourceIsNotExternallyManaged", "eventType", "create"), e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfNotExternallyManaged(logger.WithValues("predicate", "ResourceIsNotExternallyManaged", "eventType", "delete"), e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfNotExternallyManaged(logger.WithValues("predicate", "ResourceIsNotExternallyManaged", "eventType", "generic"), e.Object)
		},
	}
}

func processIfNotExternallyManaged(logger logr.Logger, obj client.Object) bool {
	kind := strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind)
	log := logger.WithValues("namespace", obj.GetNamespace(), kind, obj.GetName())
	if annotations.IsExternallyManaged(obj) {
		log.V(4).Info("Resource is externally managed, will not attempt to map resource")
		return false
	}
	log.V(6).Info("Resource is managed, will attempt to map resource")
	return true
}

// ResourceIsTopologyOwned returns a predicate that returns true only if the resource has
// the `topology.cluster.x-k8s.io/owned` label.
func ResourceIsTopologyOwned(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfTopologyOwned(logger.WithValues("predicate", "ResourceIsTopologyOwned", "eventType", "update"), e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfTopologyOwned(logger.WithValues("predicate", "ResourceIsTopologyOwned", "eventType", "create"), e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfTopologyOwned(logger.WithValues("predicate", "ResourceIsTopologyOwned", "eventType", "delete"), e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfTopologyOwned(logger.WithValues("predicate", "ResourceIsTopologyOwned", "eventType", "generic"), e.Object)
		},
	}
}

func processIfTopologyOwned(logger logr.Logger, obj client.Object) bool {
	kind := strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind)
	log := logger.WithValues("namespace", obj.GetNamespace(), kind, obj.GetName())
	if labels.IsTopologyOwned(obj) {
		log.V(6).Info("Resource is topology owned, will attempt to map resource")
		return true
	}
	// We intentionally log this line only on level 6, because it will be very frequently
	// logged for MachineDeployments and MachineSets not owned by a topology.
	log.V(6).Info("Resource is not topology owned, will not attempt to map resource")
	return false
}
