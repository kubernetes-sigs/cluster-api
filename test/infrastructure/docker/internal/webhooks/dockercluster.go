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
	"slices"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

// DockerCluster implements a validating and defaulting webhook for DockerCluster.
type DockerCluster struct{}

func (webhook *DockerCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrav1.DockerCluster{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-dockercluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters,versions=v1beta2,name=default.dockercluster.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

var _ admission.Defaulter[*infrav1.DockerCluster] = &DockerCluster{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *DockerCluster) Default(_ context.Context, cluster *infrav1.DockerCluster) error {
	defaultDockerClusterSpec(&cluster.Spec)
	return nil
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-dockercluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters,versions=v1beta2,name=validation.dockercluster.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

var _ admission.Validator[*infrav1.DockerCluster] = &DockerCluster{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerCluster) ValidateCreate(_ context.Context, cluster *infrav1.DockerCluster) (admission.Warnings, error) {
	if allErrs := validateDockerClusterSpec(cluster.Spec); len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DockerCluster").GroupKind(), cluster.Name, allErrs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerCluster) ValidateUpdate(_ context.Context, _, cluster *infrav1.DockerCluster) (admission.Warnings, error) {
	if allErrs := validateDockerClusterSpec(cluster.Spec); len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DockerCluster").GroupKind(), cluster.Name, allErrs)
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerCluster) ValidateDelete(_ context.Context, _ *infrav1.DockerCluster) (admission.Warnings, error) {
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
