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

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "cluster.x-k8s.io", Version: "v1beta2"}

	// schemeBuilder is used to add go types to the GroupVersionKind scheme.
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = schemeBuilder.AddToScheme

	objectTypes = []runtime.Object{}

	// GroupVersionInfrastructure is the recommended group version for infrastructure objects.
	GroupVersionInfrastructure = schema.GroupVersion{Group: "infrastructure.cluster.x-k8s.io", Version: "v1beta2"}

	// GroupVersionBootstrap is the recommended group version for bootstrap objects.
	GroupVersionBootstrap = schema.GroupVersion{Group: "bootstrap.cluster.x-k8s.io", Version: "v1beta2"}

	// GroupVersionControlPlane is the recommended group version for controlplane objects.
	GroupVersionControlPlane = schema.GroupVersion{Group: "controlplane.cluster.x-k8s.io", Version: "v1beta2"}
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, objectTypes...)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
