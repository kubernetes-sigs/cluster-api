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

package util

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_inspectImages(t *testing.T) {
	type args struct {
		objs []unstructured.Unstructured
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "controller without the RBAC proxy",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       deploymentKind,
							"spec": map[string]interface{}{
								"template": map[string]interface{}{
									"spec": map[string]interface{}{
										"containers": []map[string]interface{}{
											{
												"name":  controllerContainerName,
												"image": "gcr.io/k8s-staging-cluster-api/cluster-api-controller:master",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    []string{"gcr.io/k8s-staging-cluster-api/cluster-api-controller:master"},
			wantErr: false,
		},
		{
			name: "controller with the RBAC proxy",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       deploymentKind,
							"spec": map[string]interface{}{
								"template": map[string]interface{}{
									"spec": map[string]interface{}{
										"containers": []map[string]interface{}{
											{
												"name":  controllerContainerName,
												"image": "gcr.io/k8s-staging-cluster-api/cluster-api-controller:master",
											},
											{
												"name":  "kube-rbac-proxy",
												"image": "gcr.io/kubebuilder/kube-rbac-proxy:v0.4.1",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    []string{"gcr.io/k8s-staging-cluster-api/cluster-api-controller:master", "gcr.io/kubebuilder/kube-rbac-proxy:v0.4.1"},
			wantErr: false,
		},
		{
			name: "controller with init container",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       deploymentKind,
							"spec": map[string]interface{}{
								"template": map[string]interface{}{
									"spec": map[string]interface{}{
										"containers": []map[string]interface{}{
											{
												"name":  controllerContainerName,
												"image": "gcr.io/k8s-staging-cluster-api/cluster-api-controller:master",
											},
										},
										"initContainers": []map[string]interface{}{
											{
												"name":  controllerContainerName,
												"image": "gcr.io/k8s-staging-cluster-api/cluster-api-controller:init",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want:    []string{"gcr.io/k8s-staging-cluster-api/cluster-api-controller:master", "gcr.io/k8s-staging-cluster-api/cluster-api-controller:init"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := InspectImages(tt.args.objs)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestFixImages(t *testing.T) {
	type args struct {
		objs           []unstructured.Unstructured
		alterImageFunc func(image string) (string, error)
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "fix deployment containers images",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       deploymentKind,
							"spec": map[string]interface{}{
								"template": map[string]interface{}{
									"spec": map[string]interface{}{
										"containers": []map[string]interface{}{
											{
												"image": "container-image",
											},
										},
										"initContainers": []map[string]interface{}{
											{
												"image": "init-container-image",
											},
										},
									},
								},
							},
						},
					},
				},
				alterImageFunc: func(image string) (string, error) {
					return fmt.Sprintf("foo-%s", image), nil
				},
			},
			want:    []string{"foo-container-image", "foo-init-container-image"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := FixImages(tt.args.objs, tt.args.alterImageFunc)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			gotImages, err := InspectImages(got)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(gotImages).To(Equal(tt.want))
		})
	}
}
