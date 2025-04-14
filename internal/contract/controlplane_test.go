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
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
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
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().StatusVersion().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("1.2.3"))
	})
	t.Run("Manages status.initialization.controlPlaneInitialized", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().Initialized("v1beta2").Path()).To(Equal(Path{"status", "initialization", "controlPlaneInitialized"}))

		err := ControlPlane().Initialized("v1beta2").Set(obj, true)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().Initialized("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())

		g.Expect(ControlPlane().Initialized("v1beta1").Path()).To(Equal(Path{"status", "ready"}))

		objV1beta1 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err = ControlPlane().Initialized("v1beta1").Set(objV1beta1, true)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = ControlPlane().Initialized("v1beta1").Get(objV1beta1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())
	})
	t.Run("Manages spec.ControlPlaneEndpoint", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().ControlPlaneEndpoint().Path()).To(Equal(Path{"spec", "controlPlaneEndpoint"}))

		endpoint := clusterv1.APIEndpoint{
			Host: "example.com",
			Port: 1234,
		}

		err := ControlPlane().ControlPlaneEndpoint().Set(obj, endpoint)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().ControlPlaneEndpoint().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(endpoint))
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

		g.Expect(ControlPlane().UpdatedReplicas("v1beta1").Path()).To(Equal(Path{"status", "updatedReplicas"}))

		err := ControlPlane().UpdatedReplicas("v1beta1").Set(obj, int64(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().UpdatedReplicas("v1beta1").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(3)))

		g.Expect(ControlPlane().UpdatedReplicas("v1beta2").Path()).To(Equal(Path{"status", "deprecated", "v1beta1", "updatedReplicas"}))

		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"deprecated": map[string]interface{}{
					"v1beta1": map[string]interface{}{
						"updatedReplicas": int64(5),
					},
				},
			},
		}}

		got, err = ControlPlane().UpdatedReplicas("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(5)))
	})
	t.Run("Manages status.readyReplicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().ReadyReplicas("v1beta1").Path()).To(Equal(Path{"status", "readyReplicas"}))

		err := ControlPlane().ReadyReplicas("v1beta1").Set(obj, int64(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().ReadyReplicas("v1beta1").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(3)))

		g.Expect(ControlPlane().ReadyReplicas("v1beta2").Path()).To(Equal(Path{"status", "deprecated", "v1beta1", "readyReplicas"}))

		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"deprecated": map[string]interface{}{
					"v1beta1": map[string]interface{}{
						"readyReplicas": int64(5),
					},
				},
			},
		}}

		got, err = ControlPlane().ReadyReplicas("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(5)))
	})
	t.Run("Manages status.unavailableReplicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().UnavailableReplicas("v1beta1").Path()).To(Equal(Path{"status", "unavailableReplicas"}))

		err := ControlPlane().UnavailableReplicas("v1beta1").Set(obj, int64(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().UnavailableReplicas("v1beta1").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(3)))

		g.Expect(ControlPlane().UnavailableReplicas("v1beta2").Path()).To(Equal(Path{"status", "deprecated", "v1beta1", "unavailableReplicas"}))

		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"deprecated": map[string]interface{}{
					"v1beta1": map[string]interface{}{
						"unavailableReplicas": int64(5),
					},
				},
			},
		}}

		got, err = ControlPlane().UnavailableReplicas("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(5)))
	})
	t.Run("Manages status.readyReplicas for v1beta2 status", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().V1Beta2ReadyReplicas("v1beta1").Path()).To(Equal(Path{"status", "v1beta2", "readyReplicas"}))

		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"readyReplicas": int64(3),
				"v1beta2": map[string]interface{}{
					"readyReplicas": int64(5),
				},
			},
		}}

		got, err := ControlPlane().V1Beta2ReadyReplicas("v1beta1").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(5)))

		g.Expect(ControlPlane().V1Beta2ReadyReplicas("v1beta2").Path()).To(Equal(Path{"status", "readyReplicas"}))

		err = ControlPlane().V1Beta2ReadyReplicas("v1beta2").Set(obj, 3)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = ControlPlane().V1Beta2ReadyReplicas("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(3)))
	})
	t.Run("Manages status.availableReplicas for v1beta2 status", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().V1Beta2AvailableReplicas("v1beta1").Path()).To(Equal(Path{"status", "v1beta2", "availableReplicas"}))

		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"availableReplicas": int64(3),
				"v1beta2": map[string]interface{}{
					"availableReplicas": int64(5),
				},
			},
		}}

		got, err := ControlPlane().V1Beta2AvailableReplicas("v1beta1").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(5)))

		g.Expect(ControlPlane().V1Beta2AvailableReplicas("v1beta2").Path()).To(Equal(Path{"status", "availableReplicas"}))

		err = ControlPlane().V1Beta2AvailableReplicas("v1beta2").Set(obj, 3)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = ControlPlane().V1Beta2AvailableReplicas("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(3)))
	})
	t.Run("Manages status.upToDateReplicas for v1beta2 status", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().V1Beta2UpToDateReplicas("v1beta1").Path()).To(Equal(Path{"status", "v1beta2", "upToDateReplicas"}))

		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"status": map[string]interface{}{
				"upToDateReplicas": int64(3),
				"v1beta2": map[string]interface{}{
					"upToDateReplicas": int64(5),
				},
			},
		}}

		got, err := ControlPlane().V1Beta2UpToDateReplicas("v1beta1").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(5)))

		g.Expect(ControlPlane().V1Beta2UpToDateReplicas("v1beta2").Path()).To(Equal(Path{"status", "upToDateReplicas"}))

		err = ControlPlane().V1Beta2UpToDateReplicas("v1beta2").Set(obj, 3)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = ControlPlane().V1Beta2UpToDateReplicas("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(3)))
	})
	t.Run("Manages status.selector", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().Selector().Path()).To(Equal(Path{"status", "selector"}))

		err := ControlPlane().Selector().Set(obj, "my-selector")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().Selector().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("my-selector"))
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
		g.Expect(got).To(BeComparableTo(metadata))
	})

	t.Run("Manages spec.machineTemplate.nodeDrainTimeout", func(t *testing.T) {
		g := NewWithT(t)

		duration := metav1.Duration{Duration: 2*time.Minute + 5*time.Second}
		expectedDurationString := "2m5s"
		g.Expect(ControlPlane().MachineTemplate().NodeDrainTimeout().Path()).To(Equal(Path{"spec", "machineTemplate", "nodeDrainTimeout"}))

		err := ControlPlane().MachineTemplate().NodeDrainTimeout().Set(obj, duration)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().NodeDrainTimeout().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(duration))

		// Check that the literal string value of the duration is correctly formatted.
		durationString, found, err := unstructured.NestedString(obj.UnstructuredContent(), "spec", "machineTemplate", "nodeDrainTimeout")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(durationString).To(Equal(expectedDurationString))
	})

	t.Run("Manages spec.machineTemplate.nodeVolumeDetachTimeout", func(t *testing.T) {
		g := NewWithT(t)

		duration := metav1.Duration{Duration: 2*time.Minute + 10*time.Second}
		expectedDurationString := "2m10s"
		g.Expect(ControlPlane().MachineTemplate().NodeVolumeDetachTimeout().Path()).To(Equal(Path{"spec", "machineTemplate", "nodeVolumeDetachTimeout"}))

		err := ControlPlane().MachineTemplate().NodeVolumeDetachTimeout().Set(obj, duration)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().NodeVolumeDetachTimeout().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(duration))

		// Check that the literal string value of the duration is correctly formatted.
		durationString, found, err := unstructured.NestedString(obj.UnstructuredContent(), "spec", "machineTemplate", "nodeVolumeDetachTimeout")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(durationString).To(Equal(expectedDurationString))
	})

	t.Run("Manages spec.machineTemplate.nodeDeletionTimeout", func(t *testing.T) {
		g := NewWithT(t)

		duration := metav1.Duration{Duration: 2*time.Minute + 5*time.Second}
		expectedDurationString := "2m5s"
		g.Expect(ControlPlane().MachineTemplate().NodeDeletionTimeout().Path()).To(Equal(Path{"spec", "machineTemplate", "nodeDeletionTimeout"}))

		err := ControlPlane().MachineTemplate().NodeDeletionTimeout().Set(obj, duration)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().NodeDeletionTimeout().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(duration))

		// Check that the literal string value of the duration is correctly formatted.
		durationString, found, err := unstructured.NestedString(obj.UnstructuredContent(), "spec", "machineTemplate", "nodeDeletionTimeout")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(durationString).To(Equal(expectedDurationString))
	})
	t.Run("Manages spec.machineTemplate.readinessGates", func(t *testing.T) {
		g := NewWithT(t)

		readinessGates := []clusterv1.MachineReadinessGate{
			{ConditionType: "foo"},
			{ConditionType: "bar"},
		}

		g.Expect(ControlPlane().MachineTemplate().ReadinessGates().Path()).To(Equal(Path{"spec", "machineTemplate", "readinessGates"}))

		err := ControlPlane().MachineTemplate().ReadinessGates().Set(obj, readinessGates)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().ReadinessGates().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(BeComparableTo(readinessGates))

		// Nil readinessGates are not set.
		obj2 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		readinessGates = nil

		err = ControlPlane().MachineTemplate().ReadinessGates().Set(obj2, readinessGates)
		g.Expect(err).ToNot(HaveOccurred())

		_, ok, err := unstructured.NestedSlice(obj2.UnstructuredContent(), ControlPlane().MachineTemplate().ReadinessGates().Path()...)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ok).To(BeFalse())

		_, err = ControlPlane().MachineTemplate().ReadinessGates().Get(obj2)
		g.Expect(err).To(HaveOccurred())

		// Empty readinessGates are set.
		obj3 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		readinessGates = []clusterv1.MachineReadinessGate{}

		err = ControlPlane().MachineTemplate().ReadinessGates().Set(obj3, readinessGates)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = ControlPlane().MachineTemplate().ReadinessGates().Get(obj3)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(BeComparableTo(readinessGates))
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
					"replicas":            int64(2),
					"updatedReplicas":     int64(2),
					"readyReplicas":       int64(2),
					"unavailableReplicas": int64(0),
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
			name: "should return true if status replicas is not set on control plane",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"updatedReplicas":     int64(2),
					"readyReplicas":       int64(2),
					"unavailableReplicas": int64(0),
				},
			}},
			wantScaling: true,
		},
		{
			name: "should return true if spec replicas and status replicas do not match",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":            int64(1),
					"updatedReplicas":     int64(2),
					"readyReplicas":       int64(2),
					"unavailableReplicas": int64(0),
				},
			}},
			wantScaling: true,
		},
		{
			name: "should return true if status updatedReplicas is not set on control plane",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":            int64(2),
					"readyReplicas":       int64(2),
					"unavailableReplicas": int64(0),
				},
			}},
			wantScaling: true,
		},
		{
			name: "should return true if spec replicas and status updatedReplicas do not match",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":            int64(2),
					"updatedReplicas":     int64(1),
					"readyReplicas":       int64(2),
					"unavailableReplicas": int64(0),
				},
			}},
			wantScaling: true,
		},
		{
			name: "should return true if status readyReplicas is not set on control plane",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":            int64(2),
					"updatedReplicas":     int64(2),
					"unavailableReplicas": int64(0),
				},
			}},
			wantScaling: true,
		},
		{
			name: "should return true if spec replicas and status readyReplicas do not match",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":            int64(2),
					"updatedReplicas":     int64(2),
					"readyReplicas":       int64(1),
					"unavailableReplicas": int64(0),
				},
			}},
			wantScaling: true,
		},
		{
			name: "should return false if status unavailableReplicas is not set on control plane",
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
			name: "should return true if status unavailableReplicas is > 0",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(2),
				},
				"status": map[string]interface{}{
					"replicas":            int64(2),
					"updatedReplicas":     int64(2),
					"readyReplicas":       int64(2),
					"unavailableReplicas": int64(1),
				},
			}},
			wantScaling: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			actual, err := ControlPlane().IsScaling(tt.obj, "v1beta1")
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(actual).To(Equal(tt.wantScaling))
		})
	}
}
