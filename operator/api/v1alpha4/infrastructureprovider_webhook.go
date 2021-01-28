/*
Copyright YEAR The Kubernetes Authors.

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

package v1alpha4

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var infrastructureproviderlog = logf.Log.WithName("infrastructureprovider-resource")

func (r *InfrastructureProvider) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-operator-cluster-x-k8s-io-v1alpha4-infrastructureprovider,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=operator.cluster.x-k8s.io,resources=infrastructureproviders,versions=v1alpha4,name=default.infrastructureprovider.operator.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/validate-operator-cluster-x-k8s-io-v1alpha4-infrastructureprovider,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=operator.cluster.x-k8s.io,resources=infrastructureproviders,versions=v1alpha4,name=validation.infrastructureprovider.operator.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1beta1

var _ webhook.Defaulter = &InfrastructureProvider{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *InfrastructureProvider) Default() {
	infrastructureproviderlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

var _ webhook.Validator = &InfrastructureProvider{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *InfrastructureProvider) ValidateCreate() error {
	infrastructureproviderlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *InfrastructureProvider) ValidateUpdate(old runtime.Object) error {
	infrastructureproviderlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *InfrastructureProvider) ValidateDelete() error {
	infrastructureproviderlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
