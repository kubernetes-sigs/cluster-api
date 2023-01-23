/*
Copyright 2023 The Kubernetes Authors.

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

package cluster

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// OwnerGraph contains a graph with all the objects considered by clusterctl move as nodes and the OwnerReference relationship
// between those objects as edges.
type OwnerGraph map[string]OwnerGraphNode

// OwnerGraphNode is a single node linking an ObjectReference to its OwnerReferences.
type OwnerGraphNode struct {
	Object corev1.ObjectReference
	Owners []metav1.OwnerReference
}

func nodeToOwnerRef(n *node, attributes ownerReferenceAttributes) metav1.OwnerReference {
	ref := metav1.OwnerReference{
		Name:       n.identity.Name,
		APIVersion: n.identity.APIVersion,
		Kind:       n.identity.Kind,
		UID:        n.identity.UID,
	}
	if attributes.BlockOwnerDeletion != nil {
		ref.BlockOwnerDeletion = attributes.BlockOwnerDeletion
	}
	if attributes.Controller != nil {
		ref.Controller = attributes.Controller
	}
	return ref
}

// GetOwnerGraph returns a graph with all the objects considered by clusterctl move as nodes and the OwnerReference relationship between those objects as edges.
// NOTE: this data structure is exposed to allow implementation of E2E tests verifying that CAPI can properly rebuild its
// own owner references; there is no guarantee about the stability of this API. Using this test with providers may require
// a custom implementation of this function, or the OwnerGraph it returns.
func GetOwnerGraph(namespace, kubeconfigPath string) (OwnerGraph, error) {
	p := newProxy(Kubeconfig{Path: kubeconfigPath, Context: ""})
	invClient := newInventoryClient(p, nil)

	graph := newObjectGraph(p, invClient)

	cl, err := p.NewClient()
	if err != nil {
		return OwnerGraph{}, errors.Wrap(err, "failed to create client for ownerGraph")
	}
	// Gets all the types defined by the CRDs installed by clusterctl plus the ConfigMap/Secret core types.
	err = graph.getDiscoveryTypes()
	if err != nil {
		return OwnerGraph{}, errors.Wrap(err, "failed to retrieve discovery types")
	}

	// Discovery the object graph for the selected types:
	// - Nodes are defined the Kubernetes objects (Clusters, Machines etc.) identified during the discovery process.
	// - Edges are derived by the OwnerReferences between nodes.
	if err := graph.Discovery(namespace); err != nil {
		return OwnerGraph{}, errors.Wrap(err, "failed to discover the object graph")
	}
	owners := OwnerGraph{}
	for _, v := range graph.uidToNode {
		// The Discovery function returns all secrets in the Cluster namespace. Ensure a Secret that is not part of the
		// Cluster is not added to the OwnerGraph.
		if v.identity.Kind == "Secret" {
			clusterSecret, err := isClusterSecret(v.identity, cl)
			if err != nil {
				return OwnerGraph{}, err
			}
			if !clusterSecret {
				continue
			}
		}
		n := OwnerGraphNode{Object: v.identity, Owners: []metav1.OwnerReference{}}
		for owner, attributes := range v.owners {
			n.Owners = append(n.Owners, nodeToOwnerRef(owner, attributes))
		}
		owners[string(v.identity.UID)] = n
	}
	return owners, nil
}

// isClusterSecret checks whether a Secret is related to a CAPI Cluster by checking if the secret type is ClusterSecretType.
func isClusterSecret(ref corev1.ObjectReference, c client.Client) (bool, error) {
	s := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, s); err != nil {
		return false, err
	}
	return s.Type == clusterv1.ClusterSecretType, nil
}
