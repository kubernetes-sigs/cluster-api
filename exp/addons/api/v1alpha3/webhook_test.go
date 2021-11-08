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

package v1alpha3

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestClusterResourceSetConversion(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, fmt.Sprintf("conversion-webhook-%s", util.RandomString(5)))
	g.Expect(err).ToNot(HaveOccurred())
	clusterName := fmt.Sprintf("test-cluster-%s", util.RandomString(5))
	crsName := fmt.Sprintf("test-clusterresourceset-%s", util.RandomString(5))
	crs := &ClusterResourceSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crsName,
			Namespace: ns.Name,
		},
		Spec: ClusterResourceSetSpec{
			Strategy: "ApplyOnce",
			ClusterSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"cni": fmt.Sprintf("%s-crs-cni", clusterName),
				},
			},
			Resources: []ResourceRef{
				{
					Name: fmt.Sprintf("%s-crs-cni", clusterName),
					Kind: "ConfigMap",
				},
			},
		},
	}

	g.Expect(env.Create(ctx, crs)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, crs)
}

func TestClusterResourceSetBindingConversion(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, fmt.Sprintf("conversion-webhook-%s", util.RandomString(5)))
	g.Expect(err).ToNot(HaveOccurred())
	crsbindingName := fmt.Sprintf("test-clusterresourcesetbinding-%s", util.RandomString(5))
	crsbinding := &ClusterResourceSetBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crsbindingName,
			Namespace: ns.Name,
		},
		Spec: ClusterResourceSetBindingSpec{
			Bindings: []*ResourceSetBinding{
				{
					ClusterResourceSetName: "test-clusterresourceset",
					Resources: []ResourceBinding{
						{
							ResourceRef: ResourceRef{
								Name: "ApplySucceeded",
								Kind: "Secret",
							},
							Applied:         true,
							Hash:            "xyz",
							LastAppliedTime: &metav1.Time{Time: time.Now().UTC()},
						},
						{
							ResourceRef: ResourceRef{
								Name: "applyFailed",
								Kind: "Secret",
							},
							Applied:         false,
							Hash:            "",
							LastAppliedTime: &metav1.Time{Time: time.Now().UTC()},
						},
					},
				},
			},
		},
	}

	g.Expect(env.Create(ctx, crsbinding)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, crsbinding)
}
