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

package conversion

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestConvertMachinePoolPreservesRemediation(t *testing.T) {
	g := NewWithT(t)

	hub := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mp",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachinePoolSpec{
			ClusterName: "cluster",
			Remediation: clusterv1.MachinePoolRemediationSpec{
				MaxInFlight: ptr.To(intstr.FromString("20%")),
			},
		},
	}

	spoke := &clusterv1beta1.MachinePool{}
	g.Expect(ConvertMachinePoolHubToV1Beta1(t.Context(), hub.DeepCopy(), spoke)).To(Succeed())

	restored := &clusterv1.MachinePool{}
	g.Expect(ConvertMachinePoolV1Beta1ToHub(t.Context(), spoke, restored)).To(Succeed())
	g.Expect(restored.Spec.Remediation).To(Equal(hub.Spec.Remediation))
}
