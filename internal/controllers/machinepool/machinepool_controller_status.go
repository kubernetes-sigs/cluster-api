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

package machinepool

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	internalversion "sigs.k8s.io/cluster-api/internal/util/version"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func (r *Reconciler) updateStatus(ctx context.Context, s *scope) error {
	log := ctrl.LoggerFrom(ctx)

	if s.infraMachinePool == nil {
		log.V(4).Info("infra machine pool isn't set, skipping setting status")
		return nil
	}
	hasMachinePoolMachines, err := s.hasMachinePoolMachines()
	if err != nil {
		return fmt.Errorf("determining if there are machine pool machines: %w", err)
	}

	setReplicas(s.machinePool, hasMachinePoolMachines, s.machines, s.nodeRefMap)

	// TODO: in future add setting conditions here

	return nil
}

func setReplicas(mp *clusterv1.MachinePool, hasMachinePoolMachines bool, machines []*clusterv1.Machine, nodeRefMap map[string]*corev1.Node) {
	if !hasMachinePoolMachines {
		// If we don't have machinepool machine then calculate the values differently
		mp.Status.ReadyReplicas = mp.Status.Replicas
		mp.Status.AvailableReplicas = mp.Status.Replicas
		mp.Status.UpToDateReplicas = mp.Spec.Replicas
		mp.Status.Versions = versionsFromNodeRefs(mp.Status.NodeRefs, nodeRefMap)

		return
	}

	var readyReplicas, availableReplicas, upToDateReplicas int32
	for _, machine := range machines {
		if conditions.IsTrue(machine, clusterv1.MachineReadyCondition) {
			readyReplicas++
		}
		if conditions.IsTrue(machine, clusterv1.MachineAvailableCondition) {
			availableReplicas++
		}
		if conditions.IsTrue(machine, clusterv1.MachineUpToDateCondition) {
			upToDateReplicas++
		}
	}

	mp.Status.ReadyReplicas = ptr.To(readyReplicas)
	mp.Status.AvailableReplicas = ptr.To(availableReplicas)
	mp.Status.UpToDateReplicas = ptr.To(upToDateReplicas)
	mp.Status.Versions = internalversion.VersionsFromMachines(machines)
}

func versionsFromNodeRefs(nodeRefs []corev1.ObjectReference, nodeRefMap map[string]*corev1.Node) []clusterv1.StatusVersion {
	if len(nodeRefs) == 0 || len(nodeRefMap) == 0 {
		return nil
	}

	nodesByName := map[string]*corev1.Node{}
	for _, node := range nodeRefMap {
		if node == nil {
			continue
		}
		nodesByName[node.Name] = node
	}

	versions := []clusterv1.StatusVersion{}
	for _, nodeRef := range nodeRefs {
		node := nodesByName[nodeRef.Name]
		if node == nil || node.Status.NodeInfo.KubeletVersion == "" {
			continue
		}
		versions = append(versions, clusterv1.StatusVersion{
			Version:  node.Status.NodeInfo.KubeletVersion,
			Replicas: 1,
		})
	}

	return internalversion.AggregateStatusVersions(versions)
}
