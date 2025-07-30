/*
Copyright 2024 The Kubernetes Authors.

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

package framework

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// ValidateResourceVersionStable checks that resourceVersions are stable.
func ValidateResourceVersionStable(ctx context.Context, proxy ClusterProxy, namespace string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) {
	// Wait until resourceVersions are stable for a bit.
	byf("Check resourceVersions are stable")
	var previousResourceVersions map[string]string
	var previousObjects map[string]client.Object
	Eventually(func(g Gomega) {
		objectsWithResourceVersion, objects, err := getObjectsWithResourceVersion(ctx, proxy, namespace, ownerGraphFilterFunction)
		g.Expect(err).ToNot(HaveOccurred())

		defer func() {
			// Set current resourceVersions as previous resourceVersions for the next try.
			previousResourceVersions = objectsWithResourceVersion
			previousObjects = objects
		}()
		// This is intentionally failing on the first run.
		g.Expect(objectsWithResourceVersion).To(BeComparableTo(previousResourceVersions))
	}, 1*time.Minute, 15*time.Second).Should(Succeed(), "resourceVersions never became stable")

	// Verify resourceVersions are stable for a while.
	byf("Check resourceVersions remain stable")
	Consistently(func(g Gomega) {
		objectsWithResourceVersion, objects, err := getObjectsWithResourceVersion(ctx, proxy, namespace, ownerGraphFilterFunction)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(previousResourceVersions).To(BeComparableTo(objectsWithResourceVersion), printObjectDiff(previousObjects, objects))
	}, 2*time.Minute, 15*time.Second).Should(Succeed(), "resourceVersions didn't stay stable")
}

func printObjectDiff(previousObjects, newObjects map[string]client.Object) func() string {
	return func() string {
		createdObjects := objectIDs(newObjects).Difference(objectIDs(previousObjects))
		deletedObjects := objectIDs(previousObjects).Difference(objectIDs(newObjects))
		preservedObjects := objectIDs(previousObjects).Intersection(objectIDs(newObjects))

		var output strings.Builder

		if len(createdObjects) > 0 {
			output.WriteString("\nDetected new objects\n")
			for objID := range createdObjects {
				obj := newObjects[objID]
				resourceYAML, _ := yaml.Marshal(obj)
				output.WriteString(fmt.Sprintf("\nNew object %s:\n%s\n", objID, resourceYAML))
			}
		}
		if len(deletedObjects) > 0 {
			output.WriteString("\nDetected deleted objects\n")
			for objID := range deletedObjects {
				obj := previousObjects[objID]
				resourceYAML, _ := yaml.Marshal(obj)
				output.WriteString(fmt.Sprintf("\nDeleted object %s:\n%s\n", objID, resourceYAML))
			}
		}

		if len(preservedObjects) > 0 {
			output.WriteString("\nDetected objects with changed resourceVersion\n")
			for objID := range preservedObjects {
				previousObj := previousObjects[objID]
				newObj := newObjects[objID]

				if previousObj.GetResourceVersion() == newObj.GetResourceVersion() {
					continue
				}

				previousResourceYAML, _ := yaml.Marshal(previousObj)
				newResourceYAML, _ := yaml.Marshal(newObj)
				output.WriteString(fmt.Sprintf("\nObject with changed resourceVersion %s:\n%s\n", objID, cmp.Diff(string(previousResourceYAML), string(newResourceYAML))))
			}
		}

		return output.String()
	}
}

func objectIDs(objs map[string]client.Object) sets.Set[string] {
	ret := sets.Set[string]{}
	for id := range objs {
		ret.Insert(id)
	}
	return ret
}

func getObjectsWithResourceVersion(ctx context.Context, proxy ClusterProxy, namespace string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) (map[string]string, map[string]client.Object, error) {
	graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction)
	if err != nil {
		return nil, nil, err
	}

	objectsWithResourceVersion := map[string]string{}
	objects := map[string]client.Object{}
	for _, node := range graph {
		nodeNamespacedName := client.ObjectKey{Namespace: node.Object.Namespace, Name: node.Object.Name}
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(node.Object.APIVersion)
		obj.SetKind(node.Object.Kind)
		if err := proxy.GetClient().Get(ctx, nodeNamespacedName, obj); err != nil {
			return nil, nil, err
		}
		objectsWithResourceVersion[fmt.Sprintf("%s/%s/%s", node.Object.Kind, node.Object.Namespace, node.Object.Name)] = obj.GetResourceVersion()
		objects[fmt.Sprintf("%s/%s/%s", node.Object.Kind, node.Object.Namespace, node.Object.Name)] = obj
	}

	// Drop KubeConfig owned by MachinePools (and corresponding dataSecret), because they can change when the token.
	keysToDelete := sets.Set[string]{}
	for key, obj := range objects {
		if obj.GetObjectKind().GroupVersionKind().Kind != "KubeadmConfig" {
			continue
		}

		isControlledByMachinePool := false
		for _, ref := range obj.GetOwnerReferences() {
			if ref.Controller != nil && *ref.Controller && ref.Kind == "MachinePool" {
				isControlledByMachinePool = true
				break
			}
		}
		if !isControlledByMachinePool {
			continue
		}

		keysToDelete.Insert(key)
		if objUnstructured, ok := obj.(*unstructured.Unstructured); ok {
			if dataSecretName, ok, err := unstructured.NestedString(objUnstructured.Object, "status", "dataSecretName"); ok && err == nil {
				keysToDelete.Insert(fmt.Sprintf("Secret/%s/%s", obj.GetNamespace(), dataSecretName))
			}
		}
	}

	for _, key := range keysToDelete.UnsortedList() {
		delete(objectsWithResourceVersion, key)
		delete(objects, key)
	}

	return objectsWithResourceVersion, objects, nil
}
