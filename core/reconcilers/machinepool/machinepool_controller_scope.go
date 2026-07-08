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

	// infraMachinePoolIsNotFound is true if getting the infra machinepool object failed with a NotFound error.
	infraMachinePoolIsNotFound bool

	// bootstrapConfig is the bootstrap config object referenced by the MachinePool.
	// It is set during the reconcile bootstrap phase.
	bootstrapConfig *unstructured.Unstructured

	// bootstrapConfigIsNotFound is true if getting the bootstrap config object failed with a NotFound error.
	bootstrapConfigIsNotFound bool

	// nodeRefMap is a map of providerIDs to Nodes that are associated with the Cluster.
	// It is set after reconcileInfrastructure is called.
	nodeRefMap map[string]*corev1.Node

	// nodeRefMapObserved is true if Nodes were successfully listed during this reconcile.
	nodeRefMapObserved bool

	// infrastructureProviderIDListObserved is true if spec.providerIDList was successfully read from the
	// infrastructure MachinePool during this reconcile.
	infrastructureProviderIDListObserved bool

	// infrastructureReplicasObserved is true if status.replicas was successfully read from the
	// infrastructure MachinePool during this reconcile.
	infrastructureReplicasObserved bool

	// machines holds a list of the machines associated with this machine pool.
	machines []*clusterv1.Machine

	// getMachinesForMachinePoolSucceeded is true if the machines associated with this machine pool
	// were successfully listed. It is used to avoid surfacing misleading aggregate conditions when
	// the machines could not be read.
	getMachinesForMachinePoolSucceeded bool
}

type machinePoolMachinesState int

const (
	machinePoolMachinesStateUnknown machinePoolMachinesState = iota
	machinePoolMachinesStateNotSupported
	machinePoolMachinesStateSupported
)

func (s *scope) machinePoolMachinesState() (machinePoolMachinesState, error) {
	if s.infraMachinePool == nil {
		return machinePoolMachinesStateUnknown, errors.New("infra machine pool not set on scope")
	}

	machineKind, found, err := unstructured.NestedString(s.infraMachinePool.Object, "status", "infrastructureMachineKind")
	if err != nil {
		return machinePoolMachinesStateUnknown, fmt.Errorf("failed to lookup infrastructureMachineKind: %w", err)
	}

	if !found || machineKind == "" {
		return machinePoolMachinesStateNotSupported, nil
	}

	return machinePoolMachinesStateSupported, nil
}
