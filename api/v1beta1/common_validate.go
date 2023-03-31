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
	metavalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Validate validates the labels and annotations in ObjectMeta.
func (metadata *ObjectMeta) Validate(parent *field.Path) field.ErrorList {
	allErrs := metav1validation.ValidateLabels(
		metadata.Labels,
		parent.Child("labels"),
	)
	allErrs = append(allErrs, metavalidation.ValidateAnnotations(
		metadata.Annotations,
		parent.Child("annotations"),
	)...)
	return allErrs
}
