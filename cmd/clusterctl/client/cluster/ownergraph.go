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
	"context"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

// OwnerGraph contains a graph with all the objects considered by clusterctl move as nodes and the OwnerReference relationship
// between those objects as edges.
type OwnerGraph map[string]OwnerGraphNode

// OwnerGraphNode is a single node linking an ObjectReference to its OwnerReferences.
type OwnerGraphNode struct {
	Object corev1.ObjectReference
	Owners []metav1.OwnerReference
}

// GetOwnerGraphFilterFunction allows filtering the objects returned by GetOwnerGraph.
// The function has to return true for objects which should be kept.
// NOTE: this function signature is exposed to allow implementation of E2E tests; there is
// no guarantee about the stability of this API.
type GetOwnerGraphFilterFunction func(u unstructured.Unstructured) bool

// FilterClusterObjectsWithNameFilter is used in e2e tests where the owner graph
// gets queried to filter out cluster-wide objects which don't have the s in their
// object name. This avoids assertions on objects which are part of in-parallel
// running tests like ExtensionConfig.
// NOTE: this function signature is exposed to allow implementation of E2E tests; there is
// no guarantee about the stability of this API.
func FilterClusterObjectsWithNameFilter(s string) func(u unstructured.Unstructured) bool {
	return func(u unstructured.Unstructured) bool {
		// Ignore cluster-wide objects which don't have the clusterName in their object
		// name to avoid asserting on cluster-wide objects which get created or deleted
		// by tests which run in-parallel (e.g. ExtensionConfig).
		if u.GetNamespace() == "" && !strings.Contains(u.GetName(), s) {
			return false
		}
		return true
	}
}

// GetOwnerGraph returns a graph with all the objects considered by clusterctl move as nodes and the OwnerReference relationship between those objects as edges.
// NOTE: this data structure is exposed to allow implementation of E2E tests verifying that CAPI can properly rebuild its
// own owner references; there is no guarantee about the stability of this API. Using this test with providers may require
// a custom implementation of this function, or the OwnerGraph it returns.
func GetOwnerGraph(ctx context.Context, namespace, kubeconfigPath string, filterFn GetOwnerGraphFilterFunction) (OwnerGraph, error) {
	p := NewProxy(Kubeconfig{Path: kubeconfigPath, Context: ""})
	// NOTE: Each Cluster API version supports one contract version, and by convention the contract version matches the current API version.
	invClient := newInventoryClient(p, nil, clusterv1.GroupVersion.Version)

	graph := newObjectGraph(p, invClient)

	// Gets all the types defined by the CRDs installed by clusterctl plus the ConfigMap/Secret core types.
	err := graph.getDiscoveryTypes(ctx)
	if err != nil {
		return OwnerGraph{}, errors.Wrap(err, "failed to retrieve discovery types")
	}

	// graph.Discovery can not be used here as it will use the latest APIVersion for ownerReferences - not those
	// present in the object `metadata.ownerReferences`.
	owners, err := discoverOwnerGraph(ctx, namespace, graph, filterFn)
	if err != nil {
		return OwnerGraph{}, errors.Wrap(err, "failed to discovery ownerGraph types")
	}
	return owners, nil
}

func discoverOwnerGraph(ctx context.Context, namespace string, o *objectGraph, filterFn GetOwnerGraphFilterFunction) (OwnerGraph, error) {
	selectors := []client.ListOption{}
	if namespace != "" {
		selectors = append(selectors, client.InNamespace(namespace))
	}
	ownerGraph := OwnerGraph{}

	discoveryBackoff := newReadBackoff()
	for _, discoveryType := range o.types {
		typeMeta := discoveryType.typeMeta
		objList := new(unstructured.UnstructuredList)

		if err := retryWithExponentialBackoff(ctx, discoveryBackoff, func(ctx context.Context) error {
			return getObjList(ctx, o.proxy, &typeMeta, selectors, objList)
		}); err != nil {
			return nil, err
		}

		// if we are discovering Secrets, also secrets from the providers namespace should be included.
		if discoveryType.typeMeta.GetObjectKind().GroupVersionKind().GroupKind() == corev1.SchemeGroupVersion.WithKind("SecretList").GroupKind() {
			providers, err := o.providerInventory.List(ctx)
			if err != nil {
				return nil, err
			}
			for _, p := range providers.Items {
				if p.Type == string(clusterctlv1.InfrastructureProviderType) {
					providerNamespaceSelector := []client.ListOption{client.InNamespace(p.Namespace)}
					providerNamespaceSecretList := new(unstructured.UnstructuredList)
					if err := retryWithExponentialBackoff(ctx, discoveryBackoff, func(ctx context.Context) error {
						return getObjList(ctx, o.proxy, &typeMeta, providerNamespaceSelector, providerNamespaceSecretList)
					}); err != nil {
						return nil, err
					}
					objList.Items = append(objList.Items, providerNamespaceSecretList.Items...)
				}
			}
		}
		for _, obj := range objList.Items {
			// Exclude the objects via the filter function.
			if filterFn != nil && !filterFn(obj) {
				continue
			}
			// Exclude the kube-root-ca.crt ConfigMap from the owner graph.
			if obj.GetKind() == "ConfigMap" && obj.GetName() == "kube-root-ca.crt" {
				continue
			}
			// Exclude the default service account from the owner graph.
			// This Secret is no longer generated by default in Kubernetes 1.24+.
			// This is not a CAPI related Secret, so it can be ignored.
			if obj.GetKind() == "Secret" && strings.Contains(obj.GetName(), "default-token") {
				continue
			}
			ownerGraph = addNodeToOwnerGraph(ownerGraph, obj)
		}
	}
	return ownerGraph, nil
}

func addNodeToOwnerGraph(graph OwnerGraph, obj unstructured.Unstructured) OwnerGraph {
	// write code to add a node to the ownerGraph
	graph[string(obj.GetUID())] = OwnerGraphNode{
		Owners: obj.GetOwnerReferences(),
		Object: corev1.ObjectReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
		},
	}
	return graph
}
