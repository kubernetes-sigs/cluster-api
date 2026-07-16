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

package admission

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/webhooks/conversion"
	"sigs.k8s.io/cluster-api/util/topology"
)

func (webhook *KubeadmConfigTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &bootstrapv1.KubeadmConfigTemplate{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		WithConverter(conversion.KubeadmConfigTemplate).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-bootstrap-cluster-x-k8s-io-v1beta2-kubeadmconfigtemplate,mutating=true,failurePolicy=fail,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigtemplates,versions=v1beta2,name=default.kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:verbs=create;update,path=/validate-bootstrap-cluster-x-k8s-io-v1beta2-kubeadmconfigtemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigtemplates,versions=v1beta2,name=validation.kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

// KubeadmConfigTemplate implements a validation and defaulting webhook for KubeadmConfigTemplate.
type KubeadmConfigTemplate struct{}

var _ admission.Defaulter[*bootstrapv1.KubeadmConfigTemplate] = &KubeadmConfigTemplate{}
var _ admission.Validator[*bootstrapv1.KubeadmConfigTemplate] = &KubeadmConfigTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) Default(ctx context.Context, c *bootstrapv1.KubeadmConfigTemplate) error {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("expected an admission.Request inside context: %v", err))
	}

	if topology.IsDryRunRequest(req, c) {
		// In case of dry-run requests from the topology controller, apply defaults from older versions of CAPI
		// so we do not trigger rollouts when dealing with objects created before dropping those defaults.
		ApplyPreviousKubeadmConfigDefaults(&c.Spec.Template.Spec)
	}

	return nil
}

// ApplyPreviousKubeadmConfigDefaults defaults a KubeadmConfig with default values we used in the past.
// This is done in multiple places (webhooks and KCP controller) to ensure no rollouts are triggered now that
// we removed this defaulting from webhooks.
func ApplyPreviousKubeadmConfigDefaults(c *bootstrapv1.KubeadmConfigSpec) {
	if c.Format == "" {
		c.Format = bootstrapv1.CloudConfig
	}
	if c.InitConfiguration.NodeRegistration.ImagePullPolicy == "" {
		c.InitConfiguration.NodeRegistration.ImagePullPolicy = "IfNotPresent"
	}
	if c.JoinConfiguration.NodeRegistration.ImagePullPolicy == "" {
		c.JoinConfiguration.NodeRegistration.ImagePullPolicy = "IfNotPresent"
	}
	if c.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.IsDefined() {
		if c.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.APIVersion == "" {
			c.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.APIVersion = "client.authentication.k8s.io/v1"
		}
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) ValidateCreate(_ context.Context, c *bootstrapv1.KubeadmConfigTemplate) (admission.Warnings, error) {
	return nil, webhook.validate(&c.Spec, c.Name)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) ValidateUpdate(_ context.Context, _, newC *bootstrapv1.KubeadmConfigTemplate) (admission.Warnings, error) {
	return nil, webhook.validate(&newC.Spec, newC.Name)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) ValidateDelete(_ context.Context, _ *bootstrapv1.KubeadmConfigTemplate) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *KubeadmConfigTemplate) validate(r *bootstrapv1.KubeadmConfigTemplateSpec, name string) error {
	var allErrs field.ErrorList //nolint:prealloc // Not all paths append

	allErrs = append(allErrs, Validate(&r.Template.Spec, false, field.NewPath("spec", "template", "spec"))...)
	// Validate the metadata of the template.
	allErrs = append(allErrs, r.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))...)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(bootstrapv1.GroupVersion.WithKind("KubeadmConfigTemplate").GroupKind(), name, allErrs)
}
