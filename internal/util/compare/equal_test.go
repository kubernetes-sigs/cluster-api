/*
Copyright 2024 The Kubernetes Authors.

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

package compare

import (
	"testing"

	. "github.com/onsi/gomega"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestDiff(t *testing.T) {
	type test struct {
		name      string
		x, y      interface{}
		wantEqual bool
		wantDiff  string
		wantError string
	}

	tests := []test{
		{
			name:      "Equal integers, no diff",
			x:         1,
			y:         1,
			wantEqual: true,
		},
		{
			name:      "Different integers, diff",
			x:         1,
			y:         2,
			wantEqual: false,
			wantDiff: `int(
-   1,
+   2,
  )`,
		},
		{
			name: "Different labels, diff",
			x: map[string]string{
				clusterv1.ClusterNameLabel:          "cluster-1",
				clusterv1.ClusterTopologyOwnedLabel: "",
			},
			y: map[string]string{
				clusterv1.ClusterNameLabel: "cluster-2",
			},
			wantEqual: false,
			wantDiff: `map[string]string{
-   "cluster.x-k8s.io/cluster-name":   "cluster-1",
+   "cluster.x-k8s.io/cluster-name":   "cluster-2",
-   "topology.cluster.x-k8s.io/owned": "",
  }`,
		},
		{
			name: "Diff unexported fields, error",
			x:    struct{ a, b, c int }{1, 2, 3},
			y:    struct{ a, b, c int }{1, 2, 4},
			wantError: `error diffing objects: cannot handle unexported field at root.a:
	"sigs.k8s.io/cluster-api/internal/util/compare".(struct { a int; b int; c int })
consider using a custom Comparer; if you control the implementation of type, you can also consider using an Exporter, AllowUnexported, or cmpopts.IgnoreUnexported`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotEqual, gotDiff, err := Diff(tt.x, tt.y)

			if tt.wantError != "" {
				g.Expect(err.Error()).To(BeComparableTo(tt.wantError))
			} else {
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(gotEqual).To(Equal(tt.wantEqual))
				g.Expect(gotDiff).To(BeComparableTo(tt.wantDiff))
			}
		})
	}
}
