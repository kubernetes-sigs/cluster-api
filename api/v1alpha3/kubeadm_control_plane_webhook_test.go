/*
Copyright 2019 The Kubernetes Authors.

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
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
)

func TestKubeadmControlPlaneDefault(t *testing.T) {
	g := gomega.NewWithT(t)

	kcp := &KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: KubeadmControlPlaneSpec{
			InfrastructureTemplate: corev1.ObjectReference{},
		},
	}
	kcp.Default()

	g.Expect(kcp.Spec.InfrastructureTemplate.Namespace).To(gomega.Equal(kcp.Namespace))
}

func TestKubeadmControlPlaneValidateCreate(t *testing.T) {
	valid := &KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
		Spec: KubeadmControlPlaneSpec{
			InfrastructureTemplate: corev1.ObjectReference{
				Namespace: "foo",
				Name:      "infraTemplate",
			},
			Replicas: pointer.Int32Ptr(1),
		},
	}
	invalidNamespace := valid.DeepCopy()
	invalidNamespace.Spec.InfrastructureTemplate.Namespace = "bar"

	missingReplicas := valid.DeepCopy()
	missingReplicas.Spec.Replicas = nil

	zeroReplicas := valid.DeepCopy()
	zeroReplicas.Spec.Replicas = pointer.Int32Ptr(0)

	evenReplicas := valid.DeepCopy()
	evenReplicas.Spec.Replicas = pointer.Int32Ptr(2)

	evenReplicasExternalEtcd := evenReplicas.DeepCopy()
	evenReplicasExternalEtcd.Spec.KubeadmConfigSpec = bootstrapv1.KubeadmConfigSpec{
		InitConfiguration: &kubeadmv1beta1.InitConfiguration{
			ClusterConfiguration: kubeadmv1beta1.ClusterConfiguration{
				Etcd: kubeadmv1beta1.Etcd{
					External: &kubeadmv1beta1.ExternalEtcd{},
				},
			},
		},
	}

	tests := []struct {
		name      string
		expectErr bool
		kcp       *KubeadmControlPlane
	}{
		{
			name:      "should succeed when given a valid config",
			expectErr: false,
			kcp:       valid,
		},
		{
			name:      "should return error when kubeadmControlPlane namespace and infrastructureTemplate  namespace mismatch",
			expectErr: true,
			kcp:       invalidNamespace,
		},
		{
			name:      "should return error when replicas is nil",
			expectErr: true,
			kcp:       missingReplicas,
		},
		{
			name:      "should return error when replicas is zero",
			expectErr: true,
			kcp:       zeroReplicas,
		},
		{
			name:      "should return error when replicas is even",
			expectErr: true,
			kcp:       evenReplicas,
		},
		{
			name:      "should allow even replicas when using external etcd",
			expectErr: false,
			kcp:       evenReplicasExternalEtcd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			if tt.expectErr {
				err := tt.kcp.ValidateCreate()
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				err := tt.kcp.ValidateCreate()
				g.Expect(err).To(gomega.Succeed())
			}
		})
	}
}

func TestKubeadmControlPlaneValidateUpdate(t *testing.T) {
	before := &KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
		Spec: KubeadmControlPlaneSpec{
			InfrastructureTemplate: corev1.ObjectReference{
				Namespace: "foo",
				Name:      "infraTemplate",
			},
			Replicas:          pointer.Int32Ptr(1),
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{},
		},
	}

	invalidUpdate := before.DeepCopy()
	invalidUpdate.Spec.KubeadmConfigSpec.InitConfiguration = &kubeadmv1beta1.InitConfiguration{}

	validUpdate := before.DeepCopy()
	validUpdate.Labels = map[string]string{"blue": "green"}
	validUpdate.Spec.InfrastructureTemplate.Name = "orange"
	validUpdate.Spec.Replicas = pointer.Int32Ptr(5)

	tests := []struct {
		name      string
		expectErr bool
		kcp       *KubeadmControlPlane
	}{
		{
			name:      "should succeed when given a valid config",
			expectErr: false,
			kcp:       validUpdate,
		},
		{
			name:      "should return error when trying to mutate the kubeadmconfigspec",
			expectErr: true,
			kcp:       invalidUpdate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			if tt.expectErr {
				err := tt.kcp.ValidateUpdate(before.DeepCopy())
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				err := tt.kcp.ValidateUpdate(before.DeepCopy())
				g.Expect(err).To(gomega.Succeed())
			}
		})
	}
}
