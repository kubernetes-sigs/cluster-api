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
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/version"
)

const defaultNodeDeletionTimeout = 10 * time.Second

func (webhook *MachinePool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if webhook.decoder == nil {
		webhook.decoder = admission.NewDecoder(mgr.GetScheme())
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&expv1.MachinePool{}).
		WithDefaulter(webhook, admission.DefaulterRemoveUnknownOrOmitableFields).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machinepool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinepools,versions=v1beta1,name=validation.machinepool.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machinepool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinepools,versions=v1beta1,name=default.machinepool.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// MachinePool implements a validation and defaulting webhook for MachinePool.
type MachinePool struct {
	decoder admission.Decoder
}

var _ webhook.CustomValidator = &MachinePool{}
var _ webhook.CustomDefaulter = &MachinePool{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *MachinePool) Default(ctx context.Context, obj runtime.Object) error {
	m, ok := obj.(*expv1.MachinePool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", obj))
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}
	dryRun := false
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	var oldMP *expv1.MachinePool
	if req.Operation == v1.Update {
		oldMP = &expv1.MachinePool{}
		if err := webhook.decoder.DecodeRaw(req.OldObject, oldMP); err != nil {
			return errors.Wrapf(err, "failed to decode oldObject to MachinePool")
		}
	}

	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

	replicas, err := calculateMachinePoolReplicas(ctx, oldMP, m, dryRun)
	if err != nil {
		return err
	}

	m.Spec.Replicas = ptr.To[int32](replicas)

	if m.Spec.MinReadySeconds == nil {
		m.Spec.MinReadySeconds = ptr.To[int32](0)
	}

	if m.Spec.Template.Spec.Bootstrap.ConfigRef != nil && m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace == "" {
		m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace = m.Namespace
	}

	if m.Spec.Template.Spec.InfrastructureRef.Namespace == "" {
		m.Spec.Template.Spec.InfrastructureRef.Namespace = m.Namespace
	}

	// Set the default value for the node deletion timeout.
	if m.Spec.Template.Spec.NodeDeletionTimeout == nil {
		m.Spec.Template.Spec.NodeDeletionTimeout = &metav1.Duration{Duration: defaultNodeDeletionTimeout}
	}

	// tolerate version strings without a "v" prefix: prepend it if it's not there.
	if m.Spec.Template.Spec.Version != nil && !strings.HasPrefix(*m.Spec.Template.Spec.Version, "v") {
		normalizedVersion := "v" + *m.Spec.Template.Spec.Version
		m.Spec.Template.Spec.Version = &normalizedVersion
	}
	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachinePool) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mp, ok := obj.(*expv1.MachinePool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", obj))
	}

	return nil, webhook.validate(nil, mp)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachinePool) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldMP, ok := oldObj.(*expv1.MachinePool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", oldObj))
	}
	newMP, ok := newObj.(*expv1.MachinePool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", newObj))
	}
	return nil, webhook.validate(oldMP, newMP)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachinePool) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mp, ok := obj.(*expv1.MachinePool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", obj))
	}

	return nil, webhook.validate(nil, mp)
}

func (webhook *MachinePool) validate(oldObj, newObj *expv1.MachinePool) error {
	// NOTE: MachinePool is behind MachinePool feature gate flag; the web hook
	// must prevent creating newObj objects when the feature flag is disabled.
	specPath := field.NewPath("spec")
	if !feature.Gates.Enabled(feature.MachinePool) {
		return field.Forbidden(
			specPath,
			"can be set only if the MachinePool feature flag is enabled",
		)
	}
	var allErrs field.ErrorList
	if newObj.Spec.Template.Spec.Bootstrap.ConfigRef == nil && newObj.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		allErrs = append(
			allErrs,
			field.Required(
				specPath.Child("template", "spec", "bootstrap", "data"),
				"expected either spec.bootstrap.dataSecretName or spec.bootstrap.configRef to be populated",
			),
		)
	}

	if newObj.Spec.Template.Spec.Bootstrap.ConfigRef != nil && newObj.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace != newObj.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("template", "spec", "bootstrap", "configRef", "namespace"),
				newObj.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if newObj.Spec.Template.Spec.InfrastructureRef.Namespace != newObj.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("infrastructureRef", "namespace"),
				newObj.Spec.Template.Spec.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if oldObj != nil && oldObj.Spec.ClusterName != newObj.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Forbidden(
				specPath.Child("clusterName"),
				"field is immutable"),
		)
	}

	if newObj.Spec.Template.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*newObj.Spec.Template.Spec.Version) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("template", "spec", "version"), *newObj.Spec.Template.Spec.Version, "must be a valid semantic version"))
		}
	}

	// Validate the metadata of the MachinePool template.
	allErrs = append(allErrs, newObj.Spec.Template.ObjectMeta.Validate(specPath.Child("template", "metadata"))...)

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("MachinePool").GroupKind(), newObj.Name, allErrs)
}

func calculateMachinePoolReplicas(ctx context.Context, oldMP *expv1.MachinePool, newMP *expv1.MachinePool, dryRun bool) (int32, error) {
	// If replicas is already set => Keep the current value.
	if newMP.Spec.Replicas != nil {
		return *newMP.Spec.Replicas, nil
	}

	log := ctrl.LoggerFrom(ctx)

	// If both autoscaler annotations are set, use them to calculate the default value.
	minSizeString, hasMinSizeAnnotation := newMP.Annotations[clusterv1.AutoscalerMinSizeAnnotation]
	maxSizeString, hasMaxSizeAnnotation := newMP.Annotations[clusterv1.AutoscalerMaxSizeAnnotation]
	if hasMinSizeAnnotation && hasMaxSizeAnnotation {
		minSize, err := strconv.ParseInt(minSizeString, 10, 32)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to caculate MachinePool replicas value: could not parse the value of the %q annotation", clusterv1.AutoscalerMinSizeAnnotation)
		}
		maxSize, err := strconv.ParseInt(maxSizeString, 10, 32)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to caculate MachinePool replicas value: could not parse the value of the %q annotation", clusterv1.AutoscalerMaxSizeAnnotation)
		}

		// If it's a new MachinePool => Use the min size.
		// Note: This will result in a scale up to get into the range where autoscaler takes over.
		if oldMP == nil {
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (MP is a new MP)", minSize, clusterv1.AutoscalerMinSizeAnnotation))
			}
			return int32(minSize), nil
		}

		// Otherwise we are handing over the control for the replicas field for an existing MachinePool
		// to the autoscaler.

		switch {
		// If the old MachinePool doesn't have replicas set => Use the min size.
		// Note: As defaulting always sets the replica field, this case should not be possible
		// We only have this handling to be 100% safe against panics.
		case oldMP.Spec.Replicas == nil:
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (old MP didn't have replicas set)", minSize, clusterv1.AutoscalerMinSizeAnnotation))
			}
			return int32(minSize), nil
		// If the old MachinePool replicas are lower than min size => Use the min size.
		// Note: This will result in a scale up to get into the range where autoscaler takes over.
		case *oldMP.Spec.Replicas < int32(minSize):
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (old MP had replicas below min size)", minSize, clusterv1.AutoscalerMinSizeAnnotation))
			}
			return int32(minSize), nil
		// If the old MachinePool replicas are higher than max size => Use the max size.
		// Note: This will result in a scale down to get into the range where autoscaler takes over.
		case *oldMP.Spec.Replicas > int32(maxSize):
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (old MP had replicas above max size)", maxSize, clusterv1.AutoscalerMaxSizeAnnotation))
			}
			return int32(maxSize), nil
		// If the old MachinePool replicas are between min and max size => Keep the current value.
		default:
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on replicas of the old MachinePool (old MP had replicas within min size / max size range)", *oldMP.Spec.Replicas))
			}
			return *oldMP.Spec.Replicas, nil
		}
	}

	// If neither the default nor the autoscaler annotations are set => Default to 1.
	if !dryRun {
		log.V(2).Info("Replica field has been defaulted to 1")
	}
	return 1, nil
}
