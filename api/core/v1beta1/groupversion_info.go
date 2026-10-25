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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "cluster.x-k8s.io", Version: "v1beta1"}

	// schemeBuilder is used to add go types to the GroupVersionKind scheme.
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = schemeBuilder.AddToScheme

	// localSchemeBuilder is used for type conversions.
	localSchemeBuilder = &schemeBuilder

	objectTypes = []runtime.Object{}

	// GroupVersionInfrastructure is the recommended group version for infrastructure objects.
	GroupVersionInfrastructure = schema.GroupVersion{Group: "infrastructure.cluster.x-k8s.io", Version: "v1beta1"}

	// GroupVersionBootstrap is the recommended group version for bootstrap objects.
	GroupVersionBootstrap = schema.GroupVersion{Group: "bootstrap.cluster.x-k8s.io", Version: "v1beta1"}

	// GroupVersionControlPlane is the recommended group version for controlplane objects.
	GroupVersionControlPlane = schema.GroupVersion{Group: "controlplane.cluster.x-k8s.io", Version: "v1beta1"}
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, objectTypes...)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
