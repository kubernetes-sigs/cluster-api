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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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

		err := ControlPlane().Replicas().Set(obj, int32(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().Replicas().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(3)))
	})
	t.Run("Manages status.replicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().StatusReplicas().Path()).To(Equal(Path{"status", "replicas"}))

		err := ControlPlane().StatusReplicas().Set(obj, int32(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().StatusReplicas().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(3)))
	})
	t.Run("Manages status.upToDateReplicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().UpToDateReplicas("v1beta1").Path()).To(Equal(Path{"status", "updatedReplicas"}))

		err := ControlPlane().UpToDateReplicas("v1beta1").Set(obj, int32(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().UpToDateReplicas("v1beta1").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(3)))

		g.Expect(ControlPlane().UpToDateReplicas("v1beta2").Path()).To(Equal(Path{"status", "upToDateReplicas"}))

		err = ControlPlane().UpToDateReplicas("v1beta2").Set(obj, int32(5))
		g.Expect(err).ToNot(HaveOccurred())

		got, err = ControlPlane().UpToDateReplicas("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(5)))
	})
	t.Run("Manages status.readyReplicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().ReadyReplicas().Path()).To(Equal(Path{"status", "readyReplicas"}))

		err := ControlPlane().ReadyReplicas().Set(obj, int32(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().ReadyReplicas().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(3)))
	})
	t.Run("Manages status.V1Beta1UnavailableReplicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().V1Beta1UnavailableReplicas().Path()).To(Equal(Path{"status", "unavailableReplicas"}))

		err := ControlPlane().V1Beta1UnavailableReplicas().Set(obj, int64(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().V1Beta1UnavailableReplicas().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int64(3)))
	})
	t.Run("Manages status.availableReplicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(ControlPlane().AvailableReplicas().Path()).To(Equal(Path{"status", "availableReplicas"}))

		err := ControlPlane().AvailableReplicas().Set(obj, int32(3))
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().AvailableReplicas().Get(obj)
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
	t.Run("Manages spec.machineTemplate.infrastructureRef (v1beta1)", func(t *testing.T) {
		g := NewWithT(t)

		fooRef := &corev1.ObjectReference{
			APIVersion: "fooApiVersion",
			Kind:       "fooKind",
			Namespace:  "fooNamespace",
			Name:       "fooName",
		}

		g.Expect(ControlPlane().MachineTemplate().InfrastructureV1Beta1Ref().Path()).To(Equal(Path{"spec", "machineTemplate", "infrastructureRef"}))

		err := ControlPlane().MachineTemplate().InfrastructureV1Beta1Ref().Set(obj, fooRef)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().InfrastructureV1Beta1Ref().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.APIVersion).To(Equal(fooRef.APIVersion))
		g.Expect(got.Kind).To(Equal(fooRef.Kind))
		g.Expect(got.Name).To(Equal(fooRef.Name))
		g.Expect(got.Namespace).To(Equal(fooRef.Namespace))
	})
	t.Run("Manages spec.machineTemplate.spec.infrastructureRef (v1beta2)", func(t *testing.T) {
		g := NewWithT(t)

		fooRef := &clusterv1.ContractVersionedObjectReference{
			APIGroup: "fooAPIGroup",
			Kind:     "fooKind",
			Name:     "fooName",
		}

		g.Expect(ControlPlane().MachineTemplate().InfrastructureRef().Path()).To(Equal(Path{"spec", "machineTemplate", "spec", "infrastructureRef"}))

		err := ControlPlane().MachineTemplate().InfrastructureRef().Set(obj, fooRef)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().InfrastructureRef().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.APIGroup).To(Equal(fooRef.APIGroup))
		g.Expect(got.Kind).To(Equal(fooRef.Kind))
		g.Expect(got.Name).To(Equal(fooRef.Name))
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

	t.Run("Manages spec.machineTemplate.nodeDrainTimeoutSeconds", func(t *testing.T) {
		g := NewWithT(t)

		duration := int32(125)
		g.Expect(ControlPlane().MachineTemplate().NodeDrainTimeoutSeconds().Path()).To(Equal(Path{"spec", "machineTemplate", "spec", "deletion", "nodeDrainTimeoutSeconds"}))

		err := ControlPlane().MachineTemplate().NodeDrainTimeoutSeconds().Set(obj, duration)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().NodeDrainTimeoutSeconds().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(duration))

		// Check that the literal string value of the duration is correctly formatted.
		gotDuration, found, err := unstructured.NestedInt64(obj.UnstructuredContent(), "spec", "machineTemplate", "spec", "deletion", "nodeDrainTimeoutSeconds")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(gotDuration).To(Equal(int64(duration)))
	})

	t.Run("Manages spec.machineTemplate.nodeVolumeDetachTimeoutSeconds", func(t *testing.T) {
		g := NewWithT(t)

		duration := int32(130)
		g.Expect(ControlPlane().MachineTemplate().NodeVolumeDetachTimeoutSeconds().Path()).To(Equal(Path{"spec", "machineTemplate", "spec", "deletion", "nodeVolumeDetachTimeoutSeconds"}))

		err := ControlPlane().MachineTemplate().NodeVolumeDetachTimeoutSeconds().Set(obj, duration)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().NodeVolumeDetachTimeoutSeconds().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(duration))

		// Check that the literal string value of the duration is correctly formatted.
		gotDuration, found, err := unstructured.NestedInt64(obj.UnstructuredContent(), "spec", "machineTemplate", "spec", "deletion", "nodeVolumeDetachTimeoutSeconds")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(gotDuration).To(Equal(int64(duration)))
	})

	t.Run("Manages spec.machineTemplate.nodeDeletionTimeoutSeconds", func(t *testing.T) {
		g := NewWithT(t)

		duration := int32(125)
		g.Expect(ControlPlane().MachineTemplate().NodeDeletionTimeoutSeconds().Path()).To(Equal(Path{"spec", "machineTemplate", "spec", "deletion", "nodeDeletionTimeoutSeconds"}))

		err := ControlPlane().MachineTemplate().NodeDeletionTimeoutSeconds().Set(obj, duration)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().NodeDeletionTimeoutSeconds().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(duration))

		// Check that the literal string value of the duration is correctly formatted.
		gotDuration, found, err := unstructured.NestedInt64(obj.UnstructuredContent(), "spec", "machineTemplate", "spec", "deletion", "nodeDeletionTimeoutSeconds")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(gotDuration).To(Equal(int64(duration)))
	})

	t.Run("Manages spec.machineTemplate.readinessGates (v1beta1 contract)", func(t *testing.T) {
		g := NewWithT(t)

		readinessGates := []clusterv1.MachineReadinessGate{
			{ConditionType: "foo"},
			{ConditionType: "bar"},
		}

		g.Expect(ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Path()).To(Equal(Path{"spec", "machineTemplate", "readinessGates"}))

		err := ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Set(obj, readinessGates)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(BeComparableTo(readinessGates))

		// Nil readinessGates are not set.
		obj2 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		readinessGates = nil

		err = ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Set(obj2, readinessGates)
		g.Expect(err).ToNot(HaveOccurred())

		_, ok, err := unstructured.NestedSlice(obj2.UnstructuredContent(), ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Path()...)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ok).To(BeFalse())

		_, err = ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Get(obj2)
		g.Expect(err).To(HaveOccurred())

		// Empty readinessGates are set.
		obj3 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		readinessGates = []clusterv1.MachineReadinessGate{}

		err = ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Set(obj3, readinessGates)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Get(obj3)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(BeComparableTo(readinessGates))
	})

	t.Run("Manages spec.machineTemplate.readinessGates (v1beta2 contract)", func(t *testing.T) {
		g := NewWithT(t)

		readinessGates := []clusterv1.MachineReadinessGate{
			{ConditionType: "foo"},
			{ConditionType: "bar"},
		}

		g.Expect(ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Path()).To(Equal(Path{"spec", "machineTemplate", "spec", "readinessGates"}))

		err := ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Set(obj, readinessGates)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(BeComparableTo(readinessGates))

		// Nil readinessGates are not set.
		obj2 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		readinessGates = nil

		err = ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Set(obj2, readinessGates)
		g.Expect(err).ToNot(HaveOccurred())

		_, ok, err := unstructured.NestedSlice(obj2.UnstructuredContent(), ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Path()...)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ok).To(BeFalse())

		_, err = ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Get(obj2)
		g.Expect(err).To(HaveOccurred())

		// Empty readinessGates are set.
		obj3 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		readinessGates = []clusterv1.MachineReadinessGate{}

		err = ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Set(obj3, readinessGates)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Get(obj3)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(BeComparableTo(readinessGates))
	})
}

func TestControlPlaneEndpoints(t *testing.T) {
	tests := []struct {
		name         string
		controlPlane *unstructured.Unstructured
		want         []Path
		expectErr    bool
	}{
		{
			name: "No ignore paths when controlPlaneEndpoint is not set",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"server": "1.2.3.4",
					},
				},
			},
			want: nil,
		},
		{
			name: "No ignore paths when controlPlaneEndpoint is nil",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": nil,
					},
				},
			},

			want: nil,
		},
		{
			name: "No ignore paths when controlPlaneEndpoint is an empty object",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{},
					},
				},
			},

			want: nil,
		},
		{
			name: "Don't ignore host when controlPlaneEndpoint.host is set",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "example.com",
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "Ignore host when controlPlaneEndpoint.host is set to its zero value",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "host"},
			},
		},
		{
			name: "Don't ignore port when controlPlaneEndpoint.port is set",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"port": int64(6443),
						},
					},
				},
			},

			want: nil,
		},
		{
			name: "Ignore port when controlPlaneEndpoint.port is set to its zero value",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"port": int64(0),
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "port"},
			},
		},
		{
			name: "Ignore host and port when controlPlaneEndpoint host and port are set to their zero values",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
							"port": int64(0),
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "host"},
				{"spec", "controlPlaneEndpoint", "port"},
			},
		},
		{
			name: "Ignore host when controlPlaneEndpoint host is to its zero values, even if port is set",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
							"port": int64(6443),
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "host"},
			},
		},
		{
			name: "Ignore port when controlPlaneEndpoint port is to its zero values, even if host is set",
			controlPlane: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "example.com",
							"port": int64(0),
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "port"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := InfrastructureCluster().IgnorePaths(tt.controlPlane)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
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

func TestGetAndSetNestedRef(t *testing.T) {
	t.Run("Gets a nested ref if defined", func(t *testing.T) {
		g := NewWithT(t)

		fooRef := &clusterv1.ContractVersionedObjectReference{
			APIGroup: "fooAPIGroup",
			Kind:     "fooKind",
			Name:     "fooName",
		}
		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

		err := setNestedRef(obj, fooRef, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).ToNot(HaveOccurred())

		ref, err := getNestedRef(obj, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ref).ToNot(BeNil())
		g.Expect(ref.APIGroup).To(Equal(fooRef.APIGroup))
		g.Expect(ref.Kind).To(Equal(fooRef.Kind))
		g.Expect(ref.Name).To(Equal(fooRef.Name))
	})
	t.Run("getNestedRef fails if the nested ref does not exist", func(t *testing.T) {
		g := NewWithT(t)

		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

		ref, err := getNestedRef(obj, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).To(HaveOccurred())
		g.Expect(ref).To(BeNil())
	})
	t.Run("getNestedRef fails if the nested ref exist but it is incomplete", func(t *testing.T) {
		g := NewWithT(t)

		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

		err := unstructured.SetNestedField(obj.UnstructuredContent(), "foo", "spec", "machineTemplate", "infrastructureRef", "kind")
		g.Expect(err).ToNot(HaveOccurred())
		err = unstructured.SetNestedField(obj.UnstructuredContent(), "bar", "spec", "machineTemplate", "infrastructureRef", "namespace")
		g.Expect(err).ToNot(HaveOccurred())
		err = unstructured.SetNestedField(obj.UnstructuredContent(), "baz", "spec", "machineTemplate", "infrastructureRef", "apiVersion")
		g.Expect(err).ToNot(HaveOccurred())
		// Reference name missing

		ref, err := getNestedRef(obj, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).To(HaveOccurred())
		g.Expect(ref).To(BeNil())
	})
}
