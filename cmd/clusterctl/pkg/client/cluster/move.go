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

package cluster

import (
	"fmt"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// objectReference defines a reference to a Kubernetes object that is visited during the discovery phase for the move operation.
type objectReference struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	UID        types.UID
}

func (s objectReference) String() string {
	return fmt.Sprintf("[%s/%s, namespace: %s, name: %s, uid: %s]", s.APIVersion, s.Kind, s.Namespace, s.Name, s.UID)
}

type empty struct{}

// node defines a node in the Kubernetes object graph that is visited during the discovery phase for the move operation.
type node struct {
	identity objectReference

	// dependents contains the list of nodes that are owned by the current node.
	// Nb. This is the reverse of metadata.OwnerRef.
	dependents map[*node]empty

	// virtual records if this node was discovered indirectly, e.g. by processing an OwnerRef, but not yet observed as a concrete object.
	virtual bool
}

// markObserved marks the fact that a node was observed as a concrete object.
func (n *node) markObserved() {
	n.virtual = false
}

func (n *node) addDependent(dependent *node) {
	n.dependents[dependent] = empty{}
}

// objectGraph manages the Kubernetes object graph that is generated during the discovery phase for the move operation.
type objectGraph struct {
	proxy     Proxy
	uidToNode map[types.UID]*node
}

func newObjectGraph(proxy Proxy) *objectGraph {
	return &objectGraph{
		proxy:     proxy,
		uidToNode: map[types.UID]*node{},
	}
}

// addObj adds a Kubernetes object to the object graph that is generated during the move discovery phase.
// During add, OwnerReferences are processed in order to create the dependency graph.
func (o *objectGraph) addObj(obj *unstructured.Unstructured) {
	// adds the node to the Graph
	newNode := o.objToNode(obj)

	// process OwnerReferences; if the owner object already exists, update the list of the owner object's dependents; otherwise
	// create a virtual node as a placeholder for the owner objects.
	for _, owner := range obj.GetOwnerReferences() {
		ownerNode, ok := o.uidToNode[owner.UID]
		if !ok {
			ownerNode = o.ownerToVirtualNode(owner, newNode.identity.Namespace)
		}

		ownerNode.addDependent(newNode)
	}
}

// ownerToVirtualNode creates a virtual node as a placeholder for the Kubernetes owner object received in input.
// The virtual node will be eventually converted to an actual node when the node will be visited during discovery.
func (o *objectGraph) ownerToVirtualNode(owner metav1.OwnerReference, namespace string) *node {
	ownerNode := &node{
		identity: objectReference{
			APIVersion: owner.APIVersion,
			Kind:       owner.Kind,
			Name:       owner.Name,
			UID:        owner.UID,
			Namespace:  namespace,
		},
		dependents: make(map[*node]empty),
		virtual:    true,
	}

	o.uidToNode[ownerNode.identity.UID] = ownerNode
	return ownerNode
}

// objToNode creates a node for the Kubernetes object received in input.
// If the node corresponding to the Kubernetes object already exists as a virtual node detected when processing OwnerReferences,
// the node is marked as Observed.
func (o *objectGraph) objToNode(obj *unstructured.Unstructured) *node {
	existingNode, found := o.uidToNode[obj.GetUID()]
	if found {
		existingNode.markObserved()
		return existingNode
	}

	newNode := &node{
		identity: objectReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			UID:        obj.GetUID(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
		},
		dependents: make(map[*node]empty),
		virtual:    false,
	}

	o.uidToNode[newNode.identity.UID] = newNode
	return newNode
}

// getDiscoveryTypes returns the list of TypeMeta to be considered for the the move discovery phase.
// This list includes all the types defines by the CRDs installed by clusterctl and the ConfigMap/Secret core types.
func (o *objectGraph) getDiscoveryTypes() ([]metav1.TypeMeta, error) {
	discoveredTypes := []metav1.TypeMeta{}

	c, err := o.proxy.NewClient()
	if err != nil {
		return nil, err
	}

	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := c.List(ctx, crdList, client.MatchingLabels{clusterctlv1.ClusterctlLabelName: ""}); err != nil {
		return nil, errors.Wrap(err, "failed to get the list of CRDs required for the move discovery phase")
	}

	for _, crd := range crdList.Items {
		for _, version := range crd.Spec.Versions {
			if !version.Storage {
				continue
			}

			discoveredTypes = append(discoveredTypes, metav1.TypeMeta{
				Kind: crd.Spec.Names.Kind,
				APIVersion: metav1.GroupVersion{
					Group:   crd.Spec.Group,
					Version: version.Name,
				}.String(),
			})
		}
	}

	discoveredTypes = append(discoveredTypes, metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"})
	discoveredTypes = append(discoveredTypes, metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"})

	return discoveredTypes, nil
}

// Discovery reads all the Kubernetes objects existing in a namespace for the types received in input, and then adds
// everything to the objects graph.
func (o *objectGraph) Discovery(namespace string, types []metav1.TypeMeta) error {
	c, err := o.proxy.NewClient()
	if err != nil {
		return err
	}

	selectors := []client.ListOption{
		client.InNamespace(namespace),
	}

	for _, typeMeta := range types {
		objList := new(unstructured.UnstructuredList)
		objList.SetAPIVersion(typeMeta.APIVersion)
		objList.SetKind(typeMeta.Kind)

		if err := c.List(ctx, objList, selectors...); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return errors.Wrapf(err, "failed to list %q resources", objList.GroupVersionKind())
		}

		for i := range objList.Items {
			obj := objList.Items[i]
			o.addObj(&obj)
		}
	}

	return nil
}
