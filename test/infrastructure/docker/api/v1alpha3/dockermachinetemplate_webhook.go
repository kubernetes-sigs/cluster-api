/*
Copyright 2020 The Kubernetes Authors.

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
	"errors"
	"reflect"

	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (m *DockerMachineTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(m).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha3-dockermachinetemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockermachinetemplates,versions=v1alpha3,name=validation.dockermachinetemplate.infrastructure.cluster.x-k8s.io,sideEffects=None

var _ webhook.Validator = &DockerMachineTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (m *DockerMachineTemplate) ValidateCreate() error {
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (m *DockerMachineTemplate) ValidateUpdate(old runtime.Object) error {
	oldCRS := old.(*DockerMachineTemplate)
	if !reflect.DeepEqual(m.Spec, oldCRS.Spec) {
		return errors.New("DockerMachineTemplateSpec is immutable")
	}
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (m *DockerMachineTemplate) ValidateDelete() error {
	return nil
}
