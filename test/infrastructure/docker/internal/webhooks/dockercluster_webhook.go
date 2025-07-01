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

package webhooks

import (
	"context"
	"fmt"
	"slices"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

// DockerCluster implements a validating and defaulting webhook for DockerCluster.
type DockerCluster struct{}

func (webhook *DockerCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.DockerCluster{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-dockercluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters,versions=v1beta2,name=default.dockercluster.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomDefaulter = &DockerCluster{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *DockerCluster) Default(_ context.Context, obj runtime.Object) error {
	cluster, ok := obj.(*infrav1.DockerCluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a DockerCluster but got a %T", obj))
	}
	defaultDockerClusterSpec(&cluster.Spec)
	return nil
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-dockercluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters,versions=v1beta2,name=validation.dockercluster.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomValidator = &DockerCluster{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerCluster) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cluster, ok := obj.(*infrav1.DockerCluster)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DockerCluster but got a %T", obj))
	}
	if allErrs := validateDockerClusterSpec(cluster.Spec); len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DockerCluster").GroupKind(), cluster.Name, allErrs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerCluster) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	cluster, ok := newObj.(*infrav1.DockerCluster)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DockerCluster but got a %T", newObj))
	}
	if allErrs := validateDockerClusterSpec(cluster.Spec); len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DockerCluster").GroupKind(), cluster.Name, allErrs)
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerCluster) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func defaultDockerClusterSpec(s *infrav1.DockerClusterSpec) {
	if s.ControlPlaneEndpoint.Port == 0 {
		s.ControlPlaneEndpoint.Port = 6443
	}

	for i, fd := range s.FailureDomains {
		if fd.ControlPlane == nil {
			fd.ControlPlane = ptr.To(false)
		}
		s.FailureDomains[i] = fd
	}
}

func validateDockerClusterSpec(spec infrav1.DockerClusterSpec) field.ErrorList {
	domainNames := make([]string, 0, len(spec.FailureDomains))
	for _, fd := range spec.FailureDomains {
		domainNames = append(domainNames, fd.Name)
	}
	originalDomainNames := slices.Clone(domainNames)
	sort.Strings(domainNames)
	if !slices.Equal(originalDomainNames, domainNames) {
		return field.ErrorList{field.Invalid(field.NewPath("spec", "failureDomains"), spec.FailureDomains, "failure domains must be sorted by name")}
	}
	return nil
}
