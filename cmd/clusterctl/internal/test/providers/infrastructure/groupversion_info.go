/*
Copyright 2020 The Kubernetes Authors.

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

// Package infrastructure defines the types for a generic infrastructure provider used for tests.
// +kubebuilder:object:generate=true
// +groupName=infrastructure.cluster.x-k8s.io
package infrastructure

import (
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = clusterv1.GroupVersionInfrastructure

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
