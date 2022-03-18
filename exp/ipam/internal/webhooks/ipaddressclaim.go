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
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
)

// SetupWebhookWithManager sets up IPAddressClaim webhooks.
func (webhook *IPAddressClaim) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ipamv1.IPAddressClaim{}).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-ipam-cluster-x-k8s-io-v1alpha1-ipaddressclaim,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,versions=v1alpha1,name=validation.ipaddressclaim.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// IPAddressClaim implements a validating webhook for IPAddressClaim.
type IPAddressClaim struct {
}

var _ webhook.CustomValidator = &IPAddressClaim{}

// ValidateCreate implements webhook.CustomValidator.
func (webhook *IPAddressClaim) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	claim, ok := obj.(*ipamv1.IPAddressClaim)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected an IPAddressClaim but got a %T", obj))
	}

	if claim.Spec.PoolRef.APIGroup == nil {
		return field.Invalid(
			field.NewPath("spec.poolRef.apiGroup"),
			claim.Spec.PoolRef.APIGroup,
			"the pool reference needs to contain a group")
	}

	return nil
}

// ValidateUpdate implements webhook.CustomValidator.
func (webhook *IPAddressClaim) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	oldClaim, ok := oldObj.(*ipamv1.IPAddressClaim)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected an IPAddressClaim but got a %T", oldObj))
	}
	newClaim, ok := newObj.(*ipamv1.IPAddressClaim)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected an IPAddressClaim but got a %T", newObj))
	}

	if !reflect.DeepEqual(oldClaim.Spec, newClaim.Spec) {
		return field.Forbidden(
			field.NewPath("spec"),
			"the spec of IPAddressClaim is immutable",
		)
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator.
func (webhook *IPAddressClaim) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}
