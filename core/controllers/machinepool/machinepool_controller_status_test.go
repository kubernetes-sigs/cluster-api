/*
Copyright 2026 The Kubernetes Authors.

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

package machinepool

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func Test_setReplicas(t *testing.T) {
	t.Run("without MachinePool Machines versions are not surfaced", func(t *testing.T) {
		g := NewWithT(t)
		mp := &clusterv1.MachinePool{
			Spec: clusterv1.MachinePoolSpec{
				Replicas: ptr.To[int32](3),
			},
			Status: clusterv1.MachinePoolStatus{
				Replicas: ptr.To[int32](2),
			},
		}

		setReplicas(mp, false, nil, nil)

		g.Expect(mp.Status.ReadyReplicas).To(Equal(ptr.To[int32](2)))
		g.Expect(mp.Status.AvailableReplicas).To(Equal(ptr.To[int32](2)))
		g.Expect(mp.Status.UpToDateReplicas).To(Equal(ptr.To[int32](3)))
		g.Expect(mp.Status.Versions).To(BeNil())
	})

	t.Run("without MachinePool Machines versions are aggregated from node refs", func(t *testing.T) {
		g := NewWithT(t)
		mp := &clusterv1.MachinePool{
			Status: clusterv1.MachinePoolStatus{
				NodeRefs: []corev1.ObjectReference{
					{Name: "node-1"},
					{Name: "node-2"},
					{Name: "node-3"},
				},
			},
		}
		nodeRefMap := map[string]*corev1.Node{
			"provider-id-1": {
				ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
				Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.1"}},
			},
			"provider-id-2": {
				ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
				Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.1"}},
			},
			"provider-id-3": {
				ObjectMeta: metav1.ObjectMeta{Name: "node-3"},
				Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.32.0"}},
			},
		}

		setReplicas(mp, false, nil, nodeRefMap)

		g.Expect(mp.Status.Versions).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.31.1", Replicas: 2},
			{Version: "v1.32.0", Replicas: 1},
		}))
	})

	t.Run("with MachinePool Machines versions are aggregated from machine spec versions", func(t *testing.T) {
		g := NewWithT(t)
		mp := &clusterv1.MachinePool{}
		machines := []*clusterv1.Machine{
			{
				Spec: clusterv1.MachineSpec{
					Version: "v1.31.1",
				},
			},
			{
				Spec: clusterv1.MachineSpec{
					Version: "v1.31.1",
				},
			},
			{
				Spec: clusterv1.MachineSpec{
					Version: "v1.32.0",
				},
			},
		}

		setReplicas(mp, true, machines, nil)

		g.Expect(mp.Status.Versions).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.31.1", Replicas: 2},
			{Version: "v1.32.0", Replicas: 1},
		}))
	})
}
