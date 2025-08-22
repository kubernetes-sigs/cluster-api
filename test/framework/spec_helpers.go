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
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/util"
)

func byf(format string, a ...interface{}) {
	By(fmt.Sprintf(format, a...))
}

// SetupSpecNamespace creates a namespace for the test spec and setups a watch for the namespace events.
func SetupSpecNamespace(ctx context.Context, specName string, clusterProxy ClusterProxy, artifactFolder string, postNamespaceCreated func(ClusterProxy, string)) (*corev1.Namespace, context.CancelFunc) {
	byf("Creating a namespace for hosting the %q test spec", specName)
	namespace, cancelWatches := CreateNamespaceAndWatchEvents(ctx, CreateNamespaceAndWatchEventsInput{
		Creator:   clusterProxy.GetClient(),
		ClientSet: clusterProxy.GetClientSet(),
		Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
		LogFolder: filepath.Join(artifactFolder, "clusters", clusterProxy.GetName()),
	})

	if postNamespaceCreated != nil {
		log.Logf("Calling postNamespaceCreated for namespace %s", namespace.Name)
		postNamespaceCreated(clusterProxy, namespace.Name)
	}

	return namespace, cancelWatches
}

// DumpAllResourcesAndLogs dumps all the resources in the spec namespace and the workload cluster.
func DumpAllResourcesAndLogs(ctx context.Context, clusterProxy ClusterProxy, clusterctlConfigPath, artifactFolder string, namespace *corev1.Namespace, cluster *clusterv1.Cluster) {
	byf("Dumping logs from the %q workload cluster", cluster.Name)

	// Dump all the logs from the workload cluster.
	clusterProxy.CollectWorkloadClusterLogs(ctx, cluster.Namespace, cluster.Name, filepath.Join(artifactFolder, "clusters", cluster.Name))

	byf("Dumping all the Cluster API resources in the %q namespace", namespace.Name)

	// Dump all Cluster API related resources to artifacts.
	DumpAllResources(ctx, DumpAllResourcesInput{
		Lister:               clusterProxy.GetClient(),
		KubeConfigPath:       clusterProxy.GetKubeconfigPath(),
		ClusterctlConfigPath: clusterctlConfigPath,
		Namespace:            namespace.Name,
		LogPath:              filepath.Join(artifactFolder, "clusters", clusterProxy.GetName(), "resources"),
	})

	// If the cluster still exists, dump pods and nodes of the workload cluster.
	if err := clusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(cluster), &clusterv1.Cluster{}); err == nil {
		byf("Dumping Pods and Nodes of Cluster %s", klog.KObj(cluster))
		DumpResourcesForCluster(ctx, DumpResourcesForClusterInput{
			Lister:  clusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name).GetClient(),
			Cluster: cluster,
			LogPath: filepath.Join(artifactFolder, "clusters", cluster.Name, "resources"),
			Resources: []DumpNamespaceAndGVK{
				{
					GVK: schema.GroupVersionKind{
						Version: corev1.SchemeGroupVersion.Version,
						Kind:    "Pod",
					},
				},
				{
					GVK: schema.GroupVersionKind{
						Version: corev1.SchemeGroupVersion.Version,
						Kind:    "Node",
					},
				},
			},
		})
	}
}

// DumpSpecResourcesAndCleanup dumps all the resources in the spec namespace and cleans up the spec namespace.
func DumpSpecResourcesAndCleanup(ctx context.Context, specName string, clusterProxy ClusterProxy, clusterctlConfigPath, artifactFolder string, namespace *corev1.Namespace, cancelWatches context.CancelFunc, cluster *clusterv1.Cluster, intervalsGetter func(spec, key string) []interface{}, skipCleanup bool) {
	// Dump all the resources in the spec namespace and the workload cluster.
	DumpAllResourcesAndLogs(ctx, clusterProxy, clusterctlConfigPath, artifactFolder, namespace, cluster)

	if !skipCleanup {
		byf("Deleting cluster %s", klog.KObj(cluster))
		// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
		// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
		// instead of DeleteClusterAndWait
		DeleteAllClustersAndWait(ctx, DeleteAllClustersAndWaitInput{
			ClusterProxy:         clusterProxy,
			ClusterctlConfigPath: clusterctlConfigPath,
			Namespace:            namespace.Name,
			ArtifactFolder:       artifactFolder,
		}, intervalsGetter(specName, "wait-delete-cluster")...)

		byf("Deleting namespace used for hosting the %q test spec", specName)
		DeleteNamespace(ctx, DeleteNamespaceInput{
			Deleter: clusterProxy.GetClient(),
			Name:    namespace.Name,
		})
	}
	cancelWatches()
}
