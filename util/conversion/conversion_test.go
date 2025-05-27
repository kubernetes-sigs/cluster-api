/*
Copyright 2019 The Kubernetes Authors.

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

package conversion

import (
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/contract"
)

var (
	ctx = ctrl.SetupSignalHandler()

	oldMachineGVK = schema.GroupVersionKind{
		Group:   clusterv1.GroupVersion.Group,
		Version: "v1old",
		Kind:    "Machine",
	}
)

func TestMarshalData(t *testing.T) {
	g := NewWithT(t)

	t.Run("should write source object to destination", func(*testing.T) {
		version := "v1.16.4"
		providerID := "aws://some-id"
		src := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
				Labels: map[string]string{
					"label1": "",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "test-cluster",
				Version:     &version,
				ProviderID:  &providerID,
			},
		}

		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(oldMachineGVK)
		dst.SetName("test-1")

		g.Expect(MarshalData(src, dst)).To(Succeed())
		// ensure the src object is not modified
		g.Expect(src.GetLabels()).ToNot(BeEmpty())

		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(BeEmpty())
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("test-cluster"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("v1.16.4"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("aws://some-id"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(ContainSubstring("metadata"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(ContainSubstring("label1"))
	})

	t.Run("should append the annotation", func(*testing.T) {
		src := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}
		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("Machine"))
		dst.SetName("test-1")
		dst.SetAnnotations(map[string]string{
			"annotation": "1",
		})

		g.Expect(MarshalData(src, dst)).To(Succeed())
		g.Expect(dst.GetAnnotations()).To(HaveLen(2))
	})
}

func TestUnmarshalData(t *testing.T) {
	g := NewWithT(t)

	t.Run("should return false without errors if annotation doesn't exist", func(*testing.T) {
		src := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}
		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(oldMachineGVK)
		dst.SetName("test-1")

		ok, err := UnmarshalData(src, dst)
		g.Expect(ok).To(BeFalse())
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should return true when a valid annotation with data exists", func(*testing.T) {
		src := &unstructured.Unstructured{}
		src.SetGroupVersionKind(oldMachineGVK)
		src.SetName("test-1")
		src.SetAnnotations(map[string]string{
			DataAnnotation: "{\"metadata\":{\"name\":\"test-1\",\"creationTimestamp\":null,\"labels\":{\"label1\":\"\"}},\"spec\":{\"clusterName\":\"\",\"bootstrap\":{},\"infrastructureRef\":{}},\"status\":{\"bootstrapReady\":true,\"infrastructureReady\":true}}",
		})

		dst := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}

		ok, err := UnmarshalData(src, dst)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ok).To(BeTrue())

		g.Expect(dst.GetLabels()).To(HaveLen(1))
		g.Expect(dst.GetName()).To(Equal("test-1"))
		g.Expect(dst.GetLabels()).To(HaveKeyWithValue("label1", ""))
		g.Expect(dst.GetAnnotations()).To(BeEmpty())
	})

	t.Run("should clean the annotation on successful unmarshal", func(*testing.T) {
		src := &unstructured.Unstructured{}
		src.SetGroupVersionKind(oldMachineGVK)
		src.SetName("test-1")
		src.SetAnnotations(map[string]string{
			"annotation-1": "",
			DataAnnotation: "{\"metadata\":{\"name\":\"test-1\",\"creationTimestamp\":null,\"labels\":{\"label1\":\"\"}},\"spec\":{\"clusterName\":\"\",\"bootstrap\":{},\"infrastructureRef\":{}},\"status\":{\"bootstrapReady\":true,\"infrastructureReady\":true}}",
		})

		dst := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}

		ok, err := UnmarshalData(src, dst)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ok).To(BeTrue())

		g.Expect(src.GetAnnotations()).ToNot(HaveKey(DataAnnotation))
		g.Expect(src.GetAnnotations()).To(HaveLen(1))
	})
}

func TestGetContractVersion(t *testing.T) {
	testCases := []struct {
		name                    string
		crdLabels               map[string]string
		expectedContractVersion string
		expectError             bool
	}{
		{
			name:                    "no contract labels",
			crdLabels:               nil,
			expectedContractVersion: "",
			expectError:             true,
		},
		{
			name: "v1beta2 contract labels (only v1beta2 exist)",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			expectedContractVersion: "v1beta2",
			expectError:             false,
		},
		{
			name: "v1beta2 contract labels (only v1beta1 exist)",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
			},
			expectedContractVersion: "v1beta1",
			expectError:             false,
		},
		{
			name: "v1beta2 contract labels (both exist)",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			expectedContractVersion: "v1beta2",
			expectError:             false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gvk := clusterv1.GroupVersionBootstrap.WithKind("TestBootstrapConfig")

			u := &unstructured.Unstructured{}
			u.SetName(contract.CalculateCRDName(gvk.Group, gvk.Kind))
			u.SetGroupVersionKind(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))
			u.SetLabels(tt.crdLabels)

			fakeClient := fake.NewClientBuilder().WithObjects(u).Build()

			contractVersion, err := GetContractVersion(ctx, fakeClient, gvk)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(contractVersion).To(Equal(tt.expectedContractVersion))
		})
	}
}

func TestGetLatestAPIVersionFromContract(t *testing.T) {
	testCases := []struct {
		name                    string
		currentContractVersion  string
		crdLabels               map[string]string
		expectedContractVersion string
		expectedAPIVersion      string
		expectError             bool
	}{
		{
			name:                    "no contract labels",
			currentContractVersion:  "v1beta1",
			crdLabels:               nil,
			expectedContractVersion: "",
			expectedAPIVersion:      "",
			expectError:             true,
		},
		{
			name:                   "v1beta1 contract labels (only v1beta1 exist)",
			currentContractVersion: "v1beta1",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
			},
			expectedContractVersion: "v1beta1",
			expectedAPIVersion:      "v1alpha2",
			expectError:             false,
		},
		{
			name:                   "v1beta1 contract labels (only v1beta2 exist)",
			currentContractVersion: "v1beta1",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta2": "v1alpha1_v1alpha2",
			},
			expectedContractVersion: "",
			expectedAPIVersion:      "",
			expectError:             true,
		},
		{
			name:                   "v1beta1 contract labels (both exist)",
			currentContractVersion: "v1beta1",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			expectedContractVersion: "v1beta1",
			expectedAPIVersion:      "v1alpha2",
			expectError:             false,
		},
		{
			name:                   "v1beta2 contract labels (only v1beta2 exist)",
			currentContractVersion: "v1beta2",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			expectedContractVersion: "v1beta2",
			expectedAPIVersion:      "v1alpha4",
			expectError:             false,
		},
		{
			name:                   "v1beta2 contract labels (only v1beta1 exist)",
			currentContractVersion: "v1beta2",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
			},
			expectedContractVersion: "v1beta1",
			expectedAPIVersion:      "v1alpha2",
			expectError:             false,
		},
		{
			name:                   "v1beta2 contract labels (both exist)",
			currentContractVersion: "v1beta2",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			expectedContractVersion: "v1beta2",
			expectedAPIVersion:      "v1alpha4",
			expectError:             false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			u := &unstructured.Unstructured{}
			u.SetLabels(tt.crdLabels)

			contractVersion, apiVersion, err := getLatestAPIVersionFromContract(u, tt.currentContractVersion)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(contractVersion).To(Equal(tt.expectedContractVersion))
			g.Expect(apiVersion).To(Equal(tt.expectedAPIVersion))
		})
	}
}
