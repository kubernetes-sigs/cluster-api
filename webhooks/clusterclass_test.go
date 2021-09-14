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

package webhooks

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/component-base/featuregate/testing"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clusterv1.AddToScheme(fakeScheme)
}

func TestClusterClassDefaultNamespaces(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	namespace := "default"
	ref := &corev1.ObjectReference{
		APIVersion: "foo",
		Kind:       "barTemplate",
		Name:       "baz",
	}
	in := &clusterv1.ClusterClass{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: clusterv1.ClusterClassSpec{
			Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
			ControlPlane: clusterv1.ControlPlaneClass{
				LocalObjectTemplate:   clusterv1.LocalObjectTemplate{Ref: ref},
				MachineInfrastructure: &clusterv1.LocalObjectTemplate{Ref: ref},
			},
			Workers: clusterv1.WorkersClass{
				MachineDeployments: []clusterv1.MachineDeploymentClass{
					{
						Class: "aa",
						Template: clusterv1.MachineDeploymentClassTemplate{
							Bootstrap:      clusterv1.LocalObjectTemplate{Ref: ref},
							Infrastructure: clusterv1.LocalObjectTemplate{Ref: ref},
						},
					},
				},
			},
		},
	}

	webhook := &ClusterClass{}

	t.Run("for ClusterClass", customDefaultValidateTest(ctx, in, webhook))

	g := NewWithT(t)
	g.Expect(webhook.Default(ctx, in)).To(Succeed())

	// Namespace defaulted on references
	g.Expect(in.Spec.Infrastructure.Ref.Namespace).To(Equal(namespace))
	g.Expect(in.Spec.ControlPlane.Ref.Namespace).To(Equal(namespace))
	g.Expect(in.Spec.ControlPlane.MachineInfrastructure.Ref.Namespace).To(Equal(namespace))
	for i := range in.Spec.Workers.MachineDeployments {
		g.Expect(in.Spec.Workers.MachineDeployments[i].Template.Bootstrap.Ref.Namespace).To(Equal(namespace))
		g.Expect(in.Spec.Workers.MachineDeployments[i].Template.Infrastructure.Ref.Namespace).To(Equal(namespace))
	}
}
