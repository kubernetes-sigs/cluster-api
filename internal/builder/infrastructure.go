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
	// InfrastructureGroupVersion is group version used for infrastructure objects.
	InfrastructureGroupVersion = schema.GroupVersion{Group: "infrastructure.cluster.x-k8s.io", Version: "v1beta1"}

	// GenericInfrastructureMachineKind is the Kind for the GenericInfrastructureMachine.
	GenericInfrastructureMachineKind = "GenericInfrastructureMachine"
	// GenericInfrastructureMachineCRD is a generic infrastructure machine CRD.
	GenericInfrastructureMachineCRD = generateCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureMachineKind))

	// GenericInfrastructureMachineTemplateKind is the Kind for the GenericInfrastructureMachineTemplate.
	GenericInfrastructureMachineTemplateKind = "GenericInfrastructureMachineTemplate"
	// GenericInfrastructureMachineTemplateCRD is a generic infrastructure machine template CRD.
	GenericInfrastructureMachineTemplateCRD = generateCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureMachineTemplateKind))

	// GenericInfrastructureClusterKind is the kind for the GenericInfrastructureCluster type.
	GenericInfrastructureClusterKind = "GenericInfrastructureCluster"
	// GenericInfrastructureClusterCRD is a generic infrastructure machine CRD.
	GenericInfrastructureClusterCRD = generateCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureClusterKind))

	// GenericInfrastructureClusterTemplateKind is the kind for the GenericInfrastructureClusterTemplate type.
	GenericInfrastructureClusterTemplateKind = "GenericInfrastructureClusterTemplate"
	// GenericInfrastructureClusterTemplateCRD is a generic infrastructure machine template CRD.
	GenericInfrastructureClusterTemplateCRD = generateCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureClusterTemplateKind))
)
