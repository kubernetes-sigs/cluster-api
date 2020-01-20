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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// healthCheckTarget contains the information required to perform a health check
// on the node to determine if any remediation is required.
type healthCheckTarget struct {
	Machine *clusterv1.Machine
	Node    *corev1.Node
	MHC     *clusterv1.MachineHealthCheck
}

// getTargetsFromMHC uses the MachineHealthCheck's selector to fetch machines
// and their nodes targeted by the health check, ready for health checking.
func (r *MachineHealthCheckReconciler) getTargetsFromMHC(clusterClient client.Client, cluster *clusterv1.Cluster, mhc *clusterv1.MachineHealthCheck) ([]healthCheckTarget, error) {
	machines, err := r.getMachinesFromMHC(mhc)
	if err != nil {
		return nil, errors.Wrap(err, "error getting machines from MachineHealthCheck")
	}
	if len(machines) == 0 {
		return nil, nil
	}

	targets := []healthCheckTarget{}
	for k := range machines {
		target := healthCheckTarget{
			MHC:     mhc,
			Machine: &machines[k],
		}
		node, err := r.getNodeFromMachine(clusterClient, target.Machine)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, errors.Wrap(err, "error getting node")
			}
			// a node with only a name represents a
			// not found node in the target
			node.Name = machines[k].Status.NodeRef.Name
		}
		target.Node = node
		targets = append(targets, target)
	}
	return targets, nil
}

//getMachinesFromMHC fetches Machines matched by the MachineHealthCheck's
// label selector
func (r *MachineHealthCheckReconciler) getMachinesFromMHC(mhc *clusterv1.MachineHealthCheck) ([]clusterv1.Machine, error) {
	selector, err := metav1.LabelSelectorAsSelector(&mhc.Spec.Selector)
	if err != nil {
		return nil, errors.New("failed to build selector")
	}

	options := client.ListOptions{
		LabelSelector: selector,
		Namespace:     mhc.GetNamespace(),
	}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(context.Background(), machineList, &options); err != nil {
		return nil, errors.Wrap(err, "failed to list machines")
	}
	return machineList.Items, nil
}

// getNodeFromMachine fetches the node from a local or remote cluster for a
// given machine.
func (r *MachineHealthCheckReconciler) getNodeFromMachine(clusterClient client.Client, machine *clusterv1.Machine) (*corev1.Node, error) {
	if machine.Status.NodeRef == nil {
		return nil, nil
	}

	node := &corev1.Node{}
	nodeKey := types.NamespacedName{
		Namespace: machine.Status.NodeRef.Namespace,
		Name:      machine.Status.NodeRef.Name,
	}
	err := clusterClient.Get(context.TODO(), nodeKey, node)
	return node, err
}
