/*
Copyright 2022 The Kubernetes Authors.

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
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/version"
)

const defaultNodeDeletionTimeout = 10 * time.Second

// SetupWebhookWithManager sets up Machine webhooks.
func (webhook *Machine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.Machine{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machines,versions=v1beta1,name=validation.machine.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machine,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machines,versions=v1beta1,name=default.machine.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// Machine implements a validating and defaulting webhook for Machine.
type Machine struct {
	Client client.Reader
}

var _ webhook.CustomValidator = &Machine{}
var _ webhook.CustomDefaulter = &Machine{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *Machine) Default(_ context.Context, obj runtime.Object) error {
	machine, ok := obj.(*clusterv1.Machine)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", obj))
	}
	if machine.Labels == nil {
		machine.Labels = make(map[string]string)
	}
	machine.Labels[clusterv1.ClusterLabelName] = machine.Spec.ClusterName

	if machine.Spec.Bootstrap.ConfigRef != nil && machine.Spec.Bootstrap.ConfigRef.Namespace == "" {
		machine.Spec.Bootstrap.ConfigRef.Namespace = machine.Namespace
	}

	if machine.Spec.InfrastructureRef.Namespace == "" {
		machine.Spec.InfrastructureRef.Namespace = machine.Namespace
	}

	if machine.Spec.Version != nil && !strings.HasPrefix(*machine.Spec.Version, "v") {
		normalizedVersion := "v" + *machine.Spec.Version
		machine.Spec.Version = &normalizedVersion
	}

	if machine.Spec.NodeDeletionTimeout == nil {
		machine.Spec.NodeDeletionTimeout = &metav1.Duration{Duration: defaultNodeDeletionTimeout}
	}
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Machine) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	machine, ok := obj.(*clusterv1.Machine)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}
	return webhook.validate(ctx, nil, machine)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Machine) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newMachine, ok := newObj.(*clusterv1.Machine)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", newObj))
	}
	oldMachine, ok := oldObj.(*clusterv1.Machine)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", oldObj))
	}
	return webhook.validate(ctx, oldMachine, newMachine)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Machine) ValidateDelete(_ context.Context, _ runtime.Object) error {
	return nil
}

func (webhook *Machine) validate(_ context.Context, oldMachine, newMachine *clusterv1.Machine) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")
	if newMachine.Spec.Bootstrap.ConfigRef == nil && newMachine.Spec.Bootstrap.DataSecretName == nil {
		allErrs = append(
			allErrs,
			field.Required(
				specPath.Child("bootstrap", "data"),
				"expected either spec.bootstrap.dataSecretName or spec.bootstrap.configRef to be populated",
			),
		)
	}

	if newMachine.Spec.Bootstrap.ConfigRef != nil && newMachine.Spec.Bootstrap.ConfigRef.Namespace != newMachine.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("bootstrap", "configRef", "namespace"),
				newMachine.Spec.Bootstrap.ConfigRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if newMachine.Spec.InfrastructureRef.Namespace != newMachine.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("infrastructureRef", "namespace"),
				newMachine.Spec.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if oldMachine != nil {
		if oldMachine.Spec.ClusterName != newMachine.Spec.ClusterName {
			allErrs = append(
				allErrs,
				field.Forbidden(specPath.Child("clusterName"), "field is immutable"),
			)
		}
	}

	if newMachine.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*newMachine.Spec.Version) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("version"), *newMachine.Spec.Version, "must be a valid semantic version"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Machine").GroupKind(), newMachine.Name, allErrs)
}
