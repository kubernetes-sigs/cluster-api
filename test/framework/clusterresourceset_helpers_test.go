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
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
)

func Test_getResourceSetBindingForClusterResourceSet(t *testing.T) {
	tests := []struct {
		name      string
		inputCRSB *addonsv1.ClusterResourceSetBinding
		inputCRS  *addonsv1.ClusterResourceSet
		want      *addonsv1.ResourceSetBinding
	}{{
		name: "nil inputs",
		want: nil,
	}, {
		name:      "nil CRS",
		inputCRSB: &addonsv1.ClusterResourceSetBinding{},
		want:      nil,
	}, {
		name:     "nil CRSB",
		inputCRS: &addonsv1.ClusterResourceSet{},
		want:     nil,
	}, {
		name:      "CRSB with no bindings",
		inputCRSB: &addonsv1.ClusterResourceSetBinding{},
		inputCRS:  &addonsv1.ClusterResourceSet{},
		want:      nil,
	}, {
		name: "CRSB with no matching bindings",
		inputCRSB: &addonsv1.ClusterResourceSetBinding{
			Spec: addonsv1.ClusterResourceSetBindingSpec{
				Bindings: []*addonsv1.ResourceSetBinding{
					{ClusterResourceSetName: "bar"},
				},
			},
		},
		inputCRS: &addonsv1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     nil,
	}, {
		name: "CRSB with single matching bindings",
		inputCRSB: &addonsv1.ClusterResourceSetBinding{
			Spec: addonsv1.ClusterResourceSetBindingSpec{
				Bindings: []*addonsv1.ResourceSetBinding{
					{ClusterResourceSetName: "foo"},
				},
			},
		},
		inputCRS: &addonsv1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &addonsv1.ResourceSetBinding{ClusterResourceSetName: "foo"},
	}, {
		name: "CRSB with multiple bindings with match at index 0",
		inputCRSB: &addonsv1.ClusterResourceSetBinding{
			Spec: addonsv1.ClusterResourceSetBindingSpec{
				Bindings: []*addonsv1.ResourceSetBinding{
					{ClusterResourceSetName: "foo"},
					{ClusterResourceSetName: "bar"},
				},
			},
		},
		inputCRS: &addonsv1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &addonsv1.ResourceSetBinding{ClusterResourceSetName: "foo"},
	}, {
		name: "CRSB with multiple bindings with match at index 1",
		inputCRSB: &addonsv1.ClusterResourceSetBinding{
			Spec: addonsv1.ClusterResourceSetBindingSpec{
				Bindings: []*addonsv1.ResourceSetBinding{
					{ClusterResourceSetName: "bar"},
					{ClusterResourceSetName: "foo"},
				},
			},
		},
		inputCRS: &addonsv1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &addonsv1.ResourceSetBinding{ClusterResourceSetName: "foo"},
	}, {
		name: "CRSB with multiple bindings with match at middle index",
		inputCRSB: &addonsv1.ClusterResourceSetBinding{
			Spec: addonsv1.ClusterResourceSetBindingSpec{
				Bindings: []*addonsv1.ResourceSetBinding{
					{ClusterResourceSetName: "bar"},
					{ClusterResourceSetName: "foo"},
					{ClusterResourceSetName: "baz"},
				},
			},
		},
		inputCRS: &addonsv1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &addonsv1.ResourceSetBinding{ClusterResourceSetName: "foo"},
	}, {
		name: "CRSB with multiple bindings with match at last index",
		inputCRSB: &addonsv1.ClusterResourceSetBinding{
			Spec: addonsv1.ClusterResourceSetBindingSpec{
				Bindings: []*addonsv1.ResourceSetBinding{
					{ClusterResourceSetName: "bar"},
					{ClusterResourceSetName: "baz"},
					{ClusterResourceSetName: "foo"},
				},
			},
		},
		inputCRS: &addonsv1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &addonsv1.ResourceSetBinding{ClusterResourceSetName: "foo"},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(
				getResourceSetBindingForClusterResourceSet(
					tt.inputCRSB,
					tt.inputCRS,
				),
			).To(Equal(tt.want))
		})
	}
}
