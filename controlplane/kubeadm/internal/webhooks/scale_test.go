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

package webhooks

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
)

func init() {
	scheme = runtime.NewScheme()
	_ = controlplanev1.AddToScheme(scheme)
	_ = admissionv1.AddToScheme(scheme)
}

var (
	scheme *runtime.Scheme
)

func TestKubeadmControlPlaneValidateScale(t *testing.T) {
	kcpManagedEtcd := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kcp-managed-etcd",
			Namespace: "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Namespace:  "foo",
					Name:       "infraTemplate",
				},
				NodeDrainTimeout: &metav1.Duration{Duration: time.Second},
			},
			Replicas: pointer.Int32Ptr(1),
			RolloutStrategy: &controlplanev1.RolloutStrategy{
				Type: controlplanev1.RollingUpdateStrategyType,
				RollingUpdate: &controlplanev1.RollingUpdate{
					MaxSurge: &intstr.IntOrString{
						IntVal: 1,
					},
				},
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				InitConfiguration: &bootstrapv1.InitConfiguration{
					LocalAPIEndpoint: bootstrapv1.APIEndpoint{
						AdvertiseAddress: "127.0.0.1",
						BindPort:         int32(443),
					},
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						Name: "kcp-managed-etcd",
					},
				},
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					ClusterName: "kcp-managed-etcd",
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageRepository: "k8s.gcr.io/coredns",
							ImageTag:        "1.6.5",
						},
					},
				},
				JoinConfiguration: &bootstrapv1.JoinConfiguration{
					Discovery: bootstrapv1.Discovery{
						Timeout: &metav1.Duration{
							Duration: 10 * time.Minute,
						},
					},
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						Name: "kcp-managed-etcd",
					},
				},
				PreKubeadmCommands: []string{
					"kcp-managed-etcd", "foo",
				},
				PostKubeadmCommands: []string{
					"kcp-managed-etcd", "foo",
				},
				Files: []bootstrapv1.File{
					{
						Path: "kcp-managed-etcd",
					},
				},
				Users: []bootstrapv1.User{
					{
						Name: "user",
						SSHAuthorizedKeys: []string{
							"ssh-rsa foo",
						},
					},
				},
				NTP: &bootstrapv1.NTP{
					Servers: []string{"test-server-1", "test-server-2"},
					Enabled: pointer.BoolPtr(true),
				},
			},
			Version: "v1.16.6",
		},
	}

	kcpExternalEtcd := kcpManagedEtcd.DeepCopy()
	kcpExternalEtcd.ObjectMeta.Name = "kcp-external-etcd"
	kcpExternalEtcd.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External = &bootstrapv1.ExternalEtcd{}

	tests := []struct {
		name              string
		admissionRequest  admission.Request
		expectRespAllowed bool
		expectRespReason  string
	}{
		{
			name:              "should return error when trying to scale to zero",
			expectRespAllowed: false,
			expectRespReason:  "replicas cannot be 0",
			admissionRequest: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       uuid.NewUUID(),
				Kind:      metav1.GroupVersionKind{Group: "autoscaling", Version: "v1", Kind: "Scale"},
				Operation: admissionv1.Update,
				Object:    runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"kcp-managed-etcd","namespace":"foo"},"spec":{"replicas":0}}`)},
			}},
		},
		{
			name:              "should return error when trying to scale to even number of replicas with managed etcd",
			expectRespAllowed: false,
			expectRespReason:  "replicas cannot be an even number when etcd is stacked",
			admissionRequest: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       uuid.NewUUID(),
				Kind:      metav1.GroupVersionKind{Group: "autoscaling", Version: "v1", Kind: "Scale"},
				Operation: admissionv1.Update,
				Object:    runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"kcp-managed-etcd","namespace":"foo"},"spec":{"replicas":2}}`)},
			}},
		},
		{
			name:              "should allow odd number of replicas with managed etcd",
			expectRespAllowed: true,
			expectRespReason:  "",
			admissionRequest: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       uuid.NewUUID(),
				Kind:      metav1.GroupVersionKind{Group: "autoscaling", Version: "v1", Kind: "Scale"},
				Operation: admissionv1.Update,
				Object:    runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"kcp-managed-etcd","namespace":"foo"},"spec":{"replicas":3}}`)},
			}},
		},
		{
			name:              "should allow even number of replicas with external etcd",
			expectRespAllowed: true,
			expectRespReason:  "",
			admissionRequest: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       uuid.NewUUID(),
				Kind:      metav1.GroupVersionKind{Group: "autoscaling", Version: "v1", Kind: "Scale"},
				Operation: admissionv1.Update,
				Object:    runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"kcp-external-etcd","namespace":"foo"},"spec":{"replicas":4}}`)},
			}},
		},
		{
			name:              "should allow odd number of replicas with external etcd",
			expectRespAllowed: true,
			expectRespReason:  "",
			admissionRequest: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       uuid.NewUUID(),
				Kind:      metav1.GroupVersionKind{Group: "autoscaling", Version: "v1", Kind: "Scale"},
				Operation: admissionv1.Update,
				Object:    runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"kcp-external-etcd","namespace":"foo"},"spec":{"replicas":3}}`)},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			decoder, _ := admission.NewDecoder(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kcpManagedEtcd, kcpExternalEtcd).Build()

			// Create the webhook and add the fakeClient as its client.
			scaleHandler := ScaleValidator{
				Client:  fakeClient,
				decoder: decoder,
			}

			resp := scaleHandler.Handle(context.Background(), tt.admissionRequest)
			g.Expect(resp.Allowed).Should(Equal(tt.expectRespAllowed))
			g.Expect(string(resp.Result.Reason)).Should(Equal(tt.expectRespReason))
		})
	}
}
