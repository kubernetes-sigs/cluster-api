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

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// The number of times we retry updating a MachineSet's status.
	statusUpdateRetries = 1
)

func (r *MachineSetReconciler) calculateStatus(ms *clusterv1.MachineSet, filteredMachines []*clusterv1.Machine) clusterv1.MachineSetStatus {
	logger := r.Log.WithValues("machineset", ms.Name, "namespace", ms.Namespace)

	newStatus := ms.Status

	// Count the number of machines that have labels matching the labels of the machine
	// template of the replica set, the matching machines may have more
	// labels than are in the template. Because the label of machineTemplateSpec is
	// a superset of the selector of the replica set, so the possible
	// matching machines must be part of the filteredMachines.
	fullyLabeledReplicasCount := 0
	readyReplicasCount := 0
	availableReplicasCount := 0
	templateLabel := labels.Set(ms.Spec.Template.Labels).AsSelectorPreValidated()

	// Retrieve Cluster, if any.
	cluster, _ := util.GetClusterByName(context.Background(), r.Client, ms.ObjectMeta.Namespace, ms.Spec.ClusterName)

	for _, machine := range filteredMachines {
		if templateLabel.Matches(labels.Set(machine.Labels)) {
			fullyLabeledReplicasCount++
		}

		if machine.Status.NodeRef == nil {
			logger.Info("Unable to retrieve Node status,missing NodeRef", "machine", machine.Name)
			continue
		}

		node, err := r.getMachineNode(cluster, machine)
		if err != nil {
			logger.Error(err, "Unable to retrieve Node status")
			continue
		}

		if noderefutil.IsNodeReady(node) {
			readyReplicasCount++
			if noderefutil.IsNodeAvailable(node, ms.Spec.MinReadySeconds, metav1.Now()) {
				availableReplicasCount++
			}
		}
	}

	newStatus.Replicas = int32(len(filteredMachines))
	newStatus.FullyLabeledReplicas = int32(fullyLabeledReplicasCount)
	newStatus.ReadyReplicas = int32(readyReplicasCount)
	newStatus.AvailableReplicas = int32(availableReplicasCount)
	return newStatus
}

// updateMachineSetStatus attempts to update the Status.Replicas of the given MachineSet, with a single GET/PUT retry.
func updateMachineSetStatus(c client.Client, ms *clusterv1.MachineSet, newStatus clusterv1.MachineSetStatus, logger logr.Logger) (*clusterv1.MachineSet, error) {
	// This is the steady state. It happens when the MachineSet doesn't have any expectations, since
	// we do a periodic relist every 10 minutes. If the generations differ but the replicas are
	// the same, a caller might've resized to the same replica count.
	if ms.Status.Replicas == newStatus.Replicas &&
		ms.Status.FullyLabeledReplicas == newStatus.FullyLabeledReplicas &&
		ms.Status.ReadyReplicas == newStatus.ReadyReplicas &&
		ms.Status.AvailableReplicas == newStatus.AvailableReplicas &&
		ms.Generation == ms.Status.ObservedGeneration {
		return ms, nil
	}

	// Save the generation number we acted on, otherwise we might wrongfully indicate
	// that we've seen a spec update when we retry.
	// TODO: This can clobber an update if we allow multiple agents to write to the
	// same status.
	newStatus.ObservedGeneration = ms.Generation

	var getErr, updateErr error
	for i := 0; ; i++ {
		var replicas int32
		if ms.Spec.Replicas != nil {
			replicas = *ms.Spec.Replicas
		}
		logger.V(4).Info(fmt.Sprintf("Updating status for %v: %s/%s, ", ms.Kind, ms.Namespace, ms.Name) +
			fmt.Sprintf("replicas %d->%d (need %d), ", ms.Status.Replicas, newStatus.Replicas, replicas) +
			fmt.Sprintf("fullyLabeledReplicas %d->%d, ", ms.Status.FullyLabeledReplicas, newStatus.FullyLabeledReplicas) +
			fmt.Sprintf("readyReplicas %d->%d, ", ms.Status.ReadyReplicas, newStatus.ReadyReplicas) +
			fmt.Sprintf("availableReplicas %d->%d, ", ms.Status.AvailableReplicas, newStatus.AvailableReplicas) +
			fmt.Sprintf("sequence No: %v->%v", ms.Status.ObservedGeneration, newStatus.ObservedGeneration))

		ms.Status = newStatus
		updateErr = c.Status().Update(context.Background(), ms)
		if updateErr == nil {
			return ms, nil
		}
		// Stop retrying if we exceed statusUpdateRetries - the machineSet will be requeued with a rate limit.
		if i >= statusUpdateRetries {
			break
		}
		// Update the MachineSet with the latest resource version for the next poll
		if getErr = c.Get(context.Background(), client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}, ms); getErr != nil {
			// If the GET fails we can't trust status.Replicas anymore. This error
			// is bound to be more interesting than the update failure.
			return nil, getErr
		}
	}

	return nil, updateErr
}

func (r *MachineSetReconciler) getMachineNode(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (*corev1.Node, error) {
	c, err := remote.NewClusterClient(r.Client, cluster, r.scheme)
	if err != nil {
		return nil, err
	}
	node := &corev1.Node{}
	err = c.Get(context.TODO(), client.ObjectKey{Name: machine.Status.NodeRef.Name}, node)
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving node %s for machine %s/%s", machine.Status.NodeRef.Name, machine.Namespace, machine.Name)
	}
	return node, nil
}
