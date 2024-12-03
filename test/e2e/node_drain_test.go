//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
)

var _ = Describe("When testing Node drain", func() {
	NodeDrainTimeoutSpec(ctx, func() NodeDrainTimeoutSpecInput {
		return NodeDrainTimeoutSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			Flavor:                 ptr.To("topology"),
			InfrastructureProvider: ptr.To("docker"),
			VerifyNodeVolumeDetach: true,
			CreateAdditionalResources: func(ctx context.Context, clusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster) {
				workloadClusterClient := clusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name).GetClient()

				nodeList := &corev1.NodeList{}
				Expect(workloadClusterClient.List(ctx, nodeList)).To(Succeed())

				// Create a fake VolumeAttachment object for each Node without having a real backing csi driver.
				for _, node := range nodeList.Items {
					va := generateVolumeAttachment(node)
					Expect(workloadClusterClient.Create(ctx, va)).To(Succeed())
					// Set .Status.Attached to true to make the VolumeAttachment blocking for machine deletions.
					va.Status.Attached = true
					Expect(workloadClusterClient.Status().Update(ctx, va)).To(Succeed())
				}
			},
		}
	})
})

func generateVolumeAttachment(node corev1.Node) *storagev1.VolumeAttachment {
	return &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("va-%s", node.GetName()),
			Finalizers: []string{
				"test.cluster.x-k8s.io/block",
			},
		},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: "manual",
			NodeName: node.GetName(),
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: ptr.To("foo"),
			},
		},
	}
}
