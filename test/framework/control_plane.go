/*
Copyright 2019 The Kubernetes Authors.

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

package framework

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
)

// WaitForControlPlaneToBeUpToDateInput is the input for WaitForControlPlaneToBeUpToDate.
type WaitForControlPlaneToBeUpToDateInput struct {
	Getter       Getter
	ControlPlane *controlplanev1.KubeadmControlPlane
}

// WaitForControlPlaneToBeUpToDate will wait for a control plane to be fully up-to-date.
func WaitForControlPlaneToBeUpToDate(ctx context.Context, input WaitForControlPlaneToBeUpToDateInput, intervals ...interface{}) {
	By("Waiting for the control plane to be ready")
	Eventually(func() (int32, error) {
		controlplane := &controlplanev1.KubeadmControlPlane{}
		key := client.ObjectKey{
			Namespace: input.ControlPlane.GetNamespace(),
			Name:      input.ControlPlane.GetName(),
		}
		if err := input.Getter.Get(ctx, key, controlplane); err != nil {
			return 0, err
		}
		return controlplane.Status.UpdatedReplicas, nil
	}, intervals...).Should(Equal(*input.ControlPlane.Spec.Replicas))
}
