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

package kubeadmcontrolplane

import (
	"context"

	pkgerrors "github.com/pkg/errors"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg"
)

func (r *Reconciler) tryInPlaceUpdate(
	ctx context.Context,
	controlPlane *pkg.ControlPlane,
	machineToInPlaceUpdate *clusterv1.Machine,
	machineUpToDateResult pkg.UpToDateResult,
) (fallbackToScaleDown bool, _ error) {
	if r.overrideTryInPlaceUpdateFunc != nil {
		return r.overrideTryInPlaceUpdateFunc(ctx, controlPlane, machineToInPlaceUpdate, machineUpToDateResult)
	}

	// Note: Usually canUpdateMachine is only called once for a single Machine rollout.
	// If it returns true, the code below will mark the in-place update as in progress via
	// UpdateInProgressAnnotation. From this point forward we are not going to call canUpdateMachine again.
	// If it returns false, we are going to fall back to scale down which will delete the Machine.
	// We only have to repeat the canUpdateMachine call if the write call to set UpdateInProgressAnnotation
	// fails or if we fail to delete the Machine.
	canUpdate, err := r.canUpdateMachine(ctx, machineToInPlaceUpdate, machineUpToDateResult)
	if err != nil {
		return false, pkgerrors.Wrapf(err, "failed to determine if Machine %s can be updated in-place", machineToInPlaceUpdate.Name)
	}

	if !canUpdate {
		return true, nil
	}

	return false, r.triggerInPlaceUpdate(ctx, controlPlane, machineToInPlaceUpdate, machineUpToDateResult)
}
