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

package framework

import (
	"context"

	. "github.com/onsi/gomega"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AssertControlPlaneFailureDomainsInput is the input for AssertControlPlaneFailureDomains.
type AssertControlPlaneFailureDomainsInput struct {
	GetLister  GetLister
	ClusterKey client.ObjectKey
	// ExpectedFailureDomains is required because this function cannot (easily) infer what success looks like.
	// In theory this field is not strictly necessary and could be replaced with enough clever logic/math.
	ExpectedFailureDomains map[string]int
}

// AssertControlPlaneFailureDomains will look at all control plane machines and see what failure domains they were
// placed in. If machines were placed in unexpected or wrong failure domains the expectation will fail.
func AssertControlPlaneFailureDomains(ctx context.Context, input AssertControlPlaneFailureDomainsInput) {
	failureDomainCounts := map[string]int{}

	// Look up the cluster object to find all known failure domains.
	cluster := &clusterv1.Cluster{}
	Expect(input.GetLister.Get(ctx, input.ClusterKey, cluster)).To(Succeed())

	for fd := range cluster.Status.FailureDomains {
		failureDomainCounts[fd] = 0
	}

	// Look up all the control plane machines.
	inClustersNamespaceListOption := client.InNamespace(input.ClusterKey.Namespace)
	matchClusterListOption := client.MatchingLabels{
		clusterv1.ClusterLabelName:             input.ClusterKey.Name,
		clusterv1.MachineControlPlaneLabelName: "",
	}

	machineList := &clusterv1.MachineList{}
	Expect(input.GetLister.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption)).
		To(Succeed(), "Couldn't list machines for the cluster %q", input.ClusterKey.Name)

	// Count all control plane machine failure domains.
	for _, machine := range machineList.Items {
		if machine.Spec.FailureDomain == nil {
			continue
		}
		failureDomainCounts[*machine.Spec.FailureDomain]++
	}
	Expect(failureDomainCounts).To(Equal(input.ExpectedFailureDomains))
}
