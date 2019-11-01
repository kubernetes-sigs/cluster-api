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

package controllers

import (
	"context"
	"time"

	"github.com/pkg/errors"
	apicorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrNodeNotFound = errors.New("cannot find node with matching ProviderID")
)

func (r *MachineReconciler) reconcileNodeRef(_ context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	logger := r.Log.WithValues("machine", machine.Name, "namespace", machine.Namespace)
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
		logger.V(2).Info("Machine doesn't have a linked cluster, won't assign NodeRef")
		return nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	// Check that the Machine has a valid ProviderID.
	if machine.Spec.ProviderID == nil || *machine.Spec.ProviderID == "" {
		logger.Info("Machine doesn't have a valid ProviderID yet")
		return nil
	}

	providerID, err := noderefutil.NewProviderID(*machine.Spec.ProviderID)
	if err != nil {
		return err
	}

	clusterClient, err := remote.NewClusterClient(r.Client, cluster, r.scheme)
	if err != nil {
		return err
	}

	// Get the Node reference.
	nodeRef, err := r.getNodeReference(clusterClient, providerID)
	if err != nil {
		if err == ErrNodeNotFound {
			return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 10 * time.Second},
				"cannot assign NodeRef to Machine %q in namespace %q, no matching Node", machine.Name, machine.Namespace)
		}
		logger.Error(err, "Failed to assign NodeRef")
		r.recorder.Event(machine, apicorev1.EventTypeWarning, "FailedSetNodeRef", err.Error())
		return err
	}

	// Set the Machine NodeRef.
	machine.Status.NodeRef = nodeRef
	logger.Info("Set Machine's NodeRef", "noderef", machine.Status.NodeRef.Name)
	r.recorder.Event(machine, apicorev1.EventTypeNormal, "SuccessfulSetNodeRef", machine.Status.NodeRef.Name)
	return nil
}

func (r *MachineReconciler) getNodeReference(client client.Client, providerID *noderefutil.ProviderID) (*apicorev1.ObjectReference, error) {
	logger := r.Log.WithValues("providerID", providerID)

	listOpt := metav1.ListOptions{}

	for {
		nodeList := apicorev1.NodeList{}
		// TODO Add a context to this method
		err := client.List(context.TODO(), &nodeList)
		if err != nil {
			return nil, err
		}

		for _, node := range nodeList.Items {
			nodeProviderID, err := noderefutil.NewProviderID(node.Spec.ProviderID)
			if err != nil {
				logger.Error(err, "Failed to parse ProviderID", "node", node.Name)
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
