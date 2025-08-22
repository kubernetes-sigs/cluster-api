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

// Package predicates implements predicate utilities.
package predicates

import (
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// ClusterCreateInfraProvisioned returns a predicate that returns true for a create event when the Cluster infrastructure is provisioned.
// It also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
//
// Deprecated: This predicate is deprecated and will be removed in a future version. On creation of a cluster the status will always be empty.
// Because of that the predicate would never return true for InfrastructureReady.
func ClusterCreateInfraProvisioned(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			log := logger.WithValues("predicate", "ClusterCreateInfraProvisioned", "eventType", "create")
			if gvk, err := apiutil.GVKForObject(e.Object, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.Object))
			}

			c, ok := e.Object.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.Object))
				return false
			}

			// Only need to trigger a reconcile if the Cluster infrastructure is provisioned.
			if ptr.Deref(c.Status.Initialization.InfrastructureProvisioned, false) {
				log.V(6).Info("Cluster infrastructure is ready, allowing further processing")
				return true
			}

			log.V(4).Info("Cluster infrastructure is not ready, blocking further processing")
			return false
		},
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// ClusterCreateNotPaused returns a predicate that returns true for a create event when a cluster has Spec.Paused set as false
// it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
func ClusterCreateNotPaused(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			log := logger.WithValues("predicate", "ClusterCreateNotPaused", "eventType", "create")
			if gvk, err := apiutil.GVKForObject(e.Object, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.Object))
			}

			c, ok := e.Object.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.Object))
				return false
			}

			// Only need to trigger a reconcile if the Cluster.Spec.Paused is false
			if !ptr.Deref(c.Spec.Paused, false) {
				log.V(6).Info("Cluster is not paused, allowing further processing")
				return true
			}

			log.V(4).Info("Cluster is paused, blocking further processing")
			return false
		},
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// ClusterUpdateInfraProvisioned returns a predicate that returns true for an update event when a cluster has Status.Initialization.InfrastructureProvisioned changed from false to true
// it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
func ClusterUpdateInfraProvisioned(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "ClusterUpdateInfraProvisioned", "eventType", "update")
			if gvk, err := apiutil.GVKForObject(e.ObjectOld, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.ObjectOld))
			}

			oldCluster, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}

			newCluster := e.ObjectNew.(*clusterv1.Cluster)

			if !ptr.Deref(oldCluster.Status.Initialization.InfrastructureProvisioned, false) && ptr.Deref(newCluster.Status.Initialization.InfrastructureProvisioned, false) {
				log.V(6).Info("Cluster infrastructure became ready, allowing further processing")
				return true
			}

			log.V(4).Info("Cluster infrastructure did not become ready, blocking further processing")
			return false
		},
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// ClusterUpdateUnpaused returns a predicate that returns true for an update event when a cluster has Spec.Paused changed from true to false
// it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
func ClusterUpdateUnpaused(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "ClusterUpdateUnpaused", "eventType", "update")
			if gvk, err := apiutil.GVKForObject(e.ObjectOld, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.ObjectOld))
			}

			oldCluster, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}

			newCluster := e.ObjectNew.(*clusterv1.Cluster)

			if ptr.Deref(oldCluster.Spec.Paused, false) && !ptr.Deref(newCluster.Spec.Paused, false) {
				log.V(4).Info("Cluster was unpaused, allowing further processing")
				return true
			}

			// This predicate always work in "or" with Paused predicates
			// so the logs are adjusted to not provide false negatives/verbosity at V<=5.
			log.V(6).Info("Cluster was not unpaused, blocking further processing")
			return false
		},
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// ClusterUnpaused returns a Predicate that returns true on Cluster creation events where Cluster.Spec.Paused is false
// and Update events when Cluster.Spec.Paused transitions to false.
// This implements a common requirement for many cluster-api and provider controllers (such as Cluster Infrastructure
// controllers) to resume reconciliation when the Cluster is unpaused.
// Example use:
//
//	err := controller.Watch(
//	    source.Kind(cache, &clusterv1.Cluster{}),
//	    handler.EnqueueRequestsFromMapFunc(clusterToMachines)
//	    predicates.ClusterUnpaused(mgr.GetScheme(), r.Log),
//	)
func ClusterUnpaused(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	log := logger.WithValues("predicate", "ClusterUnpaused")

	// Use any to ensure we process either create or update events we care about
	return Any(scheme, log, ClusterCreateNotPaused(scheme, log), ClusterUpdateUnpaused(scheme, log))
}

// ClusterPausedTransitions returns a predicate that returns true for an update event when a cluster has Spec.Paused changed.
func ClusterPausedTransitions(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "ClusterPausedTransitions", "eventType", "update")
			if gvk, err := apiutil.GVKForObject(e.ObjectOld, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.ObjectOld))
			}

			oldCluster, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}

			newCluster := e.ObjectNew.(*clusterv1.Cluster)

			if ptr.Deref(oldCluster.Spec.Paused, false) && !ptr.Deref(newCluster.Spec.Paused, false) {
				log.V(6).Info("Cluster unpausing, allowing further processing")
				return true
			}

			if !ptr.Deref(oldCluster.Spec.Paused, false) && ptr.Deref(newCluster.Spec.Paused, false) {
				log.V(6).Info("Cluster pausing, allowing further processing")
				return true
			}

			// This predicate always work in "or" with Paused predicates
			// so the logs are adjusted to not provide false negatives/verbosity at V<=5.
			log.V(6).Info("Cluster paused state was not changed, blocking further processing")
			return false
		},
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// ClusterControlPlaneInitialized returns a Predicate that returns true on Update events
// when ControlPlaneInitializedCondition on a Cluster changes to true.
// Example use:
//
//	err := controller.Watch(
//	    source.Kind(cache, &clusterv1.Cluster{}),
//	    handler.EnqueueRequestsFromMapFunc(clusterToMachines)
//	    predicates.ClusterControlPlaneInitialized(mgr.GetScheme(), r.Log),
//	)
func ClusterControlPlaneInitialized(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "ClusterControlPlaneInitialized", "eventType", "update")
			if gvk, err := apiutil.GVKForObject(e.ObjectOld, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.ObjectOld))
			}

			oldCluster, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}

			newCluster := e.ObjectNew.(*clusterv1.Cluster)
			if !conditions.IsTrue(oldCluster, clusterv1.ClusterControlPlaneInitializedCondition) &&
				conditions.IsTrue(newCluster, clusterv1.ClusterControlPlaneInitializedCondition) {
				log.V(6).Info("Cluster ControlPlaneInitialized was set, allow further processing")
				return true
			}

			log.V(6).Info("Cluster ControlPlaneInitialized hasn't changed, blocking further processing")
			return false
		},
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// ClusterPausedTransitionsOrInfrastructureProvisioned returns a Predicate that returns true on Cluster Update events where
// either Cluster.Spec.Paused transitions or Cluster.Status.Initialization.InfrastructureProvisioned transitions to true.
// This implements a common requirement for some cluster-api and provider controllers (such as Machine Infrastructure
// controllers) to resume reconciliation when the Cluster gets paused or unpaused and when the infrastructure becomes ready.
// Example use:
//
//	err := controller.Watch(
//	    source.Kind(cache, &clusterv1.Cluster{}),
//	    handler.EnqueueRequestsFromMapFunc(clusterToMachines)
//	    predicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), r.Log),
//	)
func ClusterPausedTransitionsOrInfrastructureProvisioned(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	log := logger.WithValues("predicate", "ClusterPausedTransitionsOrInfrastructureProvisioned")

	return Any(scheme, log, ClusterPausedTransitions(scheme, log), ClusterUpdateInfraProvisioned(scheme, log))
}

// ClusterUnpausedAndInfrastructureProvisioned returns a Predicate that returns true on Cluster creation events where
// both Cluster.Spec.Paused is false and Cluster.Status.Initialization.InfrastructureProvisioned is true and Update events when
// either Cluster.Spec.Paused transitions to false or Cluster.Status.Initialization.InfrastructureProvisioned transitions to true.
// This implements a common requirement for some cluster-api and provider controllers (such as Machine Infrastructure
// controllers) to resume reconciliation when the Cluster is unpaused and when the infrastructure becomes ready.
// Example use:
//
//	err := controller.Watch(
//	    source.Kind(cache, &clusterv1.Cluster{}),
//	    handler.EnqueueRequestsFromMapFunc(clusterToMachines)
//	    predicates.ClusterUnpausedAndInfrastructureProvisioned(mgr.GetScheme(), r.Log),
//	)
//
// Deprecated: This predicate is deprecated and will be removed in a future version,
// use ClusterPausedTransitionsOrInfrastructureProvisioned instead.
func ClusterUnpausedAndInfrastructureProvisioned(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	log := logger.WithValues("predicate", "ClusterUnpausedAndInfrastructureProvisioned")

	// Only continue processing create events if both not paused and infrastructure is ready
	createPredicates := All(scheme, log, ClusterCreateNotPaused(scheme, log), ClusterCreateInfraProvisioned(scheme, log))

	// Process update events if either Cluster is unpaused or infrastructure becomes ready
	updatePredicates := Any(scheme, log, ClusterUpdateUnpaused(scheme, log), ClusterUpdateInfraProvisioned(scheme, log))

	// Use any to ensure we process either create or update events we care about
	return Any(scheme, log, createPredicates, updatePredicates)
}

// ClusterHasTopology returns a Predicate that returns true when cluster.Spec.Topology
// is NOT nil and false otherwise.
func ClusterHasTopology(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfTopologyManaged(scheme, logger.WithValues("predicate", "ClusterHasTopology", "eventType", "update"), e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfTopologyManaged(scheme, logger.WithValues("predicate", "ClusterHasTopology", "eventType", "create"), e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfTopologyManaged(scheme, logger.WithValues("predicate", "ClusterHasTopology", "eventType", "delete"), e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfTopologyManaged(scheme, logger.WithValues("predicate", "ClusterHasTopology", "eventType", "generic"), e.Object)
		},
	}
}

func processIfTopologyManaged(scheme *runtime.Scheme, logger logr.Logger, object client.Object) bool {
	if gvk, err := apiutil.GVKForObject(object, scheme); err == nil {
		logger = logger.WithValues(gvk.Kind, klog.KObj(object))
	}
	cluster, ok := object.(*clusterv1.Cluster)
	if !ok {
		logger.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", object))
		return false
	}

	if cluster.Spec.Topology.IsDefined() {
		logger.V(6).Info("Cluster has topology, allowing further processing")
		return true
	}

	logger.V(6).Info("Cluster does not have topology, blocking further processing")
	return false
}

// ClusterTopologyVersionChanged returns a Predicate that returns true when cluster.Spec.Topology.Version
// was changed.
func ClusterTopologyVersionChanged(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			logger := logger.WithValues("predicate", "ClusterTopologyVersionChanged", "eventType", "update")
			if gvk, err := apiutil.GVKForObject(e.ObjectOld, scheme); err == nil {
				logger = logger.WithValues(gvk.Kind, klog.KObj(e.ObjectOld))
			}

			oldCluster, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				logger.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}

			newCluster := e.ObjectNew.(*clusterv1.Cluster)

			if !oldCluster.Spec.Topology.IsDefined() || !newCluster.Spec.Topology.IsDefined() {
				logger.V(6).Info("Cluster does not have topology, blocking further processing")
				return false
			}

			if oldCluster.Spec.Topology.Version != newCluster.Spec.Topology.Version {
				logger.V(6).Info("Cluster topology version has changed, allowing further processing")
				return true
			}

			logger.V(6).Info("Cluster topology version has not changed, blocking further processing")
			return false
		},
		CreateFunc: func(_ event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}
