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
	"cmp"
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/controllers/external"
	secretutil "sigs.k8s.io/cluster-api/util/secret"
)

const clusterTopologyNameKey = "cluster.spec.topology.class"
const clusterTopologyNamespaceKey = "cluster.spec.topology.classNamespace"
const clusterResourceSetBindingClusterNameKey = "clusterresourcesetbinding.spec.clustername"

type empty struct{}

type ownerReferenceAttributes struct {
	Controller         *bool
	BlockOwnerDeletion *bool
}

// node defines a node in the Kubernetes object graph that is visited during the discovery phase for the move operation.
type node struct {
	identity corev1.ObjectReference

	// owners contains the list of nodes that own the current node.
	owners map[*node]ownerReferenceAttributes

	// softOwners contains the list of nodes that soft-own the current node.
	// E.g. secrets are soft-owned by a cluster via a naming convention, but without an explicit OwnerReference.
	softOwners map[*node]empty

	// forceMove is set to true if the CRD of this object has the "move" label attached.
	// This ensures the node is moved, regardless of its owner refs.
	forceMove bool

	// forceMoveHierarchy is set to true if the CRD of this object has the "move-hierarchy" label attached.
	// This ensures the node and it's entire hierarchy of dependants (via owner ref chain) is moved.
	forceMoveHierarchy bool

	// isGlobal gets set to true if this object is a global resource (no namespace).
	isGlobal bool

	// isGlobalHierarchy gets set to true if this object is part of a hierarchy of a global resource e.g.
	// a secrets holding credentials for a global identity object.
	// When this flag is true the object should not be deleted from the source cluster.
	isGlobalHierarchy bool

	// shouldNotDelete marks object and direct its descendants to not be deleted
	shouldNotDelete bool

	// virtual records if this node was discovered indirectly, e.g. by processing an OwnerRef, but not yet observed as a concrete object.
	virtual bool

	// newID stores the new UID the objects gets once created in the target cluster.
	newUID types.UID

	// tenant define the list of objects which are tenant for the node, no matter if the node has a direct OwnerReference to the object or if
	// the node is linked to a object indirectly in the OwnerReference chain.
	tenant map[*node]empty

	// restoreObject holds the object that is referenced when creating a node during fromDirectory from file.
	// the object can then be referenced latter when restoring objects to a target management cluster
	restoreObject *unstructured.Unstructured

	// additionalInfo captures any additional information about the object the node represents.
	// E.g. for the cluster object we capture information to see if the cluster uses a manged topology
	// and the cluster class used.
	additionalInfo map[string]interface{}

	// blockingMove is true when the object should prevent a move operation from proceeding as indicated by
	// the presence of the block-move annotation.
	blockingMove bool
}

type discoveryTypeInfo struct {
	typeMeta           metav1.TypeMeta
	forceMove          bool
	forceMoveHierarchy bool
	scope              apiextensionsv1.ResourceScope
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

func (n *node) getFilename() string {
	return n.identity.Kind + "_" + n.identity.Namespace + "_" + n.identity.Name + ".yaml"
}

func (n *node) identityStr() string {
	return fmt.Sprintf("Kind=%s, Name=%s, Namespace=%s", n.identity.Kind, n.identity.Name, n.identity.Namespace)
}

func (n *node) captureAdditionalInformation(obj *unstructured.Unstructured) error {
	// If the node is a cluster check it see if it is uses a managed topology.
	// In case, it uses a managed topology capture the name of the cluster class in use.
	if n.identity.GroupVersionKind().GroupKind() == clusterv1.GroupVersion.WithKind("Cluster").GroupKind() {
		cluster := &clusterv1.Cluster{}
		if err := localScheme.Convert(obj, cluster, nil); err != nil {
			return errors.Wrapf(err, "failed to convert object %s to Cluster", n.identityStr())
		}
		if cluster.Spec.Topology != nil {
			if n.additionalInfo == nil {
				n.additionalInfo = map[string]interface{}{}
			}
			n.additionalInfo[clusterTopologyNameKey] = cluster.GetClassKey().Name
			n.additionalInfo[clusterTopologyNamespaceKey] = cluster.GetClassKey().Namespace
		}
	}

	// If the node is a ClusterResourceSetBinding capture the name of the cluster it is referencing to.
	if n.identity.GroupVersionKind().GroupKind() == addonsv1.GroupVersion.WithKind("ClusterResourceSetBinding").GroupKind() {
		binding := &addonsv1.ClusterResourceSetBinding{}
		if err := localScheme.Convert(obj, binding, nil); err != nil {
			return errors.Wrapf(err, "failed to convert object %s to ClusterResourceSetBinding", n.identityStr())
		}
		if n.additionalInfo == nil {
			n.additionalInfo = map[string]interface{}{}
		}
		n.additionalInfo[clusterResourceSetBindingClusterNameKey] = binding.Spec.ClusterName
	}
	return nil
}

// objectGraph manages the Kubernetes object graph that is generated during the discovery phase for the move operation.
type objectGraph struct {
	proxy             Proxy
	providerInventory InventoryClient
	uidToNode         map[types.UID]*node
	types             map[string]*discoveryTypeInfo
}

func newObjectGraph(proxy Proxy, providerInventory InventoryClient) *objectGraph {
	return &objectGraph{
		proxy:             proxy,
		providerInventory: providerInventory,
		uidToNode:         map[types.UID]*node{},
	}
}

// addObj adds a Kubernetes object to the object graph that is generated during the move discovery phase.
// During add, OwnerReferences are processed in order to create the dependency graph.
func (o *objectGraph) addObj(obj *unstructured.Unstructured) error {
	// Adds the node to the Graph.
	newNode, err := o.objToNode(obj)
	if err != nil {
		return errors.Wrapf(err, "failed to create node for object (Kind=%s, Name=%s)", obj.GetKind(), obj.GetName())
	}

	// Process OwnerReferences; if the owner object does not exists yet, create a virtual node as a placeholder for it.
	o.processOwnerReferences(obj, newNode)
	return nil
}

// addRestoredObj adds a Kubernetes object to the object graph from file that is generated during a fromDirectory
// Populates the restoredObject field to be referenced during fromDirectory
// During add, OwnerReferences are processed in order to create the dependency graph.
func (o *objectGraph) addRestoredObj(obj *unstructured.Unstructured) error {
	// Add object to graph
	if _, err := o.objToNode(obj); err != nil {
		return errors.Wrapf(err, "failed to create node for object (Kind=%s, Name=%s)", obj.GetKind(), obj.GetName())
	}

	// Check to ensure node has been added to graph
	node, found := o.uidToNode[obj.GetUID()]
	if !found {
		return errors.Errorf("error adding obj %v with id %v to object graph", obj.GetName(), obj.GetUID())
	}

	// Copy the raw object yaml to be referenced when restoring object
	node.restoreObject = obj.DeepCopy()

	// Process OwnerReferences; if the owner object does not exists yet, create a virtual node as a placeholder for it.
	o.processOwnerReferences(obj, node)

	return nil
}

func (o *objectGraph) processOwnerReferences(obj *unstructured.Unstructured, node *node) {
	for _, ownerReference := range obj.GetOwnerReferences() {
		ownerNode, ok := o.uidToNode[ownerReference.UID]
		if !ok {
			ownerNode = o.ownerToVirtualNode(ownerReference)
		}

		node.addOwner(ownerNode, ownerReferenceAttributes{
			Controller:         ownerReference.Controller,
			BlockOwnerDeletion: ownerReference.BlockOwnerDeletion,
		})
	}
}

// ownerToVirtualNode creates a virtual node as a placeholder for the Kubernetes owner object received in input.
// The virtual node will be eventually converted to an actual node when the node will be visited during discovery.
func (o *objectGraph) ownerToVirtualNode(owner metav1.OwnerReference) *node {
	ownerNode := &node{
		identity: corev1.ObjectReference{
			APIVersion: owner.APIVersion,
			Kind:       owner.Kind,
			Name:       owner.Name,
			UID:        owner.UID,
			// NOTE: deferring initialization of fields derived from object meta to when the node reference is actually processed.
		},
		owners:     make(map[*node]ownerReferenceAttributes),
		softOwners: make(map[*node]empty),
		tenant:     make(map[*node]empty),
		virtual:    true,
		// NOTE: deferring initialization of fields derived from object meta to when the node reference is actually processed.
	}

	o.uidToNode[ownerNode.identity.UID] = ownerNode
	return ownerNode
}

// objToNode creates a node for the Kubernetes object received in input.
// If the node corresponding to the Kubernetes object already exists as a virtual node detected when processing OwnerReferences,
// the node is marked as Observed.
func (o *objectGraph) objToNode(obj *unstructured.Unstructured) (*node, error) {
	existingNode, found := o.uidToNode[obj.GetUID()]
	if found {
		existingNode.markObserved()

		if err := o.objInfoToNode(obj, existingNode); err != nil {
			return nil, errors.Wrapf(err, "failed to add object info to node for object %s", existingNode.identityStr())
		}

		return existingNode, nil
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
		tenant:         make(map[*node]empty),
		virtual:        false,
		additionalInfo: make(map[string]interface{}),
	}
	if err := o.objInfoToNode(obj, newNode); err != nil {
		return nil, errors.Wrapf(err, "failed to add object info to node for object %s", existingNode.identityStr())
	}

	o.uidToNode[newNode.identity.UID] = newNode
	return newNode, nil
}

func (o *objectGraph) objInfoToNode(obj *unstructured.Unstructured, n *node) error {
	o.objMetaToNode(obj, n)
	if err := n.captureAdditionalInformation(obj); err != nil {
		return errors.Wrapf(err, "failed to capture additional information of object %s", n.identityStr())
	}
	return nil
}

func (o *objectGraph) objMetaToNode(obj *unstructured.Unstructured, n *node) {
	n.identity.Namespace = obj.GetNamespace()
	if _, ok := obj.GetLabels()[clusterctlv1.ClusterctlMoveLabel]; ok {
		n.forceMove = true
	}
	if _, ok := obj.GetLabels()[clusterctlv1.ClusterctlMoveHierarchyLabel]; ok {
		n.forceMoveHierarchy = true
	}

	kindAPIStr := getKindAPIString(metav1.TypeMeta{Kind: obj.GetKind(), APIVersion: obj.GetAPIVersion()})
	if discoveryType, ok := o.types[kindAPIStr]; ok {
		if !n.forceMove && discoveryType.forceMove {
			n.forceMove = true
		}

		if !n.forceMoveHierarchy && discoveryType.forceMoveHierarchy {
			n.forceMoveHierarchy = true
		}

		if discoveryType.scope == apiextensionsv1.ClusterScoped {
			n.isGlobal = true
		}
	}

	_, n.blockingMove = obj.GetAnnotations()[clusterctlv1.BlockMoveAnnotation]
}

// getDiscoveryTypes returns the list of TypeMeta to be considered for the move discovery phase.
// This list includes all the types defines by the CRDs installed by clusterctl and the ConfigMap/Secret core types.
func (o *objectGraph) getDiscoveryTypes(ctx context.Context) error {
	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	getDiscoveryTypesBackoff := newReadBackoff()
	if err := retryWithExponentialBackoff(ctx, getDiscoveryTypesBackoff, func(ctx context.Context) error {
		return getCRDList(ctx, o.proxy, crdList)
	}); err != nil {
		return err
	}

	o.types = make(map[string]*discoveryTypeInfo)

	for _, crd := range crdList.Items {
		for _, version := range crd.Spec.Versions {
			if !version.Storage {
				continue
			}

			// If a CRD is labeled with force move-hierarchy, keep track of this so all the objects of this kind could be moved
			// together with their descendants identified via the owner chain.
			// NOTE: Cluster, ClusterClass and ClusterResourceSet are automatically considered as force move-hierarchy.
			forceMoveHierarchy := false
			if crd.Spec.Group == clusterv1.GroupVersion.Group && crd.Spec.Names.Kind == "Cluster" {
				forceMoveHierarchy = true
			}
			if crd.Spec.Group == clusterv1.GroupVersion.Group && crd.Spec.Names.Kind == "ClusterClass" {
				forceMoveHierarchy = true
			}
			if crd.Spec.Group == addonsv1.GroupVersion.Group && crd.Spec.Names.Kind == "ClusterResourceSet" {
				forceMoveHierarchy = true
			}
			if _, ok := crd.Labels[clusterctlv1.ClusterctlMoveHierarchyLabel]; ok {
				forceMoveHierarchy = true
			}

			// If a CRD is with as force move, keep track of this so all the objects of this type could be moved.
			// NOTE: if a kind is set for force move-hierarchy, it is also automatically force moved.
			forceMove := forceMoveHierarchy
			if _, ok := crd.Labels[clusterctlv1.ClusterctlMoveLabel]; ok {
				forceMove = true
			}

			typeMeta := metav1.TypeMeta{
				Kind: crd.Spec.Names.Kind,
				APIVersion: metav1.GroupVersion{
					Group:   crd.Spec.Group,
					Version: version.Name,
				}.String(),
			}

			o.types[getKindAPIString(typeMeta)] = &discoveryTypeInfo{
				typeMeta:           typeMeta,
				forceMove:          forceMove,
				forceMoveHierarchy: forceMoveHierarchy,
				scope:              crd.Spec.Scope,
			}
		}
	}

	secretTypeMeta := metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"}
	o.types[getKindAPIString(secretTypeMeta)] = &discoveryTypeInfo{typeMeta: secretTypeMeta}

	configMapTypeMeta := metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"}
	o.types[getKindAPIString(configMapTypeMeta)] = &discoveryTypeInfo{typeMeta: configMapTypeMeta}

	return nil
}

// getKindAPIString returns a concatenated string of the API name and the plural of the kind
// Ex: KIND=Foo API NAME=foo.bar.domain.tld => foos.foo.bar.domain.tld.
func getKindAPIString(typeMeta metav1.TypeMeta) string {
	api := strings.Split(typeMeta.APIVersion, "/")[0]
	return fmt.Sprintf("%ss.%s", strings.ToLower(typeMeta.Kind), api)
}

func getCRDList(ctx context.Context, proxy Proxy, crdList *apiextensionsv1.CustomResourceDefinitionList) error {
	c, err := proxy.NewClient(ctx)
	if err != nil {
		return err
	}

	if err := c.List(ctx, crdList, client.HasLabels{clusterctlv1.ClusterctlLabel}); err != nil {
		return errors.Wrap(err, "failed to get the list of CRDs required for the move discovery phase")
	}
	return nil
}

// Discovery reads all the Kubernetes objects existing in a namespace (or in all namespaces if empty) for the types received in input, and then adds
// everything to the objects graph.
func (o *objectGraph) Discovery(ctx context.Context, namespace string) error {
	log := logf.Log
	log.Info("Discovering Cluster API objects")

	selectors := []client.ListOption{}
	if namespace != "" {
		selectors = append(selectors, client.InNamespace(namespace))
	}

	discoveryBackoff := newReadBackoff()
	for _, discoveryType := range o.types {
		typeMeta := discoveryType.typeMeta
		objList := new(unstructured.UnstructuredList)

		if err := retryWithExponentialBackoff(ctx, discoveryBackoff, func(ctx context.Context) error {
			return getObjList(ctx, o.proxy, &typeMeta, selectors, objList)
		}); err != nil {
			return err
		}

		// if we are discovering Secrets, also secrets from the providers namespace should be included.
		if discoveryType.typeMeta.GetObjectKind().GroupVersionKind().GroupKind() == corev1.SchemeGroupVersion.WithKind("Secret").GroupKind() {
			providers, err := o.providerInventory.List(ctx)
			if err != nil {
				return err
			}
			for _, p := range providers.Items {
				if p.Type == string(clusterctlv1.InfrastructureProviderType) {
					providerNamespaceSelector := []client.ListOption{client.InNamespace(p.Namespace)}
					providerNamespaceSecretList := new(unstructured.UnstructuredList)
					if err := retryWithExponentialBackoff(ctx, discoveryBackoff, func(ctx context.Context) error {
						return getObjList(ctx, o.proxy, &typeMeta, providerNamespaceSelector, providerNamespaceSecretList)
					}); err != nil {
						return err
					}
					objList.Items = append(objList.Items, providerNamespaceSecretList.Items...)
				}
			}
		}

		if len(objList.Items) == 0 {
			continue
		}

		log.V(5).Info(typeMeta.Kind, "count", len(objList.Items))
		for i := range objList.Items {
			obj := objList.Items[i]
			if err := o.addObj(&obj); err != nil {
				return errors.Wrapf(err, "failed to add obj (Kind=%s, Name=%s) to graph", obj.GetKind(), obj.GetName())
			}
		}
	}

	// On top of the object in the namespace being moved (secrets from the providers namespace),
	// it is also required to discover ClusterClasses in other namespaces referenced by clusters in the namespace being moved.
	discoveredExternalCC := sets.Set[string]{}
	for _, cluster := range o.getClusters() {
		className, hasName := cluster.additionalInfo[clusterTopologyNameKey]
		classNamespace, hasNamespace := cluster.additionalInfo[clusterTopologyNamespaceKey]
		// If the cluster doesn't have a reference to a ClusterClass, no-op.
		if !hasName || !hasNamespace {
			continue
		}

		// If the cluster reference a ClusterClass in the namespace being moved, no-op (it has been already discovered).
		if classNamespace == namespace {
			continue
		}

		// If the referenced ClusterClass has been already discovered, no-op.
		externalCCKey := klog.KRef(classNamespace.(string), className.(string))
		if discoveredExternalCC.Has(externalCCKey.String()) {
			continue
		}

		// Get the CC and referenced templates.
		ccUnstructured, err := o.fetchRef(ctx, discoveryBackoff, &corev1.ObjectReference{
			Kind:       "ClusterClass",
			Namespace:  externalCCKey.Namespace,
			Name:       externalCCKey.Name,
			APIVersion: clusterv1.GroupVersion.String(),
		})
		if err != nil {
			return errors.Wrap(err, "failed to get ClusterClass")
		}

		cc := &clusterv1.ClusterClass{}
		if err := localScheme.Convert(ccUnstructured, cc, nil); err != nil {
			return errors.Wrap(err, "failed to convert Unstructured to ClusterClass")
		}

		errs := []error{}
		_, err = o.fetchRef(ctx, discoveryBackoff, cc.Spec.Infrastructure.Ref)
		errs = append(errs, err)
		_, err = o.fetchRef(ctx, discoveryBackoff, cc.Spec.ControlPlane.Ref)
		errs = append(errs, err)

		if cc.Spec.ControlPlane.MachineInfrastructure != nil {
			_, err = o.fetchRef(ctx, discoveryBackoff, cc.Spec.ControlPlane.MachineInfrastructure.Ref)
			errs = append(errs, err)
		}

		for _, mdClass := range cc.Spec.Workers.MachineDeployments {
			_, err = o.fetchRef(ctx, discoveryBackoff, mdClass.Template.Infrastructure.Ref)
			errs = append(errs, err)
			_, err = o.fetchRef(ctx, discoveryBackoff, mdClass.Template.Bootstrap.Ref)
			errs = append(errs, err)
		}

		for _, mpClass := range cc.Spec.Workers.MachinePools {
			_, err = o.fetchRef(ctx, discoveryBackoff, mpClass.Template.Infrastructure.Ref)
			errs = append(errs, err)
			_, err = o.fetchRef(ctx, discoveryBackoff, mpClass.Template.Bootstrap.Ref)
			errs = append(errs, err)
		}

		if err := kerrors.NewAggregate(errs); err != nil {
			return errors.Wrap(err, "failed to fetch ClusterClass references")
		}

		discoveredExternalCC.Insert(externalCCKey.String())
	}

	log.V(1).Info("Total objects", "count", len(o.uidToNode))

	// Completes the graph by searching for soft ownership relations such as secrets linked to the cluster
	// by a naming convention (without any explicit OwnerReference).
	o.setSoftOwnership()

	// Completes the graph by setting for each node the list of tenants the node belongs to.
	o.setTenants()

	// Ensure objects which are referenced across namespaces are not deleted.
	return o.setShouldNotDelete(ctx, namespace)
}

// fetchRef collects specified reference and adds to moved objects.
func (o *objectGraph) fetchRef(ctx context.Context, opts wait.Backoff, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	if ref == nil {
		return nil, nil
	}

	var obj *unstructured.Unstructured

	if err := retryWithExponentialBackoff(ctx, opts, func(ctx context.Context) error {
		c, err := o.proxy.NewClient(ctx)
		if err != nil {
			return err
		}

		obj, err = external.Get(ctx, c, ref)
		return err
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to get referenced %q resource", ref.String())
	}

	if err := o.addObj(obj); err != nil {
		return nil, errors.Wrapf(err, "failed to add obj (Kind=%s, Name=%s) to graph", obj.GetKind(), obj.GetName())
	}

	return obj, nil
}

func getObjList(ctx context.Context, proxy Proxy, typeMeta *metav1.TypeMeta, selectors []client.ListOption, objList client.ObjectList) error {
	c, err := proxy.NewClient(ctx)
	if err != nil {
		return err
	}

	if typeMeta != nil {
		objList.GetObjectKind().SetGroupVersionKind(typeMeta.GroupVersionKind())
	}

	if err := c.List(ctx, objList, selectors...); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list %q resources", objList.GetObjectKind().GroupVersionKind())
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

// getClusterClasses returns the list of ClusterClasses existing in the object graph.
func (o *objectGraph) getClusterClasses() []*node {
	clusterClasses := []*node{}
	for _, node := range o.uidToNode {
		if node.identity.GroupVersionKind().GroupKind() == clusterv1.GroupVersion.WithKind("ClusterClass").GroupKind() {
			clusterClasses = append(clusterClasses, node)
		}
	}
	return clusterClasses
}

// getClusterResourceSetBinding returns the list of ClusterResourceSetBinding existing in the object graph.
func (o *objectGraph) getClusterResourceSetBinding() []*node {
	crs := []*node{}
	for _, node := range o.uidToNode {
		if node.identity.GroupVersionKind().GroupKind() == addonsv1.GroupVersion.WithKind("ClusterResourceSetBinding").GroupKind() {
			crs = append(crs, node)
		}
	}
	return crs
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

// getCRSs returns the list of ClusterResourceSet existing in the object graph.
func (o *objectGraph) getCRSs() []*node {
	clusters := []*node{}
	for _, node := range o.uidToNode {
		if node.identity.GroupVersionKind().GroupKind() == addonsv1.GroupVersion.WithKind("ClusterResourceSet").GroupKind() {
			clusters = append(clusters, node)
		}
	}
	return clusters
}

// getMoveNodes returns the list of nodes existing in the object graph that belong at least to one tenant (e.g Cluster or to a ClusterResourceSet)
// or it is labeled for force move (at object level or at CRD level).
func (o *objectGraph) getMoveNodes() []*node {
	nodes := []*node{}
	for _, node := range o.uidToNode {
		if len(node.tenant) > 0 || node.forceMove {
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
	log := logf.Log
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
			log.V(5).Info("Excluding secret from move (not linked with any Cluster)", "name", secret.identity.Name)
			continue
		}

		// If the secret is linked to a cluster, then add the cluster to the list of the secrets's softOwners.
		for _, cluster := range clusters {
			if secretClusterName == cluster.identity.Name && secret.identity.Namespace == cluster.identity.Namespace {
				secret.addSoftOwner(cluster)
			}
		}
	}

	clusterClasses := o.getClusterClasses()

	// Cluster that uses a ClusterClass are soft owned by that ClusterClass.
	for _, clusterClass := range clusterClasses {
		for _, cluster := range clusters {
			// if the cluster uses a managed topology and uses the clusterclass
			// set the clusterclass as a soft owner of the cluster.
			className, hasName := cluster.additionalInfo[clusterTopologyNameKey]
			classNamespace, hasNamespace := cluster.additionalInfo[clusterTopologyNamespaceKey]
			if namespace, ok := classNamespace.(string); !hasNamespace || !ok {
				classNamespace = cmp.Or(namespace, cluster.identity.Namespace)
			}

			if hasName && className == clusterClass.identity.Name && clusterClass.identity.Namespace == classNamespace {
				cluster.addSoftOwner(clusterClass)
			}
		}
	}

	crsBindings := o.getClusterResourceSetBinding()
	// ClusterResourceSetBinding that refers to a Cluster are soft owned by that Cluster.
	for _, binding := range crsBindings {
		clusterName, ok := binding.additionalInfo[clusterResourceSetBindingClusterNameKey]
		if !ok {
			continue
		}

		for _, cluster := range clusters {
			if clusterName == cluster.identity.Name && binding.identity.Namespace == cluster.identity.Namespace {
				binding.addSoftOwner(cluster)
			}
		}
	}
}

// setTenants identifies all the nodes linked to a parent with forceMoveHierarchy = true (e.g. Clusters or ClusterResourceSet)
// via the owner ref chain.
func (o *objectGraph) setTenants() {
	for _, node := range o.getNodes() {
		if node.forceMoveHierarchy {
			o.setTenantHierarchy(node, node, node.isGlobal)
		}
	}
}

// setTenantHierarchy sets a tenant for a node and for its own dependents/softDependents.
func (o *objectGraph) setTenantHierarchy(node, tenant *node, isGlobalHierarchy bool) {
	node.tenant[tenant] = empty{}
	node.isGlobalHierarchy = node.isGlobalHierarchy || isGlobalHierarchy
	for _, other := range o.getNodes() {
		if other.isOwnedBy(node) || other.isSoftOwnedBy(node) {
			o.setTenantHierarchy(other, tenant, isGlobalHierarchy)
		}
	}
}

// checkVirtualNode logs if nodes are still virtual.
func (o *objectGraph) checkVirtualNode() {
	log := logf.Log
	for _, node := range o.uidToNode {
		if node.virtual {
			log.V(5).Info("Object won't be moved because it's not included in GVK considered for move", "kind", node.identity.Kind, "name", node.identity.Name)
		}
	}
}

// Ensure objects which are referenced across namespaces are not deleted.
func (o *objectGraph) setShouldNotDelete(ctx context.Context, namespace string) error {
	if namespace == "" {
		return nil
	}

	// If there are ClusterClasses outside of the namespace being moved, those CC should not be deleted (we are moving a namespace).
	for _, class := range o.getClusterClasses() {
		if class.identity.Namespace != namespace {
			class.shouldNotDelete = true
			// Ensure that also the templates referenced by the CC won't be deleted.
			o.setShouldNotDeleteHierarchy(class)
		}
	}

	// If there are clusters outside of the namespace being moved and referencing one of ClusterClass in the namespace being moved,
	// ensure those ClusterClass are not deleted.
	discoveryBackoff := newReadBackoff()
	allClusters := &clusterv1.ClusterList{}
	if err := retryWithExponentialBackoff(ctx, discoveryBackoff, func(ctx context.Context) error {
		return getObjList(ctx, o.proxy, nil, []client.ListOption{}, allClusters)
	}); err != nil {
		return err
	}

	for _, cluster := range allClusters.Items {
		// ignore cluster in the namespace being moved.
		if cluster.Namespace == namespace {
			continue
		}

		// ignore cluster not using a CC.
		if cluster.Spec.Topology == nil {
			continue
		}

		// ignore cluster not referencing a CC in the namespace being moved.
		if cluster.Spec.Topology.ClassNamespace != namespace {
			continue
		}

		// Otherwise mark the referenced CC as should not be deleted.
		for _, class := range o.getClusterClasses() {
			if class.identity.Namespace == cluster.Spec.Topology.ClassNamespace && class.identity.Name == cluster.Spec.Topology.Class {
				class.shouldNotDelete = true
				// Ensure that also the templates referenced by the CC won't be deleted.
				o.setShouldNotDeleteHierarchy(class)
			}
		}
	}

	return nil
}

// setShouldNotDeleteHierarchy sets should not delete for a node and for its own dependents/softDependents.
func (o *objectGraph) setShouldNotDeleteHierarchy(node *node) {
	node.shouldNotDelete = true
	for _, other := range o.getNodes() {
		// Skip removal only for direct owners from the object namespace
		if other.isOwnedBy(node) {
			o.setShouldNotDeleteHierarchy(other)
		}
	}
}
