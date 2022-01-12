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

package builder

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// ControlPlaneGroupVersion is group version used for control plane objects.
	ControlPlaneGroupVersion = schema.GroupVersion{Group: "controlplane.cluster.x-k8s.io", Version: "v1beta1"}

	// GenericControlPlaneKind is the Kind for the GenericControlPlane.
	GenericControlPlaneKind = "GenericControlPlane"
	// GenericControlPlaneCRD is a generic control plane CRD.
	GenericControlPlaneCRD = generateCRD(ControlPlaneGroupVersion.WithKind(GenericControlPlaneKind))

	// GenericControlPlaneTemplateKind is the Kind for the GenericControlPlaneTemplate.
	GenericControlPlaneTemplateKind = "GenericControlPlaneTemplate"
	// GenericControlPlaneTemplateCRD is a generic control plane template CRD.
	GenericControlPlaneTemplateCRD = generateCRD(ControlPlaneGroupVersion.WithKind(GenericControlPlaneTemplateKind))
)
