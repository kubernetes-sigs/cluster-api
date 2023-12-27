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
	"context"
	"io"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
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

func Test_ObjectViewer(t *testing.T) {
	namespace := "default"
	clusterName := "test"
	labels := map[string]string{
		clusterv1.ClusterNameLabel:           clusterName,
		clusterv1.MachineDeploymentNameLabel: "test-md-0",
	}
	deployment := &clusterv1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineDeployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "test-md-0",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName: clusterName,
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	type fields struct {
		objs     []client.Object
		ref      corev1.ObjectReference
		revision int64
	}
	tests := []struct {
		name           string
		fields         fields
		expectedOutput string
		wantErr        bool
	}{

		{
			name: "should print an overview of all revisions",
			fields: fields{
				objs: []client.Object{
					deployment,
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-1",
							Namespace: namespace,
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: labels,
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation:        "11",
								clusterv1.RevisionHistoryAnnotation: "1,3,5,7,9",
								clusterv1.ChangeCauseAnnotation:     "update to the latest version",
							},
						},
					},
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-3",
							Namespace: namespace,
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: labels,
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation:        "10",
								clusterv1.RevisionHistoryAnnotation: "4,6,8",
							},
						},
					},
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-2",
							Namespace: namespace,
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: labels,
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "2",
							},
						},
					},
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      deployment.Name,
					Namespace: namespace,
				},
			},
			expectedOutput: `  REVISIONS     CHANGE-CAUSE                  
  2             <none>                        
  4,6,8,10      <none>                        
  1,3,5,7,9,11  update to the latest version  
`,
		},
		{
			name: "should print an overview of all revisions even if there is no machineSets",
			fields: fields{
				objs: []client.Object{
					deployment,
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      deployment.Name,
					Namespace: namespace,
				},
			},
			expectedOutput: `  REVISIONS  CHANGE-CAUSE  
`,
		},
		{
			name: "should print the details of revision=999",
			fields: fields{
				objs: []client.Object{
					deployment,
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ms-rev-2",
							Namespace: namespace,
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: labels,
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "2",
							},
						},
					},
					&clusterv1.MachineSet{
						TypeMeta: metav1.TypeMeta{
							Kind: "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      "ms-rev-999",
							OwnerReferences: []metav1.OwnerReference{
								*metav1.NewControllerRef(deployment, clusterv1.GroupVersion.WithKind("MachineDeployment")),
							},
							Labels: labels,
							Annotations: map[string]string{
								clusterv1.RevisionAnnotation: "999",
							},
						},
						Spec: clusterv1.MachineSetSpec{
							ClusterName: clusterName,
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									clusterv1.ClusterNameLabel: clusterName,
								},
							},
							Template: clusterv1.MachineTemplateSpec{
								ObjectMeta: clusterv1.ObjectMeta{
									Labels: map[string]string{
										clusterv1.ClusterNameLabel: clusterName,
									},
									Annotations: map[string]string{"foo": "bar"},
								},
								Spec: clusterv1.MachineSpec{
									ClusterName: clusterName,
									Bootstrap: clusterv1.Bootstrap{
										ConfigRef: &corev1.ObjectReference{
											APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
											Kind:       "KubeadmConfigTemplate",
											Name:       "md-template",
											Namespace:  namespace,
										},
										DataSecretName: ptr.To[string]("secret-name"),
									},
									InfrastructureRef: corev1.ObjectReference{
										APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
										Kind:       "InfrastructureMachineTemplate",
										Name:       "md-template",
										Namespace:  namespace,
									},
									Version:                 ptr.To[string]("v1.25.1"),
									ProviderID:              ptr.To[string]("test://id-1"),
									FailureDomain:           ptr.To[string]("one"),
									NodeDrainTimeout:        &metav1.Duration{Duration: 0},
									NodeVolumeDetachTimeout: &metav1.Duration{Duration: 0},
									NodeDeletionTimeout:     &metav1.Duration{Duration: 0},
								},
							},
						},
					},
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      deployment.Name,
					Namespace: namespace,
				},
				revision: int64(999),
			},
			expectedOutput: `objectmeta:
    labels:
        cluster.x-k8s.io/cluster-name: test
    annotations:
        foo: bar
spec:
    clustername: test
    bootstrap:
        configref:
            kind: KubeadmConfigTemplate
            namespace: default
            name: md-template
            uid: ""
            apiversion: bootstrap.cluster.x-k8s.io/v1beta1
            resourceversion: ""
            fieldpath: ""
        datasecretname: secret-name
    infrastructureref:
        kind: InfrastructureMachineTemplate
        namespace: default
        name: md-template
        uid: ""
        apiversion: infrastructure.cluster.x-k8s.io/v1beta1
        resourceversion: ""
        fieldpath: ""
    version: v1.25.1
    providerid: test://id-1
    failuredomain: one
    nodedraintimeout:
        duration: 0s
    nodevolumedetachtimeout:
        duration: 0s
    nodedeletiontimeout:
        duration: 0s
`,
		},
		{
			name: "should print an error for non-existent revision",
			fields: fields{
				objs: []client.Object{
					deployment,
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      deployment.Name,
					Namespace: namespace,
				},
				revision: int64(999),
			},
			wantErr: true,
		},
		{
			name: "should print an error for an invalid revision",
			fields: fields{
				objs: []client.Object{
					deployment,
				},
				ref: corev1.ObjectReference{
					Kind:      MachineDeployment,
					Name:      deployment.Name,
					Namespace: namespace,
				},
				revision: int64(-1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r := newRolloutClient()
			proxy := test.NewFakeProxy().WithObjs(tt.fields.objs...)
			output, err := captureStdout(func() {
				err := r.ObjectViewer(context.Background(), proxy, tt.fields.ref, tt.fields.revision)
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
