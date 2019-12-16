/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha3

import (
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (r *KubeadmControlPlane) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-controlplane-cluster-x-k8s-io-v1alpha3-kubeadmcontrolplane,mutating=true,failurePolicy=fail,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,versions=v1alpha3,name=default.kubeadmcontrolplane.controlplane.cluster.x-k8s.io
// +kubebuilder:webhook:verbs=create;update,path=/validate-controlplane-cluster-x-k8s-io-v1alpha3-kubeadmcontrolplane,mutating=false,failurePolicy=fail,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,versions=v1alpha3,name=validation.kubeadmcontrolplane.controlplane.cluster.x-k8s.io

var _ webhook.Defaulter = &KubeadmControlPlane{}
var _ webhook.Validator = &KubeadmControlPlane{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *KubeadmControlPlane) Default() {
	if r.Spec.Replicas == nil {
		replicas := int32(1)
		r.Spec.Replicas = &replicas
	}

	if r.Spec.InfrastructureTemplate.Namespace == "" {
		r.Spec.InfrastructureTemplate.Namespace = r.Namespace
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *KubeadmControlPlane) ValidateCreate() error {
	var allErrs field.ErrorList

	if r.Spec.Replicas == nil {
		allErrs = append(
			allErrs,
			field.Required(
				field.NewPath("spec", "replicas"),
				"is required",
			),
		)
	} else if *r.Spec.Replicas <= 0 {
		// The use of the scale subresource should provide a guarantee that negative values
		// should not be accepted for this field, but since we have to validate that Replicas != 0
		// it doesn't hurt to also additionally validate for negative numbers here as well.
		allErrs = append(
			allErrs,
			field.Forbidden(
				field.NewPath("spec", "replicas"),
				"cannot be less than or equal to 0",
			),
		)
	}

	externalEtcd := false
	if r.Spec.KubeadmConfigSpec.InitConfiguration != nil {
		if r.Spec.KubeadmConfigSpec.InitConfiguration.Etcd.External != nil {
			externalEtcd = true
		}
	}

	if !externalEtcd {
		if r.Spec.Replicas != nil && *r.Spec.Replicas%2 == 0 {
			allErrs = append(
				allErrs,
				field.Forbidden(
					field.NewPath("spec", "replicas"),
					"cannot be an even number when using managed etcd",
				),
			)
		}
	}

	if r.Spec.InfrastructureTemplate.Namespace != r.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "infrastructureTemplate", "namespace"),
				r.Spec.InfrastructureTemplate.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(GroupVersion.WithKind("KubeadmControlPlane").GroupKind(), r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *KubeadmControlPlane) ValidateUpdate(old runtime.Object) error {
	var allErrs field.ErrorList

	oldKubeadmControlPlane := old.(*KubeadmControlPlane)
	if !reflect.DeepEqual(r.Spec.KubeadmConfigSpec, oldKubeadmControlPlane.Spec.KubeadmConfigSpec) {
		allErrs = append(
			allErrs,
			field.Forbidden(
				field.NewPath("spec", "kubeadmConfigSpec"),
				"cannot be modified",
			),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(GroupVersion.WithKind("KubeadmControlPlane").GroupKind(), r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *KubeadmControlPlane) ValidateDelete() error {
	return nil
}
