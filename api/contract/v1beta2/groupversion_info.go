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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "contract.cluster.x-k8s.io", Version: "v1beta2"}

	// SchemeGroupVersion is an alias to GroupVersion, e.g. needed for applyconfigurations.
	SchemeGroupVersion = GroupVersion

	// schemeBuilder is used to add go types to the GroupVersionKind scheme.
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = schemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	return fmt.Errorf("don't use this")

	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Group: "vmware.infrastructure.cluster.x-k8s.io", Version: "v1beta2"})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "vmware.infrastructure.cluster.x-k8s.io", Version: "v1beta2", Kind: "VSphereMachine"}, &InfraMachine{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "vmware.infrastructure.cluster.x-k8s.io", Version: "v1beta2", Kind: "VSphereMachineList"}, &InfraMachineList{})

	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Group: "bootstrap.cluster.x-k8s.io", Version: "v1beta2"})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "bootstrap.cluster.x-k8s.io", Version: "v1beta2", Kind: "KubeadmConfig"}, &BootstrapConfig{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "bootstrap.cluster.x-k8s.io", Version: "v1beta2", Kind: "KubeadmConfigList"}, &BootstrapConfigList{})

	return nil
}
