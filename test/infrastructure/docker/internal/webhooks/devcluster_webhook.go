/*
Copyright 2024 The Kubernetes Authors.

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

package webhooks

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

// DevCluster implements a validating and defaulting webhook for DevCluster.
type DevCluster struct{}

func (webhook *DevCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.DevCluster{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-devcluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=devclusters,versions=v1beta2,name=default.devcluster.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomDefaulter = &DevCluster{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *DevCluster) Default(_ context.Context, obj runtime.Object) error {
	cluster, ok := obj.(*infrav1.DevCluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a DevCluster but got a %T", obj))
	}
	defaultDevClusterSpec(&cluster.Spec)
	return nil
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-devcluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=devclusters,versions=v1beta2,name=validation.devcluster.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomValidator = &DevCluster{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DevCluster) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cluster, ok := obj.(*infrav1.DevCluster)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DevCluster but got a %T", obj))
	}
	if allErrs := validateDevClusterSpec(cluster.Spec); len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DevCluster").GroupKind(), cluster.Name, allErrs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DevCluster) ValidateUpdate(_ context.Context, _, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DevCluster) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func defaultDevClusterSpec(s *infrav1.DevClusterSpec) {
	if s.Backend.InMemory == nil &&
		s.Backend.Docker == nil {
		s.Backend.Docker = &infrav1.DockerClusterBackendSpec{}
	}

	if s.Backend.Docker != nil {
		if s.ControlPlaneEndpoint.Port == 0 {
			s.ControlPlaneEndpoint.Port = 6443
		}
	}
}

func validateDevClusterSpec(_ infrav1.DevClusterSpec) field.ErrorList {
	return nil
}
