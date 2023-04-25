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

package alpha

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
)

func captureStdout(fnc func()) (string, error) {
	r, w, _ := os.Pipe()

	stdout := os.Stdout
	defer func() {
		os.Stdout = stdout
	}()
	os.Stdout = w

	fnc()
	_ = w.Close()

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func Test_ObjectStatusWatcher(t *testing.T) {
	namespace := metav1.NamespaceDefault
	clusterName := "test"
	mdName := "test-md-0"
	kcpName := "test-cp"

	type fields struct {
		objs []client.Object
		ref  corev1.ObjectReference
	}
	tests := []struct {
		name           string
		fields         fields
		expectedOutput string
		wantErr        bool
	}{
		{
			name: "watch machindeployment is successfully rolled out",
			fields: fields{
				objs: []client.Object{
					&clusterv1.MachineDeployment{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineDeployment",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      mdName,
						},
						Spec: clusterv1.MachineDeploymentSpec{
							ClusterName: clusterName,
							Replicas:    pointer.Int32(2),
						},
						Status: clusterv1.MachineDeploymentStatus{
							Replicas:          2,
							UpdatedReplicas:   2,
							AvailableReplicas: 2,
						},
					},
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      mdName,
					Namespace: namespace,
				},
			},
			expectedOutput: fmt.Sprintf(
				"machinedeployment %q successfully rolled out\n", mdName,
			),
		},
		{
			name: "watch kubeadmcontrolplane is successfully rolled out",
			fields: fields{
				objs: []client.Object{
					&controlplanev1.KubeadmControlPlane{
						TypeMeta: metav1.TypeMeta{
							Kind: "KubeadmControlPlane",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      kcpName,
						},
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							Replicas: pointer.Int32(2),
						},
						Status: controlplanev1.KubeadmControlPlaneStatus{
							Replicas:        2,
							UpdatedReplicas: 2,
							ReadyReplicas:   2,
						},
					},
				},
				ref: corev1.ObjectReference{
					Kind:      KubeadmControlPlane,
					Name:      kcpName,
					Namespace: namespace,
				},
			},
			expectedOutput: fmt.Sprintf(
				"kubeadmcontrolplane %q successfully rolled out\n", kcpName,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r := newRolloutClient()
			proxy := test.NewFakeProxy().WithObjs(tt.fields.objs...)
			output, err := captureStdout(func() {
				err := r.ObjectStatusWatcher(proxy, tt.fields.ref)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
				} else {
					g.Expect(err).ToNot(HaveOccurred())
				}
			})
			if err != nil {
				t.Fatalf("unable to captureStdout")
			}
			g.Expect(output).To(Equal(tt.expectedOutput))
		})
	}
}

func Test_MachineDeploymentStatusWatcherStatus(t *testing.T) {
	namespace := metav1.NamespaceDefault
	clusterName := "test"
	mdName := "test-md-0"
	statusViewer := &MachineDeploymentStatusWatcher{}

	tests := []struct {
		name           string
		md             *clusterv1.MachineDeployment
		expectedDone   bool
		expectedStatus string
		wantErr        bool
	}{
		{
			name: "new replicas are fewer than desired replicas",
			md: &clusterv1.MachineDeployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "MachineDeployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      mdName,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					ClusterName: clusterName,
					Replicas:    pointer.Int32(2),
				},
				Status: clusterv1.MachineDeploymentStatus{
					Replicas:          2,
					UpdatedReplicas:   1,
					AvailableReplicas: 0,
				},
			},
			expectedDone:   false,
			expectedStatus: fmt.Sprintf("Waiting for machinedeployment %q rollout to finish: %d out of %d new replicas have been updated...\n", mdName, 1, 2),
		},
		{
			name: "replicas are greater than desired replicas due to surge",
			md: &clusterv1.MachineDeployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "MachineDeployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      mdName,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					ClusterName: clusterName,
					Replicas:    pointer.Int32(2),
				},
				Status: clusterv1.MachineDeploymentStatus{
					Replicas:          3,
					UpdatedReplicas:   2,
					AvailableReplicas: 0,
				},
			},
			expectedDone:   false,
			expectedStatus: fmt.Sprintf("Waiting for machinedeployment %q rollout to finish: %d old replicas are pending termination...\n", mdName, 1),
		},
		{
			name: "available replicas are fewer than updated replicas",
			md: &clusterv1.MachineDeployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "MachineDeployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      mdName,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					ClusterName: clusterName,
					Replicas:    pointer.Int32(2),
				},
				Status: clusterv1.MachineDeploymentStatus{
					Replicas:          2,
					UpdatedReplicas:   2,
					AvailableReplicas: 0,
				},
			},
			expectedDone:   false,
			expectedStatus: fmt.Sprintf("Waiting for machinedeployment %q rollout to finish: %d of %d updated replicas are available...\n", mdName, 0, 2),
		},
		{
			name: "all replicas are available",
			md: &clusterv1.MachineDeployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "MachineDeployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      mdName,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					ClusterName: clusterName,
					Replicas:    pointer.Int32(2),
				},
				Status: clusterv1.MachineDeploymentStatus{
					Replicas:          2,
					UpdatedReplicas:   2,
					AvailableReplicas: 2,
				},
			},
			expectedDone:   true,
			expectedStatus: fmt.Sprintf("machinedeployment %q successfully rolled out\n", mdName),
		},
		{
			name:           "failed to get machinedeployment",
			expectedDone:   false,
			expectedStatus: "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			proxy := test.NewFakeProxy()
			if tt.md != nil {
				proxy = proxy.WithObjs(tt.md)
			}
			status, done, err := statusViewer.Status(proxy, mdName, namespace)
			g.Expect(done).Should(Equal(tt.expectedDone))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(status).To(Equal(tt.expectedStatus))
		})
	}
}

func Test_KubeadmControlPlaneStatusWatcherStatus(t *testing.T) {
	namespace := metav1.NamespaceDefault
	kcpName := "test-cp"
	statusViewer := &KubeadmControlPlaneStatusWatcher{}

	tests := []struct {
		name           string
		md             *controlplanev1.KubeadmControlPlane
		expectedDone   bool
		expectedStatus string
		wantErr        bool
	}{
		{
			name: "new replicas are fewer than desired replicas",
			md: &controlplanev1.KubeadmControlPlane{
				TypeMeta: metav1.TypeMeta{
					Kind: "KubeadmControlPlane",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      kcpName,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: pointer.Int32(2),
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Replicas:        2,
					UpdatedReplicas: 1,
					ReadyReplicas:   0,
				},
			},
			expectedDone:   false,
			expectedStatus: fmt.Sprintf("Waiting for kubeadmcontrolplane %q rollout to finish: %d out of %d new replicas have been updated...\n", kcpName, 1, 2),
		},
		{
			name: "replicas are greater than desired replicas due to surge",
			md: &controlplanev1.KubeadmControlPlane{
				TypeMeta: metav1.TypeMeta{
					Kind: "KubeadmControlPlane",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      kcpName,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: pointer.Int32(2),
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Replicas:        3,
					UpdatedReplicas: 2,
					ReadyReplicas:   0,
				},
			},
			expectedDone:   false,
			expectedStatus: fmt.Sprintf("Waiting for kubeadmcontrolplane %q rollout to finish: %d old replicas are pending termination...\n", kcpName, 1),
		},
		{
			name: "ready replicas are fewer than updated replicas",
			md: &controlplanev1.KubeadmControlPlane{
				TypeMeta: metav1.TypeMeta{
					Kind: "KubeadmControlPlane",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      kcpName,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: pointer.Int32(2),
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Replicas:        2,
					UpdatedReplicas: 2,
					ReadyReplicas:   0,
				},
			},
			expectedDone:   false,
			expectedStatus: fmt.Sprintf("Waiting for kubeadmcontrolplane %q rollout to finish: %d of %d updated replicas are ready...\n", kcpName, 0, 2),
		},
		{
			name: "all replicas are available",
			md: &controlplanev1.KubeadmControlPlane{
				TypeMeta: metav1.TypeMeta{
					Kind: "KubeadmControlPlane",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      kcpName,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: pointer.Int32(2),
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Replicas:        2,
					UpdatedReplicas: 2,
					ReadyReplicas:   2,
				},
			},
			expectedDone:   true,
			expectedStatus: fmt.Sprintf("kubeadmcontrolplane %q successfully rolled out\n", kcpName),
		},
		{
			name:           "failed to get kubeadmcontrolplane",
			expectedDone:   false,
			expectedStatus: "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			proxy := test.NewFakeProxy()
			if tt.md != nil {
				proxy = proxy.WithObjs(tt.md)
			}
			status, done, err := statusViewer.Status(proxy, kcpName, namespace)
			g.Expect(done).Should(Equal(tt.expectedDone))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(status).To(Equal(tt.expectedStatus))
		})
	}
}
