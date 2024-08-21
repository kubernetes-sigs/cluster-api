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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// ValidateResourceVersionStable checks that resource versions are stable.
func ValidateResourceVersionStable(ctx context.Context, proxy ClusterProxy, namespace string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) {
	// Wait until resource versions are stable for a bit.
	byf("Check Resource versions are stable")
	var previousResourceVersions map[string]string
	var previousObjects map[string]client.Object
	Eventually(func(g Gomega) {
		objectsWithResourceVersion, objects, err := getObjectsWithResourceVersion(ctx, proxy, namespace, ownerGraphFilterFunction)
		g.Expect(err).ToNot(HaveOccurred())

		defer func() {
			// Set current resource versions as previous resource versions for the next try.
			previousResourceVersions = objectsWithResourceVersion
			previousObjects = objects
		}()
		// This is intentionally failing on the first run.
		g.Expect(objectsWithResourceVersion).To(BeComparableTo(previousResourceVersions))
	}, 1*time.Minute, 15*time.Second).Should(Succeed(), "Resource versions never became stable")

	// Verify resource versions are stable for a while.
	byf("Check Resource versions remain stable")
	Consistently(func(g Gomega) {
		objectsWithResourceVersion, objects, err := getObjectsWithResourceVersion(ctx, proxy, namespace, ownerGraphFilterFunction)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(objectsWithResourceVersion).To(BeComparableTo(previousResourceVersions), printObjectDiff(previousObjects, objects))
	}, 2*time.Minute, 15*time.Second).Should(Succeed(), "Resource versions didn't stay stable")
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
		obj := &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: node.Object.APIVersion,
				Kind:       node.Object.Kind,
			},
		}
		if err := proxy.GetClient().Get(ctx, nodeNamespacedName, obj); err != nil {
			return nil, nil, err
		}
		objectsWithResourceVersion[fmt.Sprintf("%s/%s/%s", node.Object.Kind, node.Object.Namespace, node.Object.Name)] = obj.ResourceVersion
		objects[fmt.Sprintf("%s/%s/%s", node.Object.Kind, node.Object.Namespace, node.Object.Name)] = obj
	}
	return objectsWithResourceVersion, objects, nil
}
