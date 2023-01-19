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
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
)

const (
	// MachineNodeNameField is used by the Machine Controller to index Machines by Node name, and add a watch on Nodes.
	MachineNodeNameField = "status.nodeRef.name"

	// MachineProviderIDField is used to index Machines by ProviderID. It's useful to find Machines
	// in a management cluster from Nodes in a workload cluster.
	MachineProviderIDField = "spec.providerID"
)

// ByMachineNode adds the machine node name index to the
// managers cache.
func ByMachineNode(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetCache().IndexField(ctx, &clusterv1.Machine{},
		MachineNodeNameField,
		MachineByNodeName,
	); err != nil {
		return errors.Wrap(err, "error setting index field")
	}

	return nil
}

// MachineByNodeName contains the logic to index Machines by Node name.
func MachineByNodeName(o client.Object) []string {
	machine, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine but got a %T", o))
	}
	if machine.Status.NodeRef != nil {
		return []string{machine.Status.NodeRef.Name}
	}
	return nil
}

// ByMachineProviderID adds the machine providerID index to the
// managers cache.
func ByMachineProviderID(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetCache().IndexField(ctx, &clusterv1.Machine{},
		MachineProviderIDField,
		machineByProviderID,
	); err != nil {
		return errors.Wrap(err, "error setting index field")
	}

	return nil
}

func machineByProviderID(o client.Object) []string {
	machine, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine but got a %T", o))
	}

	if pointer.StringDeref(machine.Spec.ProviderID, "") == "" {
		return nil
	}

	providerID, err := noderefutil.NewProviderID(*machine.Spec.ProviderID)
	if err != nil {
		// Failed to create providerID, skipping.
		return nil
	}
	return []string{providerID.IndexKey()}
}
