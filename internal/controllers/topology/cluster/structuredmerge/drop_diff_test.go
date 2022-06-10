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

package structuredmerge

import (
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/internal/contract"
)

func Test_dropDiffForNotAllowedPaths(t *testing.T) {
	tests := []struct {
		name         string
		ctx          *dropDiffInput
		wantModified map[string]interface{}
	}{
		{
			name: "Sets not allowed paths to original value if defined",
			ctx: &dropDiffInput{
				path: contract.Path{},
				original: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
					"status": map[string]interface{}{
						"foo": "123",
					},
				},
				modified: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo-changed",
						"labels": map[string]interface{}{
							"foo": "123",
						},
						"annotations": map[string]interface{}{
							"foo": "123",
						},
					},
					"spec": map[string]interface{}{
						"foo": "123",
					},
					"status": map[string]interface{}{
						"foo": "123-changed",
					},
				},
				shouldDropDiffFunc: isNotAllowedPath(
					[]contract.Path{ // NOTE: we are dropping everything not in this list (IsNotAllowed)
						{"metadata", "labels"},
						{"metadata", "annotations"},
						{"spec"},
					},
				),
			},
			wantModified: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "foo", // metadata.name aligned to original
					"labels": map[string]interface{}{
						"foo": "123",
					},
					"annotations": map[string]interface{}{
						"foo": "123",
					},
				},
				"spec": map[string]interface{}{
					"foo": "123",
				},
				"status": map[string]interface{}{ // status aligned to original
					"foo": "123",
				},
			},
		},
		{
			name: "Drops not allowed paths if they do not exist in original",
			ctx: &dropDiffInput{
				path:     contract.Path{},
				original: map[string]interface{}{
					// Original doesn't have values for not allowed paths.
				},
				modified: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
						"labels": map[string]interface{}{
							"foo": "123",
						},
						"annotations": map[string]interface{}{
							"foo": "123",
						},
					},
					"spec": map[string]interface{}{
						"foo": "123",
					},
					"status": map[string]interface{}{
						"foo": "123",
					},
				},
				shouldDropDiffFunc: isNotAllowedPath(
					[]contract.Path{ // NOTE: we are dropping everything not in this list (IsNotAllowed)
						{"metadata", "labels"},
						{"metadata", "annotations"},
						{"spec"},
					},
				),
			},
			wantModified: map[string]interface{}{
				"metadata": map[string]interface{}{
					// metadata.name dropped
					"labels": map[string]interface{}{
						"foo": "123",
					},
					"annotations": map[string]interface{}{
						"foo": "123",
					},
				},
				"spec": map[string]interface{}{
					"foo": "123",
				},
				// status dropped
			},
		},
		{
			name: "Cleanup empty maps",
			ctx: &dropDiffInput{
				path:     contract.Path{},
				original: map[string]interface{}{
					// Original doesn't have values for not allowed paths.
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "123",
					},
				},
				shouldDropDiffFunc: isNotAllowedPath(
					[]contract.Path{}, // NOTE: we are dropping everything not in this list (IsNotAllowed)
				),
			},
			wantModified: map[string]interface{}{
				// we are dropping spec.foo and then spec given that it is an empty map
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			dropDiff(tt.ctx)

			g.Expect(tt.ctx.modified).To(Equal(tt.wantModified))
		})
	}
}

func Test_dropDiffForIgnoredPaths(t *testing.T) {
	tests := []struct {
		name         string
		ctx          *dropDiffInput
		wantModified map[string]interface{}
	}{
		{
			name: "Sets ignored paths to original value if defined",
			ctx: &dropDiffInput{
				path: contract.Path{},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "foo",
							"port": "123",
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "foo-changed",
							"port": "123-changed",
						},
					},
				},
				shouldDropDiffFunc: isIgnorePath(
					[]contract.Path{
						{"spec", "controlPlaneEndpoint"},
					},
				),
			},
			wantModified: map[string]interface{}{
				"spec": map[string]interface{}{
					"foo": "bar",
					"controlPlaneEndpoint": map[string]interface{}{ // spec.controlPlaneEndpoint aligned to original
						"host": "foo",
						"port": "123",
					},
				},
			},
		},
		{
			name: "Drops ignore paths if they do not exist in original",
			ctx: &dropDiffInput{
				path:     contract.Path{},
				original: map[string]interface{}{
					// Original doesn't have values for ignore paths.
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "foo-changed",
							"port": "123-changed",
						},
					},
				},
				shouldDropDiffFunc: isIgnorePath(
					[]contract.Path{
						{"spec", "controlPlaneEndpoint"},
					},
				),
			},
			wantModified: map[string]interface{}{
				"spec": map[string]interface{}{
					"foo": "bar",
					// spec.controlPlaneEndpoint dropped
				},
			},
		},
		{
			name: "Cleanup empty maps",
			ctx: &dropDiffInput{
				path:     contract.Path{},
				original: map[string]interface{}{
					// Original doesn't have values for not allowed paths.
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "123",
					},
				},
				shouldDropDiffFunc: isIgnorePath(
					[]contract.Path{
						{"spec", "foo"},
					},
				),
			},
			wantModified: map[string]interface{}{
				// we are dropping spec.foo and then spec given that it is an empty map
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			dropDiff(tt.ctx)

			g.Expect(tt.ctx.modified).To(Equal(tt.wantModified))
		})
	}
}
