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
	appsv1 "k8s.io/api/apps/v1"
)

// GetControllerDeploymentsInput is the input for GetControllerDeployments.
type GetControllerDeploymentsInput struct {
	Lister            Lister
	ExcludeNamespaces []string
}

// GetControllerDeployments returns all the deployment for the cluster API controllers existing in a management cluster.
func GetControllerDeployments(ctx context.Context, input GetControllerDeploymentsInput) []*appsv1.Deployment {
	deploymentList := &appsv1.DeploymentList{}
	Expect(input.Lister.List(ctx, deploymentList, capiProviderOptions()...)).To(Succeed(), "Failed to list deployments for the cluster API controllers")

	deployments := make([]*appsv1.Deployment, 0, len(deploymentList.Items))
	for i := range deploymentList.Items {
		d := &deploymentList.Items[i]
		if !skipDeployment(d, input.ExcludeNamespaces) {
			deployments = append(deployments, d)
		}
	}
	return deployments
}

func skipDeployment(d *appsv1.Deployment, excludeNamespaces []string) bool {
	if !d.DeletionTimestamp.IsZero() {
		return true
	}
	for _, n := range excludeNamespaces {
		if d.Namespace == n {
			return true
		}
	}
	return false
}
