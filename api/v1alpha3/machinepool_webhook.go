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
	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (m *MachinePool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(m).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1alpha3-machinepool,mutating=false,failurePolicy=fail,groups=cluster.x-k8s.io,resources=machinepools,versions=v1alpha3,name=validation.machinepool.cluster.x-k8s.io
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1alpha3-machinepool,mutating=true,failurePolicy=fail,groups=cluster.x-k8s.io,resources=machinepools,versions=v1alpha3,name=default.machinepool.cluster.x-k8s.io

var _ webhook.Defaulter = &MachinePool{}
var _ webhook.Validator = &MachinePool{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (m *MachinePool) Default() {
	// TODO(juan-lee): Add machine pool implementation.
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (m *MachinePool) ValidateCreate() error {
	// TODO(juan-lee): Add machine pool implementation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (m *MachinePool) ValidateUpdate(old runtime.Object) error {
	// TODO(juan-lee): Add machine pool implementation.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (m *MachinePool) ValidateDelete() error {
	// TODO(juan-lee): Add machine pool implementation.
	return nil
}
