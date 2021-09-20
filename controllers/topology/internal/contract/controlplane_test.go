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

package contract

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestControlPlane(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

	t.Run("Manages spec.version", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().Version().Path()).To(Equal(Path{"spec", "version"}))

		err := ControlPlane().Version().Set(obj, "vFoo")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().Version().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("vFoo"))
	})
	t.Run("Manages status.version", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().StatusVersion().Path()).To(Equal(Path{"status", "version"}))

		err := ControlPlane().StatusVersion().Set(obj, "1.2.3")
		g.Expect(err).NotTo(HaveOccurred())

		got, err := ControlPlane().StatusVersion().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("1.2.3"))
	})
	t.Run("Manages spec.replicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().Replicas().Path()).To(Equal(Path{"spec", "replicas"}))

		err := ControlPlane().Replicas().Set(obj, int64(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().Replicas().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(3)))
	})
	t.Run("Manages status.replicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().StatusReplicas().Path()).To(Equal(Path{"status", "replicas"}))

		err := ControlPlane().StatusReplicas().Set(obj, int64(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().StatusReplicas().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(3)))
	})
	t.Run("Manages status.updatedreplicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().UpdatedReplicas().Path()).To(Equal(Path{"status", "updatedReplicas"}))

		err := ControlPlane().UpdatedReplicas().Set(obj, int64(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().UpdatedReplicas().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(3)))
	})
	t.Run("Manages status.readyReplicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().ReadyReplicas().Path()).To(Equal(Path{"status", "readyReplicas"}))

		err := ControlPlane().ReadyReplicas().Set(obj, int64(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().ReadyReplicas().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(3)))
	})
	t.Run("Manages spec.machineTemplate.infrastructureRef", func(t *testing.T) {
		g := NewWithT(t)

		refObj := fooRefBuilder()

		g.Expect(ControlPlane().MachineTemplate().InfrastructureRef().Path()).To(Equal(Path{"spec", "machineTemplate", "infrastructureRef"}))

		err := ControlPlane().MachineTemplate().InfrastructureRef().Set(obj, refObj)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().InfrastructureRef().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.APIVersion).To(Equal(refObj.GetAPIVersion()))
		g.Expect(got.Kind).To(Equal(refObj.GetKind()))
		g.Expect(got.Name).To(Equal(refObj.GetName()))
		g.Expect(got.Namespace).To(Equal(refObj.GetNamespace()))
	})
	t.Run("Manages spec.machineTemplate.metadata", func(t *testing.T) {
		g := NewWithT(t)

		metadata := &clusterv1.ObjectMeta{
			Labels: map[string]string{
				"label1": "labelValue1",
			},
			Annotations: map[string]string{
				"annotation1": "annotationValue1",
			},
		}

		g.Expect(ControlPlane().MachineTemplate().Metadata().Path()).To(Equal(Path{"spec", "machineTemplate", "metadata"}))

		err := ControlPlane().MachineTemplate().Metadata().Set(obj, metadata)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().Metadata().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(Equal(metadata))
	})
}

func TestControlPlaneIsUpgrading(t *testing.T) {
	tests := []struct {
		name          string
		obj           *unstructured.Unstructured
		wantUpgrading bool
	}{
		{
			name: "should return false if status is not set on control plane",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"version": "v1.2.3",
				},
			}},
			wantUpgrading: false,
		},
		{
			name: "should return false if status.version is not set on control plane",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"version": "v1.2.3",
				},
				"status": map[string]interface{}{},
			}},
			wantUpgrading: false,
		},
		{
			name: "should return false if status.version is equal to spec.version",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"version": "v1.2.3",
				},
				"status": map[string]interface{}{
					"version": "v1.2.3",
				},
			}},
			wantUpgrading: false,
		},
		{
			name: "should return true if status.version is less than spec.version",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"version": "v1.2.3",
				},
				"status": map[string]interface{}{
					"version": "v1.2.2",
				},
			}},
			wantUpgrading: true,
		},
		{
			name: "should return false if status.version is greater than spec.version",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"version": "v1.2.2",
				},
				"status": map[string]interface{}{
					"version": "v1.2.3",
				},
			}},
			wantUpgrading: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			actual, _ := ControlPlane().IsUpgrading(tt.obj)
			g.Expect(actual).To(Equal(tt.wantUpgrading))
		})
	}
}

func TestControlPlaneIsScaling(t *testing.T) {
	tests := []struct {
		name        string
		obj         *unstructured.Unstructured
		wantScaling bool
	}{
		{
			name: "should return false for stable control plane",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":        int64(2),
					"updatedReplicas": int64(2),
					"readyReplicas":   int64(2),
				},
			}},
			wantScaling: false,
		},
		{
			name: "should return true if status is not set on control plane",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
			}},
			wantScaling: true,
		},
		{
			name: "should return true if status.replicas is not set on control plane",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{},
			}},
			wantScaling: true,
		},
		{
			name: "should return true if spec and status replicas do not match",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":        int64(1),
					"updatedReplicas": int64(2),
					"readyReplicas":   int64(2),
				},
			}},
			wantScaling: true,
		},
		{
			name: "should return true if spec and status updatedReplicas do not match",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":        int64(2),
					"updatedReplicas": int64(1),
					"readyReplicas":   int64(2),
				},
			}},
			wantScaling: true,
		},
		{
			name: "should return true if spec and status readyReplicas do not match",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":        int64(2),
					"updatedReplicas": int64(2),
					"readyReplicas":   int64(1),
				},
			}},
			wantScaling: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			actual, err := ControlPlane().IsScaling(tt.obj)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(actual).To(Equal(tt.wantScaling))
		})
	}
}
