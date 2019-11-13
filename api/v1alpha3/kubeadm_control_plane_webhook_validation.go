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

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1alpha3-kubeadmcontrolplane,mutating=false,failurePolicy=fail,groups=cluster.x-k8s.io,resources=kubeadmcontrolplane,versions=v1alpha3,name=validation.kubeadmcontrolplane.cluster.x-k8s.io

var _ webhook.Validator = &KubeadmControlPlane{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *KubeadmControlPlane) ValidateCreate() error {
	if r.Spec.Replicas == nil {
		return field.Required(field.NewPath("spec", "replicas"), "is required")
	}

	if *r.Spec.Replicas <= 0 {
		return field.Forbidden(field.NewPath("spec", "replicas"), "cannot be less than or equal to 0")
	}

	if r.Spec.KubeadmConfigSpec.InitConfiguration != nil {
		if r.Spec.KubeadmConfigSpec.InitConfiguration.Etcd.External == nil {
			if *r.Spec.Replicas%2 == 0 {
				return field.Forbidden(field.NewPath("spec", "replicas"), "cannot be an even number when using managed etcd")
			}
		}
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *KubeadmControlPlane) ValidateUpdate(old runtime.Object) error {
	oldKubeadmControlPlane := old.(*KubeadmControlPlane)
	if !reflect.DeepEqual(r.Spec.KubeadmConfigSpec, oldKubeadmControlPlane.Spec.KubeadmConfigSpec) {
		return field.Forbidden(field.NewPath("spec", "kubeadmConfigSpec"), "cannot be modified")
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *KubeadmControlPlane) ValidateDelete() error {
	return nil
}
