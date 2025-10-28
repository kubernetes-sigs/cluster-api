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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
)

func Test_tryInPlaceUpdate(t *testing.T) {
	machineToInPlaceUpdate := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machine-to-in-place-update",
		},
	}

	tests := []struct {
		name                           string
		preflightChecksFunc            func(ctx context.Context, controlPlane *internal.ControlPlane, excludeFor ...*clusterv1.Machine) ctrl.Result
		canUpdateMachineFunc           func(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult) (bool, error)
		wantCanUpdateMachineCalled     bool
		wantTriggerInPlaceUpdateCalled bool
		wantFallbackToScaleDown        bool
		wantError                      bool
		wantErrorMessage               string
		wantRes                        ctrl.Result
	}{
		{
			name: "Requeue if preflight checks for all Machines failed",
			preflightChecksFunc: func(_ context.Context, _ *internal.ControlPlane, _ ...*clusterv1.Machine) ctrl.Result {
				return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}
			},
			wantRes: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
		},
		{
			name: "Fallback to scale down if checks for all Machines failed, but checks succeed when excluding machineToInPlaceUpdate",
			preflightChecksFunc: func(_ context.Context, _ *internal.ControlPlane, excludeFor ...*clusterv1.Machine) ctrl.Result {
				if len(excludeFor) == 1 && excludeFor[0] == machineToInPlaceUpdate {
					return ctrl.Result{} // If machineToInPlaceUpdate is excluded preflight checks succeed => scale down
				}
				return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}
			},
			wantFallbackToScaleDown: true,
		},
		{
			name: "Return error if canUpdateMachine returns an error",
			preflightChecksFunc: func(_ context.Context, _ *internal.ControlPlane, _ ...*clusterv1.Machine) ctrl.Result {
				return ctrl.Result{}
			},
			canUpdateMachineFunc: func(_ context.Context, _ *clusterv1.Machine, _ internal.UpToDateResult) (bool, error) {
				return false, errors.New("canUpdateMachine error")
			},
			wantCanUpdateMachineCalled: true,
			wantError:                  true,
			wantErrorMessage:           "failed to determine if Machine machine-to-in-place-update can be updated in-place: canUpdateMachine error",
		},
		{
			name: "Fallback to scale down if canUpdateMachine returns false",
			preflightChecksFunc: func(_ context.Context, _ *internal.ControlPlane, _ ...*clusterv1.Machine) ctrl.Result {
				return ctrl.Result{}
			},
			canUpdateMachineFunc: func(_ context.Context, _ *clusterv1.Machine, _ internal.UpToDateResult) (bool, error) {
				return false, nil
			},
			wantCanUpdateMachineCalled: true,
			wantFallbackToScaleDown:    true,
		},
		{
			name: "Trigger in-place update if canUpdateMachine returns true",
			preflightChecksFunc: func(_ context.Context, _ *internal.ControlPlane, _ ...*clusterv1.Machine) ctrl.Result {
				return ctrl.Result{}
			},
			canUpdateMachineFunc: func(_ context.Context, _ *clusterv1.Machine, _ internal.UpToDateResult) (bool, error) {
				return true, nil
			},
			wantCanUpdateMachineCalled:     true,
			wantTriggerInPlaceUpdateCalled: true,
			wantFallbackToScaleDown:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var canUpdateMachineCalled bool
			var triggerInPlaceUpdateCalled bool
			r := &KubeadmControlPlaneReconciler{
				overridePreflightChecksFunc: func(ctx context.Context, controlPlane *internal.ControlPlane, excludeFor ...*clusterv1.Machine) ctrl.Result {
					return tt.preflightChecksFunc(ctx, controlPlane, excludeFor...)
				},
				overrideCanUpdateMachineFunc: func(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult) (bool, error) {
					canUpdateMachineCalled = true
					return tt.canUpdateMachineFunc(ctx, machine, machineUpToDateResult)
				},
				overrideTriggerInPlaceUpdate: func(_ context.Context, _ *clusterv1.Machine, _ internal.UpToDateResult) error {
					triggerInPlaceUpdateCalled = true
					return nil
				},
			}

			fallbackToScaleDown, res, err := r.tryInPlaceUpdate(ctx, nil, machineToInPlaceUpdate, internal.UpToDateResult{})
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(res).To(Equal(tt.wantRes))
			g.Expect(fallbackToScaleDown).To(Equal(tt.wantFallbackToScaleDown))

			g.Expect(canUpdateMachineCalled).To(Equal(tt.wantCanUpdateMachineCalled), "canUpdateMachineCalled: actual: %t expected: %t", canUpdateMachineCalled, tt.wantCanUpdateMachineCalled)
			g.Expect(triggerInPlaceUpdateCalled).To(Equal(tt.wantTriggerInPlaceUpdateCalled), "triggerInPlaceUpdateCalled: actual: %t expected: %t", triggerInPlaceUpdateCalled, tt.wantTriggerInPlaceUpdateCalled)
		})
	}
}
