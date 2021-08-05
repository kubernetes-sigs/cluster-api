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

package testtypes

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// RemediationGroupVersion is group version used for remediation objects.
	RemediationGroupVersion = schema.GroupVersion{Group: "remediation.external.io", Version: "v1alpha4"}

	// GenericRemediationCRD is a generic infrastructure remediation CRD.
	GenericRemediationCRD = generateCRD(RemediationGroupVersion.WithKind("GenericExternalRemediation"))

	// GenericRemediationTemplateCRD is a generic infrastructure remediation template CRD.
	GenericRemediationTemplateCRD = generateCRD(RemediationGroupVersion.WithKind("GenericExternalRemediationTemplate"))
)
