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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// CleanUpInput are all the dependencies needed to clean up a Cluster API cluster.
type CleanUpInput struct {
	Management    ManagementCluster
	Cluster       *clusterv1.Cluster
	DeleteTimeout time.Duration
}

func (c *CleanUpInput) SetDefaults() {
	if c.DeleteTimeout == 0*time.Second {
		c.DeleteTimeout = 2 * time.Minute
	}
}

// CleanUp deletes the cluster and waits for everything to be gone.
// Generally this test can be reused for many tests since the implementation is so simple.
func CleanUp(input *CleanUpInput) {
	// TODO: check that all the things we expect have the cluster label or
	// else they can't get deleted
	input.SetDefaults()
	ctx := context.Background()
	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	By(fmt.Sprintf("deleting cluster %s", input.Cluster.GetName()))
	Expect(mgmtClient.Delete(ctx, input.Cluster)).NotTo(HaveOccurred())

	Eventually(func() []clusterv1.Cluster {
		clusters := clusterv1.ClusterList{}
		Expect(mgmtClient.List(ctx, &clusters)).NotTo(HaveOccurred())
		return clusters.Items
	}, input.DeleteTimeout, 10*time.Second).Should(HaveLen(0))
}
