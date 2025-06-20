/*
Copyright 2025 The Kubernetes Authors.

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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/contract"
)

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

			gk := clusterv1.GroupVersionBootstrap.WithKind("TestBootstrapConfig").GroupKind()

			u := &unstructured.Unstructured{}
			u.SetName(contract.CalculateCRDName(gk.Group, gk.Kind))
			u.SetGroupVersionKind(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))
			u.SetLabels(tt.crdLabels)

			fakeClient := fake.NewClientBuilder().WithObjects(u).Build()

			contractVersion, err := GetContractVersion(t.Context(), fakeClient, gk)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(contractVersion).To(Equal(tt.expectedContractVersion))
		})
	}
}

func TestGetAPIVersion(t *testing.T) {
	testCases := []struct {
		name               string
		crdLabels          map[string]string
		expectedAPIVersion string
		expectError        bool
	}{
		{
			name:               "no contract labels",
			crdLabels:          nil,
			expectedAPIVersion: "",
			expectError:        true,
		},
		{
			name: "only v1beta1 contract label",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
			},
			expectedAPIVersion: "bootstrap.cluster.x-k8s.io/v1alpha2",
			expectError:        false,
		},
		{
			name: "only v1beta2 contract label",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			expectedAPIVersion: "bootstrap.cluster.x-k8s.io/v1alpha4",
			expectError:        false,
		},
		{
			name: "v1beta1 and v1beta2 contract labels",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			expectedAPIVersion: "bootstrap.cluster.x-k8s.io/v1alpha4",
			expectError:        false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gk := clusterv1.GroupVersionBootstrap.WithKind("TestBootstrapConfig").GroupKind()

			u := &unstructured.Unstructured{}
			u.SetName(contract.CalculateCRDName(gk.Group, gk.Kind))
			u.SetGroupVersionKind(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))
			u.SetLabels(tt.crdLabels)

			fakeClient := fake.NewClientBuilder().WithObjects(u).Build()

			apiVersion, err := GetAPIVersion(t.Context(), fakeClient, gk)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(apiVersion).To(Equal(tt.expectedAPIVersion))
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

			contractVersion, apiVersion, err := GetLatestContractAndAPIVersionFromContract(u, tt.currentContractVersion)

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

func TestGetContractVersionForVersion(t *testing.T) {
	testCases := []struct {
		name                    string
		crdLabels               map[string]string
		version                 string
		expectedContractVersion string
		expectError             bool
	}{
		{
			name:                    "no contract labels",
			crdLabels:               nil,
			expectedContractVersion: "",
			version:                 "v1alpha3",
			expectError:             true,
		},
		{
			name: "pick v1beta1",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			version:                 "v1alpha1",
			expectedContractVersion: "v1beta1",
			expectError:             false,
		},
		{
			name: "pick v1beta1",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			version:                 "v1alpha2",
			expectedContractVersion: "v1beta1",
			expectError:             false,
		},
		{
			name: "pick v1beta2",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			version:                 "v1alpha3",
			expectedContractVersion: "v1beta2",
			expectError:             false,
		},
		{
			name: "pick v1beta2",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			version:                 "v1alpha4",
			expectedContractVersion: "v1beta2",
			expectError:             false,
		},
		{
			name: "error",
			crdLabels: map[string]string{
				"cluster.x-k8s.io/v1beta1": "v1alpha1_v1alpha2",
				"cluster.x-k8s.io/v1beta2": "v1alpha3_v1alpha4",
			},
			version:     "v1alpha5",
			expectError: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gk := clusterv1.GroupVersionBootstrap.WithKind("TestBootstrapConfig").GroupKind()

			u := &unstructured.Unstructured{}
			u.SetName(contract.CalculateCRDName(gk.Group, gk.Kind))
			u.SetGroupVersionKind(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))
			u.SetLabels(tt.crdLabels)

			fakeClient := fake.NewClientBuilder().WithObjects(u).Build()

			contractVersion, err := GetContractVersionForVersion(t.Context(), fakeClient, gk, tt.version)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(contractVersion).To(Equal(tt.expectedContractVersion))
		})
	}
}
