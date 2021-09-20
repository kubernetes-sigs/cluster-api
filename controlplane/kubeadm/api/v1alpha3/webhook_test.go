/*
Copyright 2020 The Kubernetes Authors.

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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta1"
	"sigs.k8s.io/cluster-api/util"
)

func TestKubeadmControlPlaneConversion(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, fmt.Sprintf("conversion-webhook-%s", util.RandomString(5)))
	g.Expect(err).ToNot(HaveOccurred())
	infraMachineTemplateName := fmt.Sprintf("test-machinetemplate-%s", util.RandomString(5))
	controlPlaneName := fmt.Sprintf("test-controlpane-%s", util.RandomString(5))
	controlPlane := &KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controlPlaneName,
			Namespace: ns.Name,
		},
		Spec: KubeadmControlPlaneSpec{
			Replicas: pointer.Int32(3),
			Version:  "v1.20.2",
			InfrastructureTemplate: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "TestMachineTemplate",
				Namespace:  ns.Name,
				Name:       infraMachineTemplateName,
			},
			KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
				ClusterConfiguration: &upstreamv1beta1.ClusterConfiguration{
					APIServer: upstreamv1beta1.APIServer{
						ControlPlaneComponent: upstreamv1beta1.ControlPlaneComponent{
							ExtraArgs: map[string]string{
								"foo": "bar",
							},
							ExtraVolumes: []upstreamv1beta1.HostPathMount{
								{
									Name:      "mount-path",
									HostPath:  "/foo",
									MountPath: "/foo",
									ReadOnly:  false,
								},
							},
						},
					},
				},
				InitConfiguration: &upstreamv1beta1.InitConfiguration{
					NodeRegistration: upstreamv1beta1.NodeRegistrationOptions{
						Name:      "foo",
						CRISocket: "/var/run/containerd/containerd.sock",
					},
				},
				JoinConfiguration: &upstreamv1beta1.JoinConfiguration{
					NodeRegistration: upstreamv1beta1.NodeRegistrationOptions{
						Name:      "foo",
						CRISocket: "/var/run/containerd/containerd.sock",
					},
				},
			},
		},
	}

	g.Expect(env.Create(ctx, controlPlane)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, controlPlane)
}
