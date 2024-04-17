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
	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// ClusterCreateInfraReady returns a predicate that returns true for a create event when a cluster has Status.InfrastructureReady set as true
// it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
func ClusterCreateInfraReady(logger logr.Logger) predicate.TypedFuncs[*clusterv1.Cluster] {
	return predicate.TypedFuncs[*clusterv1.Cluster]{
		CreateFunc: func(c event.TypedCreateEvent[*clusterv1.Cluster]) bool {
			log := logger.WithValues("predicate", "ClusterCreateInfraReady", "eventType", "create")
			log = log.WithValues("Cluster", klog.KObj(c.Object))

			// Only need to trigger a reconcile if the Cluster.Status.InfrastructureReady is true
			if c.Object.Status.InfrastructureReady {
				log.V(6).Info("Cluster infrastructure is ready, allowing further processing")
				return true
			}

			log.V(4).Info("Cluster infrastructure is not ready, blocking further processing")
			return false
		},
	}
}

// ClusterCreateNotPaused returns a predicate that returns true for a create event when a cluster has Spec.Paused set as false
// it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
func ClusterCreateNotPaused(logger logr.Logger) predicate.TypedFuncs[*clusterv1.Cluster] {
	return predicate.TypedFuncs[*clusterv1.Cluster]{
		CreateFunc: func(c event.TypedCreateEvent[*clusterv1.Cluster]) bool {
			log := logger.WithValues("predicate", "ClusterCreateNotPaused", "eventType", "create")
			log = log.WithValues("Cluster", klog.KObj(c.Object))

			// Only need to trigger a reconcile if the Cluster.Spec.Paused is false
			if !c.Object.Spec.Paused {
				log.V(6).Info("Cluster is not paused, allowing further processing")
				return true
			}

			log.V(4).Info("Cluster is paused, blocking further processing")
			return false
		},
	}
}

// ClusterUpdateInfraReady returns a predicate that returns true for an update event when a cluster has Status.InfrastructureReady changed from false to true
// it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
func ClusterUpdateInfraReady(logger logr.Logger) predicate.TypedFuncs[*clusterv1.Cluster] {
	return predicate.TypedFuncs[*clusterv1.Cluster]{
		UpdateFunc: func(u event.TypedUpdateEvent[*clusterv1.Cluster]) bool {
			log := logger.WithValues("predicate", "ClusterUpdateInfraReady", "eventType", "update")
			log = log.WithValues("Cluster", klog.KObj(u.ObjectOld))

			if !u.ObjectOld.Status.InfrastructureReady && u.ObjectNew.Status.InfrastructureReady {
				log.V(6).Info("Cluster infrastructure became ready, allowing further processing")
				return true
			}

			log.V(4).Info("Cluster infrastructure did not become ready, blocking further processing")
			return false
		},
	}
}

// ClusterUpdateUnpaused returns a predicate that returns true for an update event when a cluster has Spec.Paused changed from true to false
// it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy.
func ClusterUpdateUnpaused(logger logr.Logger) predicate.TypedFuncs[*clusterv1.Cluster] {
	return predicate.TypedFuncs[*clusterv1.Cluster]{
		UpdateFunc: func(u event.TypedUpdateEvent[*clusterv1.Cluster]) bool {
			log := logger.WithValues("predicate", "ClusterUpdateUnpaused", "eventType", "update")
			log = log.WithValues("Cluster", klog.KObj(u.ObjectOld))

			if u.ObjectOld.Spec.Paused && !u.ObjectNew.Spec.Paused {
				log.V(4).Info("Cluster was unpaused, allowing further processing")
				return true
			}

			// This predicate always work in "or" with Paused predicates
			// so the logs are adjusted to not provide false negatives/verbosity al V<=5.
			log.V(6).Info("Cluster was not unpaused, blocking further processing")
			return false
		},
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
func ClusterUnpaused(logger logr.Logger) predicate.TypedPredicate[*clusterv1.Cluster] {
	log := logger.WithValues("predicate", "ClusterUnpaused")

	// Use any to ensure we process either create or update events we care about
	return predicate.Or(ClusterCreateNotPaused(log), ClusterUpdateUnpaused(log))
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
func ClusterControlPlaneInitialized(logger logr.Logger) predicate.TypedPredicate[*clusterv1.Cluster] {
	return predicate.TypedFuncs[*clusterv1.Cluster]{
		UpdateFunc: func(u event.TypedUpdateEvent[*clusterv1.Cluster]) bool {
			log := logger.WithValues("predicate", "ClusterControlPlaneInitialized", "eventType", "update")
			log = log.WithValues("Cluster", klog.KObj(u.ObjectOld))

			if !conditions.IsTrue(u.ObjectOld, clusterv1.ControlPlaneInitializedCondition) &&
				conditions.IsTrue(u.ObjectNew, clusterv1.ControlPlaneInitializedCondition) {
				log.V(6).Info("Cluster ControlPlaneInitialized was set, allow further processing")
				return true
			}

			log.V(6).Info("Cluster ControlPlaneInitialized hasn't changed, blocking further processing")
			return false
		},
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
func ClusterUnpausedAndInfrastructureReady(logger logr.Logger) predicate.TypedPredicate[*clusterv1.Cluster] {
	log := logger.WithValues("predicate", "ClusterUnpausedAndInfrastructureReady")

	// Only continue processing create events if both not paused and infrastructure is ready
	createPredicates := predicate.And(ClusterCreateNotPaused(log), ClusterCreateInfraReady(log))

	// Process update events if either Cluster is unpaused or infrastructure becomes ready
	updatePredicates := predicate.And(ClusterUpdateUnpaused(log), ClusterUpdateInfraReady(log))

	// Use any to ensure we process either create or update events we care about
	return predicate.Or(createPredicates, updatePredicates)
}

// ClusterHasTopology returns a Predicate that returns true when cluster.Spec.Topology
// is NOT nil and false otherwise.
func ClusterHasTopology(logger logr.Logger) predicate.TypedPredicate[*clusterv1.Cluster] {
	return predicate.NewTypedPredicateFuncs(processIfTopologyManaged(logger.WithValues("predicate", "ClusterHasTopology")))
}

func processIfTopologyManaged(logger logr.Logger) func(*clusterv1.Cluster) bool {
	return func(cluster *clusterv1.Cluster) bool {
		log := logger.WithValues("Cluster", klog.KObj(cluster))

		if cluster.Spec.Topology != nil {
			log.V(6).Info("Cluster has topology, allowing further processing")
			return true
		}

		log.V(6).Info("Cluster does not have topology, blocking further processing")
		return false
	}
}
