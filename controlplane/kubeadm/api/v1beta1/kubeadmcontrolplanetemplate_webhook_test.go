/*
Copyright 2021 The Kubernetes Authors.

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

package v1beta1

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"
)

func TestKubeadmControlPlaneTemplateDefault(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	g := NewWithT(t)

	kcpTemplate := &KubeadmControlPlaneTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: KubeadmControlPlaneTemplateSpec{
			Template: KubeadmControlPlaneTemplateResource{
				Spec: KubeadmControlPlaneTemplateResourceSpec{
					MachineTemplate: &KubeadmControlPlaneTemplateMachineTemplate{
						NodeDrainTimeout: &metav1.Duration{Duration: 10 * time.Second},
					},
				},
			},
		},
	}
	updateDefaultingValidationKCPTemplate := kcpTemplate.DeepCopy()
	updateDefaultingValidationKCPTemplate.Spec.Template.Spec.MachineTemplate.NodeDrainTimeout = &metav1.Duration{Duration: 20 * time.Second}
	t.Run("for KubeadmControlPlaneTemplate", utildefaulting.DefaultValidateTest(updateDefaultingValidationKCPTemplate))
	kcpTemplate.Default()

	g.Expect(kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.Format).To(Equal(bootstrapv1.CloudConfig))
	g.Expect(kcpTemplate.Spec.Template.Spec.RolloutStrategy.Type).To(Equal(RollingUpdateStrategyType))
	g.Expect(kcpTemplate.Spec.Template.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal).To(Equal(int32(1)))
}

func TestKubeadmControlPlaneTemplateValidationFeatureGateEnabled(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	t.Run("create kubeadmcontrolplanetemplate should pass if gate enabled and valid kubeadmcontrolplanetemplate", func(t *testing.T) {
		testnamespace := "test"
		g := NewWithT(t)
		kcpTemplate := &KubeadmControlPlaneTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubeadmcontrolplanetemplate-test",
				Namespace: testnamespace,
			},
			Spec: KubeadmControlPlaneTemplateSpec{
				Template: KubeadmControlPlaneTemplateResource{
					Spec: KubeadmControlPlaneTemplateResourceSpec{
						MachineTemplate: &KubeadmControlPlaneTemplateMachineTemplate{
							NodeDrainTimeout: &metav1.Duration{Duration: time.Second},
						},
					},
				},
			},
		}
		g.Expect(kcpTemplate.ValidateCreate()).To(Succeed())
	})
}

func TestKubeadmControlPlaneTemplateValidationFeatureGateDisabled(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create KubeadmControlPlaneTemplate.
	t.Run("create kubeadmcontrolplanetemplate should not pass if gate disabled and valid kubeadmcontrolplanetemplate", func(t *testing.T) {
		testnamespace := "test"
		g := NewWithT(t)
		kcpTemplate := &KubeadmControlPlaneTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubeadmcontrolplanetemplate-test",
				Namespace: testnamespace,
			},
			Spec: KubeadmControlPlaneTemplateSpec{
				Template: KubeadmControlPlaneTemplateResource{
					Spec: KubeadmControlPlaneTemplateResourceSpec{
						MachineTemplate: &KubeadmControlPlaneTemplateMachineTemplate{
							NodeDrainTimeout: &metav1.Duration{Duration: time.Second},
						},
					},
				},
			},
		}
		g.Expect(kcpTemplate.ValidateCreate()).NotTo(Succeed())
	})
}
