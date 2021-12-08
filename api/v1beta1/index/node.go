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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/controllers/noderefutil"
)

const (
	// NodeProviderIDField is used to index Nodes by ProviderID. Remote caches use this to find Nodes in a workload cluster
	// out of management cluster machine.spec.providerID.
	NodeProviderIDField = "spec.providerID"
)

// NodeByProviderID contains the logic to index Nodes by ProviderID.
func NodeByProviderID(o client.Object) []string {
	node, ok := o.(*corev1.Node)
	if !ok {
		panic(fmt.Sprintf("Expected a Node but got a %T", o))
	}

	if node.Spec.ProviderID == "" {
		return nil
	}

	providerID, err := noderefutil.NewProviderID(node.Spec.ProviderID)
	if err != nil {
		// Failed to create providerID, skipping.
		return nil
	}
	return []string{providerID.IndexKey()}
}
