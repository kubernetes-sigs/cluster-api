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
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"sigs.k8s.io/cluster-api/feature"
)

const dockerClusterTemplateImmutableMsg = "DockerClusterTemplate spec.template.spec field is immutable. Please create a new resource instead."

func (r *DockerClusterTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-dockerclustertemplate,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclustertemplates,versions=v1beta1,name=default.dockerclustertemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Defaulter = &DockerClusterTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *DockerClusterTemplate) Default() {
	defaultDockerClusterSpec(&r.Spec.Template.Spec)
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-dockerclustertemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclustertemplates,versions=v1beta1,name=validation.dockerclustertemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Validator = &DockerClusterTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *DockerClusterTemplate) ValidateCreate() (admission.Warnings, error) {
	// NOTE: DockerClusterTemplate is behind ClusterTopology feature gate flag; the web hook
	// must prevent creating new objects in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return nil, field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterTopology feature flag is enabled",
		)
	}

	allErrs := validateDockerClusterSpec(r.Spec.Template.Spec)
	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("DockerClusterTemplate").GroupKind(), r.Name, allErrs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *DockerClusterTemplate) ValidateUpdate(oldRaw runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	old, ok := oldRaw.(*DockerClusterTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DockerClusterTemplate but got a %T", oldRaw))
	}
	if !reflect.DeepEqual(r.Spec.Template.Spec, old.Spec.Template.Spec) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "template", "spec"), r, dockerClusterTemplateImmutableMsg))
	}
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(GroupVersion.WithKind("DockerClusterTemplate").GroupKind(), r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *DockerClusterTemplate) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}
