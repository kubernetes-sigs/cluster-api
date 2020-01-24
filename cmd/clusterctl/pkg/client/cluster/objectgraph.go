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
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type empty struct{}

type dependAttributes struct {
	Controller         *bool
	BlockOwnerDeletion *bool
}

// node defines a node in the Kubernetes object graph that is visited during the discovery phase for the move operation.
type node struct {
	identity corev1.ObjectReference

	// dependents contains the list of nodes that are owned by the current node.
	// Nb. This is the reverse of metadata.OwnerReferences.
	dependents map[*node]dependAttributes

	// dependents contains the list of nodes that are soft-owned by the current node.
	// E.g. secrets that are linked to a cluster by a naming convention, but without an explicit OwnerReference.
	softDependents map[*node]empty

	// hasOwnerReferences tracks if the object has at least one OwnerReference.
	hasOwnerReferences bool

	// virtual records if this node was discovered indirectly, e.g. by processing an OwnerRef, but not yet observed as a concrete object.
	virtual bool

	//newID stores the new UID the objects gets once created in the target cluster.
	newUID types.UID
}

// markObserved marks the fact that a node was observed as a concrete object.
func (n *node) markObserved() {
	n.virtual = false
}

func (n *node) addDependent(dependent *node, attributes dependAttributes) {
	n.dependents[dependent] = attributes
}

func (n *node) addSoftDependent(dependent *node) {
	n.softDependents[dependent] = struct{}{}
}

// objectGraph manages the Kubernetes object graph that is generated during the discovery phase for the move operation.
type objectGraph struct {
	proxy     Proxy
	uidToNode map[types.UID]*node
	log       logr.Logger
}

func newObjectGraph(proxy Proxy, log logr.Logger) *objectGraph {
	return &objectGraph{
		proxy:     proxy,
		uidToNode: map[types.UID]*node{},
		log:       log,
	}
}

// addObj adds a Kubernetes object to the object graph that is generated during the move discovery phase.
// During add, OwnerReferences are processed in order to create the dependency graph.
func (o *objectGraph) addObj(obj *unstructured.Unstructured) {
	// Adds the node to the Graph.
	newNode := o.objToNode(obj)

	// Tracks if the node has OwnerReferences.
	newNode.hasOwnerReferences = len(obj.GetOwnerReferences()) > 0

	// Process OwnerReferences; if the owner object already exists, update the list of the owner object's dependents; otherwise
	// create a virtual node as a placeholder for the owner objects.
	for _, owner := range obj.GetOwnerReferences() {
		ownerNode, ok := o.uidToNode[owner.UID]
		if !ok {
			ownerNode = o.ownerToVirtualNode(owner, newNode.identity.Namespace)
		}

		ownerNode.addDependent(newNode, dependAttributes{
			Controller:         owner.Controller,
			BlockOwnerDeletion: owner.BlockOwnerDeletion,
		})
	}
}

// ownerToVirtualNode creates a virtual node as a placeholder for the Kubernetes owner object received in input.
// The virtual node will be eventually converted to an actual node when the node will be visited during discovery.
func (o *objectGraph) ownerToVirtualNode(owner metav1.OwnerReference, namespace string) *node {
	ownerNode := &node{
		identity: corev1.ObjectReference{
			APIVersion: owner.APIVersion,
			Kind:       owner.Kind,
			Name:       owner.Name,
			UID:        owner.UID,
			Namespace:  namespace,
		},
		dependents:     make(map[*node]dependAttributes),
		softDependents: make(map[*node]empty),
		virtual:        true,
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
		identity: corev1.ObjectReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			UID:        obj.GetUID(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
		},
		dependents:     make(map[*node]dependAttributes),
		softDependents: make(map[*node]empty),
		virtual:        false,
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

// Discovery reads all the Kubernetes objects existing in a namespace (or in all namespaces if empty) for the types received in input, and then adds
// everything to the objects graph.
func (o *objectGraph) Discovery(namespace string, types []metav1.TypeMeta) error {
	o.log.Info("Discovering Cluster API objects")
	c, err := o.proxy.NewClient()
	if err != nil {
		return err
	}

	selectors := []client.ListOption{}
	if namespace != "" {
		selectors = append(selectors, client.InNamespace(namespace))
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

		o.log.V(2).Info(typeMeta.Kind, "Count", len(objList.Items))
		for i := range objList.Items {
			obj := objList.Items[i]
			o.addObj(&obj)
		}
	}

	o.log.V(1).Info("Total objects", "Count", len(o.uidToNode))

	// Completes the graph by searching for soft dependents relations such as secrets linked to the cluster
	// by a naming convention (without any explicit OwnerReference).
	o.setSoftDependents()

	return nil
}

// getClusters returns the list of Clusters existing in the object graph.
func (o *objectGraph) getClusters() []*node {
	clusters := []*node{}
	for _, node := range o.uidToNode {
		if node.identity.GroupVersionKind().GroupKind() == clusterv1.GroupVersion.WithKind("Cluster").GroupKind() {
			clusters = append(clusters, node)
		}
	}

	// Sorts clusters so move always happens in a predictable order.
	sort.Slice(clusters, func(i, j int) bool {
		return fmt.Sprintf("%s/%s", clusters[i].identity.Namespace, clusters[i].identity.Name) < fmt.Sprintf("%s/%s", clusters[j].identity.Namespace, clusters[j].identity.Name)
	})

	return clusters
}

// getClusters returns the list of Secrets existing in the object graph.
func (o *objectGraph) getSecrets() []*node {
	secrets := []*node{}
	for _, node := range o.uidToNode {
		if node.identity.APIVersion == "v1" && node.identity.Kind == "Secret" {
			secrets = append(secrets, node)
		}
	}
	return secrets
}

// setSoftDependents searches for soft dependents relations such as secrets linked to the cluster by a naming convention (without any explicit OwnerReference).
func (o *objectGraph) setSoftDependents() {
	clusters := o.getClusters()
	for _, secret := range o.getSecrets() {
		// If the secret has at least one OwnerReference ignore it.
		// NB. Cluster API generated secrets have an explicit OwnerReference to the ControlPlane or the KubeadmConfig object while user provided secrets might not have one.
		if secret.hasOwnerReferences {
			continue
		}

		// If the secret name does not comply the naming convention {cluster-name}-{certificate-name/kubeconfig}, ignore it.
		nameSplit := strings.Split(secret.identity.Name, "-")
		if len(nameSplit) != 2 {
			continue
		}

		// If the secret is linked to a cluster by the naming convention, then add the secret to the list of the cluster's softDependents.
		for _, cluster := range clusters {
			if nameSplit[0] == cluster.identity.Name && secret.identity.Namespace == cluster.identity.Namespace {
				cluster.addSoftDependent(secret)
			}
		}
	}
}
