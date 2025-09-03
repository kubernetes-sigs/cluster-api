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

package kind

import (
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
)

func TestGetMapping(t *testing.T) {
	testCases := []struct {
		name            string
		k8sVersion      semver.Version
		customImage     string
		expectedMapping Mapping
	}{
		{
			name:        "Exact match for custom image",
			k8sVersion:  semver.MustParse("1.23.17"),
			customImage: "kindest/node:v1.23.17@sha256:f77f8cf0b30430ca4128cc7cfafece0c274a118cd0cdb251049664ace0dee4ff",
			expectedMapping: Mapping{
				Mode:  Mode0_19,
				Image: "kindest/node:v1.23.17@sha256:f77f8cf0b30430ca4128cc7cfafece0c274a118cd0cdb251049664ace0dee4ff",
			},
		},
		{
			name:        "No match for custom image fallback on K8s version match",
			k8sVersion:  semver.MustParse("1.23.17"),
			customImage: "foo",
			expectedMapping: Mapping{
				Mode:  Mode0_20,
				Image: "foo",
			},
		},
		{
			name:       "Exact match for Kubernetes version, kind Mode0_20",
			k8sVersion: semver.MustParse("1.29.1"),
			expectedMapping: Mapping{
				Mode:  Mode0_20,
				Image: "kindest/node:v1.29.1@sha256:0c06baa545c3bb3fbd4828eb49b8b805f6788e18ce67bff34706ffa91866558b",
			},
		},
		{
			name:       "Exact match for Kubernetes version, kind Mode0_20",
			k8sVersion: semver.MustParse("1.27.3"),
			expectedMapping: Mapping{
				Mode:  Mode0_20,
				Image: "kindest/node:v1.27.3@sha256:3966ac761ae0136263ffdb6cfd4db23ef8a83cba8a463690e98317add2c9ba72",
			},
		},
		{
			name:       "Exact match for Kubernetes version, kind Mode0_19",
			k8sVersion: semver.MustParse("1.27.1"),
			expectedMapping: Mapping{
				Mode:  Mode0_19,
				Image: "kindest/node:v1.27.1@sha256:b7d12ed662b873bd8510879c1846e87c7e676a79fefc93e17b2a52989d3ff42b",
			},
		},
		{
			name:       "In case of multiple matches for Kubernetes version, return the most recent kind mode",
			k8sVersion: semver.MustParse("1.23.17"),
			expectedMapping: Mapping{
				Mode:  Mode0_20,
				Image: "kindest/node:v1.23.17@sha256:14d0a9a892b943866d7e6be119a06871291c517d279aedb816a4b4bc0ec0a5b3",
			},
		},
		{
			name:       "No match Future version gets latest kind mode",
			k8sVersion: semver.MustParse("1.27.99"),
			expectedMapping: Mapping{
				Mode:  latestMode,
				Image: "kindest/node:v1.27.99",
			},
		},
		{
			name:       "No match - In case of patch version older than the last know matches return the oldest mode know for the major/minor",
			k8sVersion: semver.MustParse("1.23.0"),
			expectedMapping: Mapping{
				Mode:  Mode0_19,
				Image: "kindest/node:v1.23.0",
			},
		},
		{
			name:        "No Match custom image, No match Future version gets latest kind mode",
			k8sVersion:  semver.MustParse("1.27.99"),
			customImage: "foo",
			expectedMapping: Mapping{
				Mode:  latestMode,
				Image: "foo",
			},
		},
		{
			name:        "No Match custom image, No match - In case of patch version older than the last know matches return the oldest mode know for the major/minor",
			k8sVersion:  semver.MustParse("1.23.0"),
			customImage: "foo",
			expectedMapping: Mapping{
				Mode:  Mode0_19,
				Image: "foo",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			got := GetMapping(tc.k8sVersion, tc.customImage)

			tc.expectedMapping.KubernetesVersion = tc.k8sVersion
			g.Expect(got).To(BeComparableTo(tc.expectedMapping))
		})
	}
}

func TestGetKubernetesVersion(t *testing.T) {
	g := NewWithT(t)

	got := GetKubernetesVersions()

	g.Expect(got).To(Equal([]string{
		"v1.19.16",
		"v1.20.15",
		"v1.21.14",
		"v1.22.15", "v1.22.17",
		"v1.23.13", "v1.23.17",
		"v1.24.7", "v1.24.12", "v1.24.13", "v1.24.15", "v1.24.17",
		"v1.25.3", "v1.25.8", "v1.25.9", "v1.25.11", "v1.25.16",
		"v1.26.0", "v1.26.3", "v1.26.4", "v1.26.6", "v1.26.13", "v1.26.14", "v1.26.15",
		"v1.27.0", "v1.27.1", "v1.27.3", "v1.27.10", "v1.27.11", "v1.27.13", "v1.27.16",
		"v1.28.0", "v1.28.6", "v1.28.7", "v1.28.9", "v1.28.12", "v1.28.13", "v1.28.15",
		"v1.29.0", "v1.29.1", "v1.29.2", "v1.29.4", "v1.29.7", "v1.29.8", "v1.29.10", "v1.29.12", "v1.29.14",
		"v1.30.0", "v1.30.3", "v1.30.4", "v1.30.6", "v1.30.8", "v1.30.10", "v1.30.13",
		"v1.31.0", "v1.31.2", "v1.31.4", "v1.31.6", "v1.31.9",
		"v1.32.0", "v1.32.2", "v1.32.5",
		"v1.33.0", "v1.33.1",
	}))
}
