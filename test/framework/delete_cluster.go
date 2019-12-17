/*
Copyright 2019 The Kubernetes Authors.

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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	dockerv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeleteCluster deletes the cluster
// Assertions:
//   * Deletes Machines
//   * Deletes MachineSets
//   * Deletes MachineDeployments
//   * Deletes KubeadmConfigs
//   * Deletes InfraCluster
//   * Deletes InfraMachines
//   * Deletes InfraMachineTemplates
//   * Deletes Secrets
func DeleteCluster(input *MultiNodeControlplaneClusterInput) {
	By("creating multi node cluster")
	MultiNodeControlPlaneCluster(input)

	ctx := context.Background()
	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	// cleanup
	CleanUp(&CleanUpInput{
		Management: input.Management,
		Cluster:    input.Cluster,
	})

	// assertions
	lbl, err := labels.Parse(fmt.Sprintf("%s=%s", clusterv1.ClusterLabelName, input.Cluster.GetClusterName()))
	Expect(err).ToNot(HaveOccurred())
	lo := &client.ListOptions{LabelSelector: lbl}

	ml := &clusterv1.MachineList{}
	Expect(mgmtClient.List(ctx, ml, lo)).To(Succeed())
	Expect(ml.Items).To(HaveLen(0))

	msl := &clusterv1.MachineSetList{}
	Expect(mgmtClient.List(ctx, msl, lo)).To(Succeed())
	Expect(msl.Items).To(HaveLen(0))

	mdl := &clusterv1.MachineDeploymentList{}
	Expect(mgmtClient.List(ctx, mdl, lo)).To(Succeed())
	Expect(mdl.Items).To(HaveLen(0))

	kcl := &clusterv1.KubeadmControlPlaneList{}
	Expect(mgmtClient.List(ctx, kcl, lo)).To(Succeed())
	Expect(kcl.Items).To(HaveLen(0))

	sl := &corev1.SecretList{}
	Expect(mgmtClient.List(ctx, sl, lo)).To(Succeed())
	Expect(sl.Items).To(HaveLen(0))

	switch TypeToKind(input.InfraCluster) {
	case "DockerCluster":
		checkDockerArtifactsAreDeleted(ctx, mgmtClient, lo)
	}
}

func checkDockerArtifactsAreDeleted(ctx context.Context, mgmtClient client.Client, opt *client.ListOptions) {
	dcl := &dockerv1.DockerClusterList{}
	Expect(mgmtClient.List(ctx, dcl, opt)).To(Succeed())
	Expect(dcl.Items).To(HaveLen(0))

	dml := &dockerv1.DockerMachineList{}
	Expect(mgmtClient.List(ctx, dml, opt)).To(Succeed())
	Expect(dml.Items).To(HaveLen(0))

	dmtl := &dockerv1.DockerMachineTemplateList{}
	Expect(mgmtClient.List(ctx, dmtl, opt)).To(Succeed())
	Expect(dmtl.Items).To(HaveLen(0))
}
