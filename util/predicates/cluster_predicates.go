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
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// ClusterCreateInfraReady returns a predicate that returns true for a create event when a cluster has Status.InfrastructureReady set as true
// it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
func ClusterCreateInfraReady(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			log := logger.WithValues("predicate", "ClusterCreateInfraReady", "eventType", "create")

			c, ok := e.Object.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.Object))
				return false
			}
			log = log.WithValues("Cluster", klog.KObj(c))

			// Only need to trigger a reconcile if the Cluster.Status.InfrastructureReady is true
			if c.Status.InfrastructureReady {
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
func ClusterCreateNotPaused(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			log := logger.WithValues("predicate", "ClusterCreateNotPaused", "eventType", "create")

			c, ok := e.Object.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.Object))
				return false
			}
			log = log.WithValues("Cluster", klog.KObj(c))

			// Only need to trigger a reconcile if the Cluster.Spec.Paused is false
			if !c.Spec.Paused {
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

// ClusterUpdateInfraReady returns a predicate that returns true for an update event when a cluster has Status.InfrastructureReady changed from false to true
// it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
func ClusterUpdateInfraReady(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "ClusterUpdateInfraReady", "eventType", "update")

			oldCluster, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}
			log = log.WithValues("Cluster", klog.KObj(oldCluster))

			newCluster := e.ObjectNew.(*clusterv1.Cluster)

			if !oldCluster.Status.InfrastructureReady && newCluster.Status.InfrastructureReady {
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
func ClusterUpdateUnpaused(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "ClusterUpdateUnpaused", "eventType", "update")

			oldCluster, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}
			log = log.WithValues("Cluster", klog.KObj(oldCluster))

			newCluster := e.ObjectNew.(*clusterv1.Cluster)

			if oldCluster.Spec.Paused && !newCluster.Spec.Paused {
				log.V(4).Info("Cluster was unpaused, allowing further processing")
				return true
			}

			// This predicate always work in "or" with Paused predicates
			// so the logs are adjusted to not provide false negatives/verbosity al V<=5.
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
//	    predicates.ClusterUnpaused(r.Log),
//	)
func ClusterUnpaused(logger logr.Logger) predicate.Funcs {
	log := logger.WithValues("predicate", "ClusterUnpaused")

	// Use any to ensure we process either create or update events we care about
	return Any(log, ClusterCreateNotPaused(log), ClusterUpdateUnpaused(log))
}

// ClusterControlPlaneInitialized returns a Predicate that returns true on Update events
// when ControlPlaneInitializedCondition on a Cluster changes to true.
// Example use:
//
//	err := controller.Watch(
//	    source.Kind(cache, &clusterv1.Cluster{}),
//	    handler.EnqueueRequestsFromMapFunc(clusterToMachines)
//	    predicates.ClusterControlPlaneInitialized(r.Log),
//	)
func ClusterControlPlaneInitialized(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "ClusterControlPlaneInitialized", "eventType", "update")

			oldCluster, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}
			log = log.WithValues("Cluster", klog.KObj(oldCluster))

			newCluster := e.ObjectNew.(*clusterv1.Cluster)

			if !conditions.IsTrue(oldCluster, clusterv1.ControlPlaneInitializedCondition) &&
				conditions.IsTrue(newCluster, clusterv1.ControlPlaneInitializedCondition) {
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

// ClusterUnpausedAndInfrastructureReady returns a Predicate that returns true on Cluster creation events where
// both Cluster.Spec.Paused is false and Cluster.Status.InfrastructureReady is true and Update events when
// either Cluster.Spec.Paused transitions to false or Cluster.Status.InfrastructureReady transitions to true.
// This implements a common requirement for some cluster-api and provider controllers (such as Machine Infrastructure
// controllers) to resume reconciliation when the Cluster is unpaused and when the infrastructure becomes ready.
// Example use:
//
//	err := controller.Watch(
//	    source.Kind(cache, &clusterv1.Cluster{}),
//	    handler.EnqueueRequestsFromMapFunc(clusterToMachines)
//	    predicates.ClusterUnpausedAndInfrastructureReady(r.Log),
//	)
func ClusterUnpausedAndInfrastructureReady(logger logr.Logger) predicate.Funcs {
	log := logger.WithValues("predicate", "ClusterUnpausedAndInfrastructureReady")

	// Only continue processing create events if both not paused and infrastructure is ready
	createPredicates := All(log, ClusterCreateNotPaused(log), ClusterCreateInfraReady(log))

	// Process update events if either Cluster is unpaused or infrastructure becomes ready
	updatePredicates := Any(log, ClusterUpdateUnpaused(log), ClusterUpdateInfraReady(log))

	// Use any to ensure we process either create or update events we care about
	return Any(log, createPredicates, updatePredicates)
}

// ClusterHasTopology returns a Predicate that returns true when cluster.Spec.Topology
// is NOT nil and false otherwise.
func ClusterHasTopology(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfTopologyManaged(logger.WithValues("predicate", "ClusterHasTopology", "eventType", "update"), e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfTopologyManaged(logger.WithValues("predicate", "ClusterHasTopology", "eventType", "create"), e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfTopologyManaged(logger.WithValues("predicate", "ClusterHasTopology", "eventType", "delete"), e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfTopologyManaged(logger.WithValues("predicate", "ClusterHasTopology", "eventType", "generic"), e.Object)
		},
	}
}

func processIfTopologyManaged(logger logr.Logger, object client.Object) bool {
	cluster, ok := object.(*clusterv1.Cluster)
	if !ok {
		logger.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", object))
		return false
	}

	log := logger.WithValues("Cluster", klog.KObj(cluster))

	if cluster.Spec.Topology != nil {
		log.V(6).Info("Cluster has topology, allowing further processing")
		return true
	}

	log.V(6).Info("Cluster does not have topology, blocking further processing")
	return false
}
