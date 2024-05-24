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

package alpha

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
)

func Test_ObjectResumer(t *testing.T) {
	type fields struct {
		objs []client.Object
		ref  corev1.ObjectReference
	}
	tests := []struct {
		name       string
		fields     fields
		wantErr    bool
		wantPaused bool
	}{
		{
			name: "paused machinedeployment should be unpaused",
			fields: fields{
				objs: []client.Object{
					&clusterv1.MachineDeployment{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineDeployment",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "md-1",
						},
						Spec: clusterv1.MachineDeploymentSpec{
							Paused: true,
						},
					},
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      "md-1",
					Namespace: "default",
				},
			},
			wantErr:    false,
			wantPaused: false,
		},
		{
			name: "unpausing an already unpaused machinedeployment should return error",
			fields: fields{
				objs: []client.Object{
					&clusterv1.MachineDeployment{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineDeployment",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "md-1",
						},
						Spec: clusterv1.MachineDeploymentSpec{
							Paused: false,
						},
					},
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      "md-1",
					Namespace: "default",
				},
			},
			wantErr:    true,
			wantPaused: false,
		},
		{
			name: "paused kubeadmcontrolplane should be unpaused",
			fields: fields{
				objs: []client.Object{
					&controlplanev1.KubeadmControlPlane{
						TypeMeta: metav1.TypeMeta{
							Kind: "KubeadmControlPlane",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "kcp",
							Annotations: map[string]string{
								clusterv1.PausedAnnotation: "true",
							},
						},
					},
				},
				ref: corev1.ObjectReference{
					Kind:      KubeadmControlPlane,
					Name:      "kcp",
					Namespace: "default",
				},
			},
			wantErr:    false,
			wantPaused: false,
		},
		{
			name: "unpausing an already unpaused kubeadmcontrolplane should return error",
			fields: fields{
				objs: []client.Object{
					&controlplanev1.KubeadmControlPlane{
						TypeMeta: metav1.TypeMeta{
							Kind: "KubeadmControlPlane",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "kcp",
						},
					},
				},
				ref: corev1.ObjectReference{
					Kind:      KubeadmControlPlane,
					Name:      "kcp",
					Namespace: "default",
				},
			},
			wantErr:    true,
			wantPaused: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r := newRolloutClient()
			proxy := test.NewFakeProxy().WithObjs(tt.fields.objs...)
			err := r.ObjectResumer(context.Background(), proxy, tt.fields.ref)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			for _, obj := range tt.fields.objs {
				cl, err := proxy.NewClient(context.Background())
				g.Expect(err).ToNot(HaveOccurred())
				key := client.ObjectKeyFromObject(obj)
				switch obj.(type) {
				case *clusterv1.MachineDeployment:
					md := &clusterv1.MachineDeployment{}
					err = cl.Get(context.TODO(), key, md)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(md.Spec.Paused).To(Equal(tt.wantPaused))
				case *controlplanev1.KubeadmControlPlane:
					kcp := &controlplanev1.KubeadmControlPlane{}
					err = cl.Get(context.TODO(), key, kcp)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(annotations.HasPaused(kcp.GetObjectMeta())).To(Equal(tt.wantPaused))
				}
			}
		})
	}
}
