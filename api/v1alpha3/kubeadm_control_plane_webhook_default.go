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
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1alpha3-kubeadmcontrolplane,mutating=true,failurePolicy=fail,groups=cluster.x-k8s.io,resources=kubeadmcontrolplane,versions=v1alpha3,name=default.kubeadmcontrolplane.cluster.x-k8s.io

var _ webhook.Defaulter = &KubeadmControlPlane{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *KubeadmControlPlane) Default() {
	if r.Spec.Replicas == nil {
		replicas := int32(1)
		r.Spec.Replicas = &replicas
	}
}
