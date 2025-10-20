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
	"context"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
)

func (r *KubeadmControlPlaneReconciler) tryInPlaceUpdate(
	ctx context.Context,
	controlPlane *internal.ControlPlane,
	machineToInPlaceUpdate *clusterv1.Machine,
	machineUpToDateResult internal.UpToDateResult,
) (fallbackToScaleDown bool, _ ctrl.Result, _ error) {
	if r.overrideTryInPlaceUpdateFunc != nil {
		return r.overrideTryInPlaceUpdateFunc(ctx, controlPlane, machineToInPlaceUpdate, machineUpToDateResult)
	}

	// Run preflight checks to ensure that the control plane is stable before proceeding with in-place update operation.
	if resultForAllMachines := r.preflightChecks(ctx, controlPlane); !resultForAllMachines.IsZero() {
		// If the control plane is not stable, check if the issues are only for machineToInPlaceUpdate.
		if result := r.preflightChecks(ctx, controlPlane, machineToInPlaceUpdate); result.IsZero() {
			// The issues are only for machineToInPlaceUpdate, fallback to scale down.
			// Note: The consequence of this is that a Machine with issues is scaled down and not in-place updated.
			return true, ctrl.Result{}, nil
		}

		return false, resultForAllMachines, nil
	}

	canUpdate, err := r.canUpdateMachine(ctx, machineToInPlaceUpdate, machineUpToDateResult)
	if err != nil {
		return false, ctrl.Result{}, errors.Wrapf(err, "failed to determine if Machine %s can be updated in-place", machineToInPlaceUpdate.Name)
	}

	if !canUpdate {
		return true, ctrl.Result{}, nil
	}

	// Always fallback to scale down until triggering in-place updates is implemented.
	return true, ctrl.Result{}, nil
}
