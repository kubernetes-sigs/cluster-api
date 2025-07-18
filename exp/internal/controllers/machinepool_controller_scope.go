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

package controllers

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// scope holds the different objects that are read and used during the reconcile.
type scope struct {
	// cluster is the Cluster object the Machine belongs to.
	// It is set at the beginning of the reconcile function.
	cluster *clusterv1.Cluster

	// machinePool is the MachinePool object. It is set at the beginning
	// of the reconcile function.
	machinePool *clusterv1.MachinePool

	// infraMachinePool is the infrastructure machinepool object. It is set during
	// the reconcile infrastructure phase.
	infraMachinePool *unstructured.Unstructured

	// nodeRefMapResult is a map of providerIDs to Nodes that are associated with the Cluster.
	// It is set after reconcileInfrastructure is called.
	nodeRefMap map[string]*corev1.Node

	// machines holds a list of the machines associated with this machine pool.
	machines []*clusterv1.Machine
}

func (s *scope) hasMachinePoolMachines() (bool, error) {
	if s.infraMachinePool == nil {
		return false, errors.New("infra machine pool not set on scope")
	}

	machineKind, found, err := unstructured.NestedString(s.infraMachinePool.Object, "status", "infrastructureMachineKind")
	if err != nil {
		return false, fmt.Errorf("failed to lookup infrastructureMachineKind: %w", err)
	}

	return found && (machineKind != ""), nil
}
