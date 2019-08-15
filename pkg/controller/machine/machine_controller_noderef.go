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

package machine

import (
	"context"
	"errors"
	"time"

	apicorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

var (
	ErrNodeNotFound = errors.New("cannot find node with matching ProviderID")
)

func (r *ReconcileMachine) reconcileNodeRef(ctx context.Context, cluster *v1alpha2.Cluster, machine *v1alpha2.Machine) error {
	// Check that the Machine hasn't been deleted or in the process.
	if !machine.DeletionTimestamp.IsZero() {
		return nil
	}

	// Check that the Machine doesn't already have a NodeRef.
	if machine.Status.NodeRef != nil {
		return nil
	}

	// Check that Cluster isn't nil.
	if cluster == nil {
		klog.Warningf("Machine %q in namespace %q doesn't have a linked cluster, won't assign NodeRef", machine.Name, machine.Namespace)
		return nil
	}

	// Check that the Machine has a valid ProviderID.
	if machine.Spec.ProviderID == nil || *machine.Spec.ProviderID == "" {
		klog.Warningf("Machine %q in namespace %q doesn't have a valid ProviderID, retrying later", machine.Name, machine.Namespace)
		return &capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second}
	}

	providerID, err := noderefutil.NewProviderID(*machine.Spec.ProviderID)
	if err != nil {
		return err
	}

	clusterClient, err := remote.NewClusterClient(r.Client, cluster)
	if err != nil {
		return err
	}

	corev1Client, err := clusterClient.CoreV1()
	if err != nil {
		return err
	}

	// Get the Node reference.
	nodeRef, err := r.getNodeReference(corev1Client, providerID)
	if err != nil {
		if err == ErrNodeNotFound {
			klog.V(2).Infof("Cannot assign NodeRef to Machine %q in namespace %q: cannot find a matching Node, retrying later",
				machine.Name, machine.Namespace)
			return &capierrors.RequeueAfterError{RequeueAfter: 10 * time.Second}
		}
		klog.Errorf("Failed to assign NodeRef to Machine %q in namespace %q: %v", machine.Name, machine.Namespace, err)
		r.recorder.Event(machine, apicorev1.EventTypeWarning, "FailedSetNodeRef", err.Error())
		return err
	}

	// Set the Machine NodeRef.
	machine.Status.NodeRef = nodeRef
	klog.Infof("Set Machine's (%q in namespace %q) NodeRef to %q", machine.Name, machine.Namespace, machine.Status.NodeRef.Name)
	r.recorder.Event(machine, apicorev1.EventTypeNormal, "SuccessfulSetNodeRef", machine.Status.NodeRef.Name)
	return nil
}

func (r *ReconcileMachine) getNodeReference(client corev1.NodesGetter, providerID *noderefutil.ProviderID) (*apicorev1.ObjectReference, error) {
	listOpt := metav1.ListOptions{}

	for {
		nodeList, err := client.Nodes().List(listOpt)
		if err != nil {
			return nil, err
		}

		for _, node := range nodeList.Items {
			nodeProviderID, err := noderefutil.NewProviderID(node.Spec.ProviderID)
			if err != nil {
				klog.V(3).Infof("Failed to parse ProviderID for Node %q: %v", node.Name, err)
				continue
			}

			if providerID.Equals(nodeProviderID) {
				return &apicorev1.ObjectReference{
					Kind:       node.Kind,
					APIVersion: node.APIVersion,
					Name:       node.Name,
					UID:        node.UID,
				}, nil
			}
		}

		listOpt.Continue = nodeList.Continue
		if listOpt.Continue == "" {
			break
		}
	}

	return nil, ErrNodeNotFound
}
