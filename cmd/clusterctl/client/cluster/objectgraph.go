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
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	secretutil "sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type empty struct{}

type ownerReferenceAttributes struct {
	Controller         *bool
	BlockOwnerDeletion *bool
}

// node defines a node in the Kubernetes object graph that is visited during the discovery phase for the move operation.
type node struct {
	identity corev1.ObjectReference

	// owners contains the list of nodes that are owned by the current node.
	owners map[*node]ownerReferenceAttributes

	// softOwners contains the list of nodes that are soft-owned by the current node.
	// E.g. secrets are soft-owned by a cluster via a naming convention, but without an explicit OwnerReference.
	softOwners map[*node]empty

	// virtual records if this node was discovered indirectly, e.g. by processing an OwnerRef, but not yet observed as a concrete object.
	virtual bool

	//newID stores the new UID the objects gets once created in the target cluster.
	newUID types.UID

	// tenantClusters define the list of Clusters which are tenant for the node, no matter if the node has a direct OwnerReference to the Cluster or if
	// the node is linked to a Cluster indirectly in the OwnerReference chain.
	tenantClusters map[*node]empty
}

// markObserved marks the fact that a node was observed as a concrete object.
func (n *node) markObserved() {
	n.virtual = false
}

func (n *node) addOwner(owner *node, attributes ownerReferenceAttributes) {
	n.owners[owner] = attributes
}

func (n *node) addSoftOwner(owner *node) {
	n.softOwners[owner] = struct{}{}
}

func (n *node) isOwnedBy(other *node) bool {
	_, ok := n.owners[other]
	return ok
}

func (n *node) isSoftOwnedBy(other *node) bool {
	_, ok := n.softOwners[other]
	return ok
}

func (n *node) isGlobal() bool {
	return n.identity.Namespace == ""
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
func (o *objectGraph) addObj(obj *unstructured.Unstructured, types discoveryTypeMetas) {
	// Adds the node to the Graph.
	newNode := o.objToNode(obj)

	// Process OwnerReferences; if the owner object doe not exists yet, create a virtual node as a placeholder for it.
	for _, ownerReference := range obj.GetOwnerReferences() {
		ownerNode, ok := o.uidToNode[ownerReference.UID]
		if !ok {
			// When creating a virtual node, we are assuming it is the same namespace of the current obj
			// unless the GVK of the virtual node is globally scoped.
			namespace := newNode.identity.Namespace
			if types.isTypeGlobal(ownerReference.APIVersion, ownerReference.Kind) {
				namespace = ""
			}
			ownerNode = o.ownerToVirtualNode(ownerReference, namespace)
		}

		newNode.addOwner(ownerNode, ownerReferenceAttributes{
			Controller:         ownerReference.Controller,
			BlockOwnerDeletion: ownerReference.BlockOwnerDeletion,
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
		owners:         make(map[*node]ownerReferenceAttributes),
		softOwners:     make(map[*node]empty),
		tenantClusters: make(map[*node]empty),
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
		owners:         make(map[*node]ownerReferenceAttributes),
		softOwners:     make(map[*node]empty),
		tenantClusters: make(map[*node]empty),
		virtual:        false,
	}

	o.uidToNode[newNode.identity.UID] = newNode
	return newNode
}

// discoveryTypeMeta holds information about types to be considered during discovery
type discoveryTypeMeta struct {
	Kind       string
	APIVersion string
	Scope      apiextensionsv1.ResourceScope
}

type discoveryTypeMetas []discoveryTypeMeta

func (t discoveryTypeMetas) isTypeGlobal(apiVersion, kind string) bool {
	for _, t := range t {
		if t.APIVersion == apiVersion && strings.TrimSuffix(t.Kind, "List") == kind {
			return t.Scope == apiextensionsv1.ClusterScoped
		}
	}
	return false
}

// getDiscoveryTypes returns the list of TypeMeta to be considered for the the move discovery phase.
// This list includes all the types defines by the CRDs installed by clusterctl and the ConfigMap/Secret core types.
func (o *objectGraph) getDiscoveryTypes() (discoveryTypeMetas, error) {
	discoveredTypes := discoveryTypeMetas{}

	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	getDiscoveryTypesBackoff := newReadBackoff()
	if err := retryWithExponentialBackoff(getDiscoveryTypesBackoff, func() error {
		return getCRDList(o.proxy, crdList)
	}); err != nil {
		return nil, err
	}

	for _, crd := range crdList.Items {
		for _, version := range crd.Spec.Versions {
			if !version.Storage {
				continue
			}

			discoveredTypes = append(discoveredTypes, discoveryTypeMeta{
				Kind: crd.Spec.Names.Kind,
				APIVersion: metav1.GroupVersion{
					Group:   crd.Spec.Group,
					Version: version.Name,
				}.String(),
				Scope: crd.Spec.Scope,
			})
		}
	}

	discoveredTypes = append(discoveredTypes, discoveryTypeMeta{Kind: "Secret", APIVersion: "v1", Scope: apiextensionsv1.NamespaceScoped})
	discoveredTypes = append(discoveredTypes, discoveryTypeMeta{Kind: "ConfigMap", APIVersion: "v1", Scope: apiextensionsv1.NamespaceScoped})

	return discoveredTypes, nil
}

func getCRDList(proxy Proxy, crdList *apiextensionsv1.CustomResourceDefinitionList) error {
	c, err := proxy.NewClient()
	if err != nil {
		return err
	}

	if err := c.List(ctx, crdList, client.HasLabels{clusterctlv1.ClusterctlLabelName}); err != nil {
		return errors.Wrap(err, "failed to get the list of CRDs required for the move discovery phase")
	}
	return nil
}

// Discovery reads all the Kubernetes objects existing in a namespace (or in all namespaces if empty) for the types received in input, and then adds
// everything to the objects graph.
func (o *objectGraph) Discovery(namespace string, types discoveryTypeMetas) error {
	log := logf.Log
	log.Info("Discovering Cluster API objects")

	discoveryBackoff := newReadBackoff()
	for i := range types {
		typeMeta := types[i]
		objList := new(unstructured.UnstructuredList)

		selectors := []client.ListOption{}
		// adds namespace selector if the resource is NamespaceScoped and namespace value is not empty.
		if typeMeta.Scope == apiextensionsv1.NamespaceScoped && namespace != "" {
			selectors = append(selectors, client.InNamespace(namespace))
		}

		if err := retryWithExponentialBackoff(discoveryBackoff, func() error {
			return getObjList(o.proxy, typeMeta, selectors, objList)
		}); err != nil {
			return err
		}

		if len(objList.Items) == 0 {
			continue
		}

		log.V(5).Info(typeMeta.Kind, "Count", len(objList.Items))
		for i := range objList.Items {
			obj := objList.Items[i]
			o.addObj(&obj, types)
		}
	}

	log.V(1).Info("Total objects", "Count", len(o.uidToNode))

	// Completes the graph by searching for soft ownership relations such as secrets linked to the cluster
	// by a naming convention (without any explicit OwnerReference).
	o.setSoftOwnership()

	// Completes the graph by setting for each node the list of Clusters the node belong to.
	o.setClusterTenants()

	// Completes the graph by identifying cluster principals.
	o.setClusterPrincipalsTenants()

	return nil
}

func getObjList(proxy Proxy, typeMeta discoveryTypeMeta, selectors []client.ListOption, objList *unstructured.UnstructuredList) error {
	c, err := proxy.NewClient()
	if err != nil {
		return err
	}

	objList.SetAPIVersion(typeMeta.APIVersion)
	objList.SetKind(typeMeta.Kind)

	if err := c.List(ctx, objList, selectors...); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list %q resources", objList.GroupVersionKind())
	}
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

// getNodes returns the list of nodes existing in the object graph.
func (o *objectGraph) getNodes() []*node {
	nodes := []*node{}
	for _, node := range o.uidToNode {
		nodes = append(nodes, node)
	}
	return nodes
}

// getNodesWithClusterTenants returns the list of nodes existing in the object graph that belong at least to one Cluster.
func (o *objectGraph) getNodesWithClusterTenants() []*node {
	nodes := []*node{}
	for _, node := range o.uidToNode {
		if len(node.tenantClusters) > 0 {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// getMachines returns the list of Machine existing in the object graph.
func (o *objectGraph) getMachines() []*node {
	machines := []*node{}
	for _, node := range o.uidToNode {
		if node.identity.GroupVersionKind().GroupKind() == clusterv1.GroupVersion.WithKind("Machine").GroupKind() {
			machines = append(machines, node)
		}
	}
	return machines
}

// setSoftOwnership searches for soft ownership relations such as secrets linked to the cluster by a naming convention (without any explicit OwnerReference).
func (o *objectGraph) setSoftOwnership() {
	clusters := o.getClusters()
	for _, secret := range o.getSecrets() {
		// If the secret has at least one OwnerReference ignore it.
		// NB. Cluster API generated secrets have an explicit OwnerReference to the ControlPlane or the KubeadmConfig object while user provided secrets might not have one.
		if len(secret.owners) > 0 {
			continue
		}

		// If the secret name is not a valid cluster secret name, ignore it.
		secretClusterName, _, err := secretutil.ParseSecretName(secret.identity.Name)
		if err != nil {
			continue
		}

		// If the secret is linked to a cluster, then add the cluster to the list of the secrets's softOwners.
		for _, cluster := range clusters {
			if secretClusterName == cluster.identity.Name && secret.identity.Namespace == cluster.identity.Namespace {
				secret.addSoftOwner(cluster)
			}
		}
	}
}

// setClusterTenants sets the cluster tenants for the clusters itself and all their dependent object tree.
func (o *objectGraph) setClusterTenants() {
	for _, cluster := range o.getClusters() {
		o.setClusterTenant(cluster, cluster)
	}
}

// setNodeTenant sets a tenant for a node and for its own dependents/sofDependents.
func (o *objectGraph) setClusterTenant(node, tenant *node) {
	node.tenantClusters[tenant] = empty{}
	for _, other := range o.getNodes() {
		if other.isOwnedBy(node) || other.isSoftOwnedBy(node) {
			o.setClusterTenant(other, tenant)
		}
	}
}

// setClusterPrincipalsTenants sets the cluster tenants for the clusters principal.
// NOTE: Cluster principals are defined as infrastructure specific objects, globally scoped;
// In case a cluster principal is used, clusterctl discovery can rely on a OwnerReference on the
// infrastructure cluster object for identifying the cluster principal
func (o *objectGraph) setClusterPrincipalsTenants() {
	for _, cluster := range o.getClusters() {
		if infrastructureCluster := o.getInfrastructureCluster(cluster); infrastructureCluster != nil {
			if principal := o.getPrincipal(infrastructureCluster); principal != nil {
				principal.tenantClusters[cluster] = empty{}
			}
		}
	}
}

// getInfrastructureCluster identifies the InfrastructureCluster object among all the nodes owned by a cluster.
// NOTE: This assumes the infrastructure provider following a common naming convention.
func (o *objectGraph) getInfrastructureCluster(cluster *node) *node {
	for _, n := range o.getNodes() {
		if !n.isOwnedBy(cluster) {
			continue
		}
		if strings.HasSuffix(n.identity.Kind, "Cluster") {
			return n
		}
	}
	return nil
}

// getPrincipal identifies the Principal among all the nodes owned by a InfrastructureCluster.
// NOTE: This assumes the infrastructure provider following a common naming convention.
func (o *objectGraph) getPrincipal(infrastructureCluster *node) *node {
	for _, n := range o.getNodes() {
		if !n.isGlobal() {
			continue
		}
		if !infrastructureCluster.isOwnedBy(n) {
			continue
		}

		if strings.HasSuffix(n.identity.Kind, "Principal") {
			return n
		}
	}
	return nil
}
