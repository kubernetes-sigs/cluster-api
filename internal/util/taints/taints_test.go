/*
Copyright 2023 The Kubernetes Authors.

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

package taints

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func TestRemoveNodeTaint(t *testing.T) {
	taint1 := corev1.Taint{Key: "taint1", Effect: corev1.TaintEffectNoSchedule}
	taint2 := corev1.Taint{Key: "taint2", Effect: corev1.TaintEffectNoSchedule}

	tests := []struct {
		name         string
		node         *corev1.Node
		dropTaint    corev1.Taint
		wantTaints   []corev1.Taint
		wantModified bool
	}{
		{
			name: "dropping taint from node should return true",
			node: &corev1.Node{Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					taint1,
					taint2,
				}}},
			dropTaint:    taint1,
			wantTaints:   []corev1.Taint{taint2},
			wantModified: true,
		},
		{
			name: "drop non-existing taint should return false",
			node: &corev1.Node{Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					taint2,
				}}},
			dropTaint:    taint1,
			wantTaints:   []corev1.Taint{taint2},
			wantModified: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := RemoveNodeTaint(tt.node, tt.dropTaint)
			g.Expect(got).To(Equal(tt.wantModified))
			g.Expect(tt.node.Spec.Taints).To(Equal(tt.wantTaints))
		})
	}
}
