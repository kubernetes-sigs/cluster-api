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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

var (
	// GroupVersion is group version identifying rpc services defined in this package
	// and their request and response types.
	GroupVersion = schema.GroupVersion{Group: "test.runtime.cluster.x-k8s.io", Version: "v1alpha1"}

	// catalogBuilder is used to add rpc services and their request and response types
	// to a Catalog.
	catalogBuilder = catalog.Builder{GroupVersion: GroupVersion}

	// AddToCatalog adds rpc services defined in this package and their request and
	// response types to a catalog.
	AddToCatalog = catalogBuilder.AddToCatalog
)

func FakeHook(*FakeRequest, *FakeResponse) {}

type FakeRequest struct {
	metav1.TypeMeta `json:",inline"`

	Second string
	First  int
}

func (in *FakeRequest) DeepCopyObject() runtime.Object {
	panic("implement me!")
}

type FakeResponse struct {
	metav1.TypeMeta `json:",inline"`

	Second string
	First  int
}

func (in *FakeResponse) DeepCopyObject() runtime.Object {
	panic("implement me!")
}

func init() {
	catalogBuilder.RegisterHook(FakeHook, &catalog.HookMeta{
		Tags:        []string{"fake-tag"},
		Summary:     "Fake summary",
		Description: "Fake description",
		Deprecated:  true,
	})
}
