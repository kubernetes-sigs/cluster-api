/*
Copyright 2022 The Kubernetes Authors.

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

package ssa

import (
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/internal/contract"
)

func Test_filterNotAllowedPaths(t *testing.T) {
	tests := []struct {
		name      string
		ctx       *FilterIntentInput
		wantValue map[string]interface{}
	}{
		{
			name: "Filters out not allowed paths",
			ctx: &FilterIntentInput{
				Path: contract.Path{},
				Value: map[string]interface{}{
					"apiVersion": "foo.bar/v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
						"labels": map[string]interface{}{
							"foo": "123",
						},
						"annotations": map[string]interface{}{
							"foo": "123",
						},
						"resourceVersion": "123",
					},
					"spec": map[string]interface{}{
						"foo": "123",
					},
					"status": map[string]interface{}{
						"foo": "123",
					},
				},
				ShouldFilter: IsNotAllowedPath(
					[]contract.Path{ // NOTE: we are dropping everything not in this list
						{"apiVersion"},
						{"kind"},
						{"metadata", "name"},
						{"metadata", "namespace"},
						{"metadata", "labels"},
						{"metadata", "annotations"},
						{"spec"},
					},
				),
			},
			wantValue: map[string]interface{}{
				"apiVersion": "foo.bar/v1",
				"kind":       "Foo",
				"metadata": map[string]interface{}{
					"name":      "foo",
					"namespace": "bar",
					"labels": map[string]interface{}{
						"foo": "123",
					},
					"annotations": map[string]interface{}{
						"foo": "123",
					},
					// metadata.resourceVersion filtered out
				},
				"spec": map[string]interface{}{
					"foo": "123",
				},
				// status filtered out
			},
		},
		{
			name: "Cleanup empty maps",
			ctx: &FilterIntentInput{
				Path: contract.Path{},
				Value: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "123",
					},
				},
				ShouldFilter: IsNotAllowedPath(
					[]contract.Path{}, // NOTE: we are filtering out everything not in this list (everything)
				),
			},
			wantValue: map[string]interface{}{
				// we are filtering out spec.foo and then spec given that it is an empty map
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			FilterIntent(tt.ctx)

			g.Expect(tt.ctx.Value).To(Equal(tt.wantValue))
		})
	}
}

func Test_filterIgnoredPaths(t *testing.T) {
	tests := []struct {
		name      string
		ctx       *FilterIntentInput
		wantValue map[string]interface{}
	}{
		{
			name: "Filters out ignore paths",
			ctx: &FilterIntentInput{
				Path: contract.Path{},
				Value: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "foo-changed",
							"port": "123-changed",
						},
					},
				},
				ShouldFilter: IsIgnorePath(
					[]contract.Path{
						{"spec", "controlPlaneEndpoint"},
					},
				),
			},
			wantValue: map[string]interface{}{
				"spec": map[string]interface{}{
					"foo": "bar",
					// spec.controlPlaneEndpoint filtered out
				},
			},
		},
		{
			name: "Cleanup empty maps",
			ctx: &FilterIntentInput{
				Path: contract.Path{},
				Value: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "123",
					},
				},
				ShouldFilter: IsIgnorePath(
					[]contract.Path{
						{"spec", "foo"},
					},
				),
			},
			wantValue: map[string]interface{}{
				// we are filtering out spec.foo and then spec given that it is an empty map
			},
		},
		{
			name: "Cleanup empty nested maps",
			ctx: &FilterIntentInput{
				Path: contract.Path{},
				Value: map[string]interface{}{
					"spec": map[string]interface{}{
						"bar": map[string]interface{}{
							"foo": "123",
						},
					},
				},
				ShouldFilter: IsIgnorePath(
					[]contract.Path{
						{"spec", "bar", "foo"},
					},
				),
			},
			wantValue: map[string]interface{}{
				// we are filtering out spec.bar.foo and then spec given that it is an empty map
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			FilterIntent(tt.ctx)

			g.Expect(tt.ctx.Value).To(Equal(tt.wantValue))
		})
	}
}
