//go:build e2e
// +build e2e

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

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
)

var _ = Describe("When following the Cluster API quick-start with ClusterClass [PR-Blocking] [ClusterClass]", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			Flavor:                 ptr.To("topology"),
			InfrastructureProvider: ptr.To("docker"),
			PostMachinesProvisioned: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				By("Deleting the Cluster")

				cluster := &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
					},
				}
				framework.DeleteCluster(ctx, framework.DeleteClusterInput{
					Cluster: cluster,
					Deleter: proxy.GetClient(),
				})
				framework.WaitForClusterDeleted(ctx, framework.WaitForClusterDeletedInput{
					Client:         proxy.GetClient(),
					Cluster:        cluster,
					ArtifactFolder: artifactFolder,
				}, e2eConfig.GetIntervals("quick-start", "wait-delete-cluster")...)

			},
		}
	})
})
