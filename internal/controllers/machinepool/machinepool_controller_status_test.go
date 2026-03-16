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

		setReplicas(mp, false, nil)

		g.Expect(mp.Status.ReadyReplicas).To(Equal(ptr.To[int32](2)))
		g.Expect(mp.Status.AvailableReplicas).To(Equal(ptr.To[int32](2)))
		g.Expect(mp.Status.UpToDateReplicas).To(Equal(ptr.To[int32](3)))
		g.Expect(mp.Status.Versions).To(BeNil())
	})

	t.Run("with MachinePool Machines versions are aggregated from kubelet versions", func(t *testing.T) {
		g := NewWithT(t)
		mp := &clusterv1.MachinePool{}
		machines := []*clusterv1.Machine{
			{
				Status: clusterv1.MachineStatus{
					NodeInfo: &corev1.NodeSystemInfo{
						KubeletVersion: "v1.31.1",
					},
				},
			},
			{
				Status: clusterv1.MachineStatus{
					NodeInfo: &corev1.NodeSystemInfo{
						KubeletVersion: "v1.31.1",
					},
				},
			},
			{
				Status: clusterv1.MachineStatus{
					NodeInfo: &corev1.NodeSystemInfo{
						KubeletVersion: "v1.32.0",
					},
				},
			},
		}

		setReplicas(mp, true, machines)

		g.Expect(mp.Status.Versions).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.31.1", Replicas: ptr.To[int32](2)},
			{Version: "v1.32.0", Replicas: ptr.To[int32](1)},
		}))
	})
}
