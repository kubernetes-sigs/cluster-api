/*
Copyright 2020 The Kubernetes Authors.

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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *MachineReconciler) reconcileInterruptibleNodeLabel(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (ctrl.Result, error) {
	// Check that the Machine hasn't been deleted or in the process
	// and that the Machine has a NodeRef.
	if !machine.DeletionTimestamp.IsZero() || machine.Status.NodeRef == nil {
		return ctrl.Result{}, nil
	}

	// Get the infrastructure object
	infra, err := external.Get(ctx, r.Client, &machine.Spec.InfrastructureRef, machine.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	log := ctrl.LoggerFrom(ctx)

	// Get interruptible instance status from the infrastructure provider.
	interruptible, _, err := unstructured.NestedBool(infra.Object, "status", "interruptible")
	if err != nil {
		log.V(1).Error(err, "Failed to get interruptible status from infrastructure provider", "machinename", machine.Name)
		return ctrl.Result{}, nil
	}
	if !interruptible {
		return ctrl.Result{}, nil
	}

	remoteClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.setInterruptibleNodeLabel(ctx, remoteClient, machine.Status.NodeRef.Name); err != nil {
		return ctrl.Result{}, err
	}

	log.V(3).Info("Set interruptible label to Machine's Node", "nodename", machine.Status.NodeRef.Name)
	r.recorder.Event(machine, corev1.EventTypeNormal, "SuccessfulSetInterruptibleNodeLabel", machine.Status.NodeRef.Name)

	return ctrl.Result{}, nil
}

func (r *MachineReconciler) setInterruptibleNodeLabel(ctx context.Context, remoteClient client.Client, nodeName string) error {
	node := &corev1.Node{}
	if err := remoteClient.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return err
	}

	if node.Labels == nil {
		node.Labels = map[string]string{}
	}

	if _, ok := node.Labels[clusterv1.InterruptibleLabel]; ok {
		return nil
	}

	patchHelper, err := patch.NewHelper(node, r.Client)
	if err != nil {
		return err
	}

	node.Labels[clusterv1.InterruptibleLabel] = ""

	return patchHelper.Patch(ctx, node)
}
