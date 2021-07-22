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

package noderefutil

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AddMachineNodeIndex adds the machine node name index to the
// managers cache.
// Deprecated: use api/v1alpha4/index.ByMachineNode instead.
func AddMachineNodeIndex(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetCache().IndexField(ctx, &clusterv1.Machine{},
		clusterv1.MachineNodeNameIndex,
		indexMachineByNodeName,
	); err != nil {
		return errors.Wrap(err, "error setting index field")
	}

	return nil
}

func indexMachineByNodeName(o client.Object) []string {
	machine, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine but got a %T", o))
	}
	if machine.Status.NodeRef != nil {
		return []string{machine.Status.NodeRef.Name}
	}
	return nil
}
