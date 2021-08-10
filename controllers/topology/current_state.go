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

package topology

import (
	"context"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

// Gets the current state of the Cluster topology.
func (r *ClusterReconciler) getCurrentState(ctx context.Context, cluster *clusterv1.Cluster) (*clusterTopologyState, error) {
	// TODO: add get class logic; also remove nolint exception from clusterTopologyState and machineDeploymentTopologyState
	return nil, nil
}
