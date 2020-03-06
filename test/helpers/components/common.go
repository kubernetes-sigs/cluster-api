/*
Copyright 2018 The Kubernetes Authors.

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

package components

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/cluster-api/test/helpers/flag"
	"sigs.k8s.io/cluster-api/test/helpers/kind"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CAPIVersion = "v0.2.2"
)

var (
	capiComponents = flag.DefineOrLookupStringFlag("capiComponents", "https://github.com/kubernetes-sigs/cluster-api/releases/download/"+CAPIVersion+"/cluster-api-components.yaml", "URL to CAPI components to load")
)

func DeployCAPIComponents(kindCluster kind.Cluster) {
	Expect(capiComponents).ToNot(BeNil())
	fmt.Fprintf(GinkgoWriter, "Applying cluster-api components\n")
	Expect(*capiComponents).ToNot(BeEmpty())
	kindCluster.ApplyYAML(*capiComponents)
}

func WaitDeployment(c client.Client, namespace, name string) {
	fmt.Fprintf(GinkgoWriter, "Ensuring %s/%s is deployed\n", namespace, name)
	Eventually(
		func() (int32, error) {
			deployment := &appsv1.Deployment{}
			if err := c.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: name}, deployment); err != nil {
				return 0, err
			}
			return deployment.Status.ReadyReplicas, nil
		}, 5*time.Minute, 15*time.Second,
	).ShouldNot(BeZero())
}
