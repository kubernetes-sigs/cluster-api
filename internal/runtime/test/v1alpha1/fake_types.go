/*
Copyright 2022 The Kubernetes Authors.

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

// Package v1alpha1 contains types for catalog tests
// Note: they have to be in a separate package because otherwise it wouldn't
// be possible to register different versions of the same hook.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

var (
	// GroupVersion is group version identifying rpc services defined in this package
	// and their request and response types.
	GroupVersion = schema.GroupVersion{Group: "test.runtime.cluster.x-k8s.io", Version: "v1alpha1"}

	// catalogBuilder is used to add rpc services and their request and response types
	// to a Catalog.
	catalogBuilder = &runtimecatalog.Builder{GroupVersion: GroupVersion}

	// AddToCatalog adds rpc services defined in this package and their request and
	// response types to a catalog.
	AddToCatalog = catalogBuilder.AddToCatalog

	// localSchemeBuilder provide access to the SchemeBuilder used for managing rpc
	// method's request and response types defined in this package.
	// NOTE: this object is required to allow registration of automatically generated
	// conversions func.
	localSchemeBuilder = catalogBuilder
)

func FakeHook(*FakeRequest, *FakeResponse) {}

// FakeRequest is a response for testing
// +kubebuilder:object:root=true
type FakeRequest struct {
	metav1.TypeMeta `json:",inline"`

	Cluster clusterv1alpha4.Cluster

	Second string
	First  int
}

// FakeResponse is a response for testing
// +kubebuilder:object:root=true
type FakeResponse struct {
	metav1.TypeMeta               `json:",inline"`
	runtimehooksv1.CommonResponse `json:",inline"`

	Second string
	First  int
}

func SecondFakeHook(*SecondFakeRequest, *SecondFakeResponse) {}

// SecondFakeRequest is a response for testing
// +kubebuilder:object:root=true
type SecondFakeRequest struct {
	metav1.TypeMeta `json:",inline"`

	Cluster clusterv1alpha4.Cluster

	Second string
	First  int
}

// SecondFakeResponse is a response for testing
// +kubebuilder:object:root=true
type SecondFakeResponse struct {
	metav1.TypeMeta               `json:",inline"`
	runtimehooksv1.CommonResponse `json:",inline"`
	Second                        string
	First                         int
}

func init() {
	catalogBuilder.RegisterHook(FakeHook, &runtimecatalog.HookMeta{
		Tags:        []string{"fake-tag"},
		Summary:     "Fake summary",
		Description: "Fake description",
		Deprecated:  true,
	})

	catalogBuilder.RegisterHook(SecondFakeHook, &runtimecatalog.HookMeta{
		Tags:        []string{"fake-tag"},
		Summary:     "Second Fake summary",
		Description: "Second Fake description",
		Deprecated:  true,
	})
}
