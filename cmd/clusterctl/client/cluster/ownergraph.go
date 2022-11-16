package cluster

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OwnerGraph map[string]OwnerGraphNode

type OwnerGraphNode struct {
	Object corev1.ObjectReference
	Owners []metav1.OwnerReference
}

func nodeToOwnerRef(n *node, attributes *ownerReferenceAttributes) metav1.OwnerReference {
	ref := metav1.OwnerReference{
		Name:       n.identity.Name,
		APIVersion: n.identity.APIVersion,
		Kind:       n.identity.Kind,
		UID:        n.identity.UID,
	}
	if attributes != nil {
		if attributes.BlockOwnerDeletion != nil {
			ref.BlockOwnerDeletion = attributes.BlockOwnerDeletion
		}
		if attributes.Controller != nil {
			ref.Controller = attributes.Controller
		}
	}
	return ref
}

func GetOwnerGraph(namespace string) (OwnerGraph, error) {
	p := newProxy(Kubeconfig{Path: "", Context: ""})
	invClient := newInventoryClient(p, nil)

	graph := newObjectGraph(p, invClient)

	// Gets all the types defined by the CRDs installed by clusterctl plus the ConfigMap/Secret core types.
	err := graph.getDiscoveryTypes()
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
		n := OwnerGraphNode{Object: v.identity, Owners: []metav1.OwnerReference{}}
		for owner, attributes := range v.owners {
			n.Owners = append(n.Owners, nodeToOwnerRef(owner, &attributes))
		}
		owners[string(v.identity.UID)] = n
	}
	return owners, nil
}
