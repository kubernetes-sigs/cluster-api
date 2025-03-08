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
	"sigs.k8s.io/cluster-api/api/v1beta1"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getResourceSetBindingForClusterResourceSet(t *testing.T) {
	tests := []struct {
		name      string
		inputCRSB *v1beta1.ClusterResourceSetBinding
		inputCRS  *v1beta1.ClusterResourceSet
		want      *v1beta1.ResourceSetBinding
	}{{
		name: "nil inputs",
		want: nil,
	}, {
		name:      "nil CRS",
		inputCRSB: &v1beta1.ClusterResourceSetBinding{},
		want:      nil,
	}, {
		name:     "nil CRSB",
		inputCRS: &v1beta1.ClusterResourceSet{},
		want:     nil,
	}, {
		name:      "CRSB with no bindings",
		inputCRSB: &v1beta1.ClusterResourceSetBinding{},
		inputCRS:  &v1beta1.ClusterResourceSet{},
		want:      nil,
	}, {
		name: "CRSB with no matching bindings",
		inputCRSB: &v1beta1.ClusterResourceSetBinding{
			Spec: v1beta1.ClusterResourceSetBindingSpec{
				Bindings: []*v1beta1.ResourceSetBinding{
					{ClusterResourceSetName: "bar"},
				},
			},
		},
		inputCRS: &v1beta1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     nil,
	}, {
		name: "CRSB with single matching bindings",
		inputCRSB: &v1beta1.ClusterResourceSetBinding{
			Spec: v1beta1.ClusterResourceSetBindingSpec{
				Bindings: []*v1beta1.ResourceSetBinding{
					{ClusterResourceSetName: "foo"},
				},
			},
		},
		inputCRS: &v1beta1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &v1beta1.ResourceSetBinding{ClusterResourceSetName: "foo"},
	}, {
		name: "CRSB with multiple bindings with match at index 0",
		inputCRSB: &v1beta1.ClusterResourceSetBinding{
			Spec: v1beta1.ClusterResourceSetBindingSpec{
				Bindings: []*v1beta1.ResourceSetBinding{
					{ClusterResourceSetName: "foo"},
					{ClusterResourceSetName: "bar"},
				},
			},
		},
		inputCRS: &v1beta1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &v1beta1.ResourceSetBinding{ClusterResourceSetName: "foo"},
	}, {
		name: "CRSB with multiple bindings with match at index 1",
		inputCRSB: &v1beta1.ClusterResourceSetBinding{
			Spec: v1beta1.ClusterResourceSetBindingSpec{
				Bindings: []*v1beta1.ResourceSetBinding{
					{ClusterResourceSetName: "bar"},
					{ClusterResourceSetName: "foo"},
				},
			},
		},
		inputCRS: &v1beta1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &v1beta1.ResourceSetBinding{ClusterResourceSetName: "foo"},
	}, {
		name: "CRSB with multiple bindings with match at middle index",
		inputCRSB: &v1beta1.ClusterResourceSetBinding{
			Spec: v1beta1.ClusterResourceSetBindingSpec{
				Bindings: []*v1beta1.ResourceSetBinding{
					{ClusterResourceSetName: "bar"},
					{ClusterResourceSetName: "foo"},
					{ClusterResourceSetName: "baz"},
				},
			},
		},
		inputCRS: &v1beta1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &v1beta1.ResourceSetBinding{ClusterResourceSetName: "foo"},
	}, {
		name: "CRSB with multiple bindings with match at last index",
		inputCRSB: &v1beta1.ClusterResourceSetBinding{
			Spec: v1beta1.ClusterResourceSetBindingSpec{
				Bindings: []*v1beta1.ResourceSetBinding{
					{ClusterResourceSetName: "bar"},
					{ClusterResourceSetName: "baz"},
					{ClusterResourceSetName: "foo"},
				},
			},
		},
		inputCRS: &v1beta1.ClusterResourceSet{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		want:     &v1beta1.ResourceSetBinding{ClusterResourceSetName: "foo"},
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
