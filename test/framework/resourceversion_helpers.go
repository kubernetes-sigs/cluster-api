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
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// ValidateResourceVersionStable checks that resource versions are stable.
func ValidateResourceVersionStable(ctx context.Context, proxy ClusterProxy, namespace string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) {
	// Wait until resource versions are stable for a bit.
	var previousResourceVersions map[string]string
	Eventually(func(g Gomega) {
		objectsWithResourceVersion, err := getObjectsWithResourceVersion(ctx, proxy, namespace, ownerGraphFilterFunction)
		g.Expect(err).ToNot(HaveOccurred())

		defer func() {
			// Set current resource versions as previous resource versions for the next try.
			previousResourceVersions = objectsWithResourceVersion
		}()
		// This is intentionally failing on the first run.
		g.Expect(objectsWithResourceVersion).To(BeComparableTo(previousResourceVersions))
	}, 1*time.Minute, 15*time.Second).Should(Succeed(), "Resource versions never became stable")

	// Verify resource versions are stable for a while.
	Consistently(func(g Gomega) {
		objectsWithResourceVersion, err := getObjectsWithResourceVersion(ctx, proxy, namespace, ownerGraphFilterFunction)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(objectsWithResourceVersion).To(BeComparableTo(previousResourceVersions))
	}, 2*time.Minute, 15*time.Second).Should(Succeed(), "Resource versions didn't stay stable")
}

func getObjectsWithResourceVersion(ctx context.Context, proxy ClusterProxy, namespace string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) (map[string]string, error) {
	graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction)
	if err != nil {
		return nil, err
	}

	objectsWithResourceVersion := map[string]string{}
	for _, node := range graph {
		nodeNamespacedName := client.ObjectKey{Namespace: node.Object.Namespace, Name: node.Object.Name}
		obj := &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: node.Object.APIVersion,
				Kind:       node.Object.Kind,
			},
		}
		if err := proxy.GetClient().Get(ctx, nodeNamespacedName, obj); err != nil {
			return nil, err
		}
		objectsWithResourceVersion[fmt.Sprintf("%s/%s/%s", node.Object.Kind, node.Object.Namespace, node.Object.Name)] = obj.ResourceVersion
	}
	return objectsWithResourceVersion, nil
}
