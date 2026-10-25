/*
Copyright 2026 The Kubernetes Authors.

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

package backends

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
)

// DevMachinePoolBackendReconciler defines reconciler behaviour for a DevMachinePool backend.
type DevMachinePoolBackendReconciler interface {
	ReconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool) (ctrl.Result, error)
	ReconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool, devMachinePool *infrav1.DevMachinePool) (ctrl.Result, error)
	PatchDevMachinePool(ctx context.Context, patchHelper *patch.Helper, devMachinePool *infrav1.DevMachinePool) error
}
