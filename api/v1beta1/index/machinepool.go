/*
Copyright 2021 The Kubernetes Authors.

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

package index

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

const (
	// MachinePoolNodeNameField is used by the MachinePool Controller to index MachinePools by Node name, and add a watch on Nodes.
	MachinePoolNodeNameField = "status.nodeRefs.name"

	// MachinePoolProviderIDField is used to index MachinePools by ProviderID. It's useful to find MachinePools
	// in a management cluster from Nodes in a workload cluster.
	MachinePoolProviderIDField = "spec.providerIDList"
)

// ByMachinePoolNode adds the machinepool node name index to the
// managers cache.
func ByMachinePoolNode(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetCache().IndexField(ctx, &expv1.MachinePool{},
		MachinePoolNodeNameField,
		MachinePoolByNodeName,
	); err != nil {
		return errors.Wrap(err, "error setting index field")
	}

	return nil
}

// MachinePoolByNodeName contains the logic to index MachinePools by Node name.
func MachinePoolByNodeName(o client.Object) []string {
	machinepool, ok := o.(*expv1.MachinePool)
	if !ok {
		panic(fmt.Sprintf("Expected a MachinePool but got a %T", o))
	}

	if len(machinepool.Status.NodeRefs) == 0 {
		return nil
	}

	nodeNames := make([]string, 0, len(machinepool.Status.NodeRefs))
	for _, ref := range machinepool.Status.NodeRefs {
		nodeNames = append(nodeNames, ref.Name)
	}
	return nodeNames
}

// ByMachinePoolProviderID adds the machinepool providerID index to the
// managers cache.
func ByMachinePoolProviderID(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetCache().IndexField(ctx, &expv1.MachinePool{},
		MachinePoolProviderIDField,
		machinePoolByProviderID,
	); err != nil {
		return errors.Wrap(err, "error setting index field")
	}

	return nil
}

func machinePoolByProviderID(o client.Object) []string {
	machinepool, ok := o.(*expv1.MachinePool)
	if !ok {
		panic(fmt.Sprintf("Expected a MachinePool but got a %T", o))
	}

	if len(machinepool.Spec.ProviderIDList) == 0 {
		return nil
	}

	providerIDs := make([]string, 0, len(machinepool.Spec.ProviderIDList))
	for _, id := range machinepool.Spec.ProviderIDList {
		if id == "" {
			// Valid providerID not found, skipping.
			continue
		}
		providerIDs = append(providerIDs, id)
	}

	return providerIDs
}
