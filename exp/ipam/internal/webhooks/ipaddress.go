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
	"net/netip"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/util/compare"
)

// SetupWebhookWithManager sets up IPAddress webhooks.
func (webhook *IPAddress) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ipamv1.IPAddress{}).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-ipam-cluster-x-k8s-io-v1beta1-ipaddress,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=ipaddresses,versions=v1beta1,name=validation.ipaddress.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch

// IPAddress implements a validating webhook for IPAddress.
type IPAddress struct {
	Client client.Reader
}

var _ webhook.CustomValidator = &IPAddress{}

// ValidateCreate implements webhook.CustomValidator.
func (webhook *IPAddress) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ip, ok := obj.(*ipamv1.IPAddress)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an IPAddress but got a %T", obj))
	}
	return nil, webhook.validate(ctx, ip)
}

// ValidateUpdate implements webhook.CustomValidator.
func (webhook *IPAddress) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldIP, ok := oldObj.(*ipamv1.IPAddress)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an IPAddress but got a %T", oldObj))
	}
	newIP, ok := newObj.(*ipamv1.IPAddress)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an IPAddress but got a %T", newObj))
	}

	equal, diff, err := compare.Diff(oldIP.Spec, newIP.Spec)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to compare old and new IPAddress spec: %v", err))
	}
	if !equal {
		return nil, field.Forbidden(field.NewPath("spec"), fmt.Sprintf("IPAddress spec is immutable. Diff: %s", diff))
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator.
func (webhook *IPAddress) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *IPAddress) validate(ctx context.Context, ip *ipamv1.IPAddress) error {
	log := ctrl.LoggerFrom(ctx)
	allErrs := field.ErrorList{}
	specPath := field.NewPath("spec")

	addr, err := netip.ParseAddr(ip.Spec.Address)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				specPath.Child("address"),
				ip.Spec.Address,
				"not a valid IP address",
			))
	}

	if ip.Spec.Prefix < 0 {
		allErrs = append(allErrs,
			field.Invalid(
				specPath.Child("prefix"),
				ip.Spec.Prefix,
				"prefix cannot be negative",
			))
	}
	if addr.Is4() && ip.Spec.Prefix > 32 {
		allErrs = append(allErrs,
			field.Invalid(
				specPath.Child("prefix"),
				ip.Spec.Prefix,
				"prefix is too large for an IPv4 address",
			))
	}
	if addr.Is6() && ip.Spec.Prefix > 128 {
		allErrs = append(allErrs,
			field.Invalid(
				specPath.Child("prefix"),
				ip.Spec.Prefix,
				"prefix is too large for an IPv6 address",
			))
	}

	if ip.Spec.Gateway != "" {
		if _, err := netip.ParseAddr(ip.Spec.Gateway); err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					specPath.Child("gateway"),
					ip.Spec.Gateway,
					"not a valid IP address",
				))
		}
	}

	if ip.Spec.PoolRef.APIGroup == nil {
		allErrs = append(allErrs,
			field.Invalid(
				specPath.Child("poolRef.apiGroup"),
				ip.Spec.PoolRef.APIGroup,
				"the pool reference needs to contain a group"))
	}

	claim := &ipamv1.IPAddressClaim{}
	err = webhook.Client.Get(ctx, types.NamespacedName{Name: ip.Spec.ClaimRef.Name, Namespace: ip.ObjectMeta.Namespace}, claim)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to fetch claim", "IPAddressClaim", klog.KRef(ip.ObjectMeta.Namespace, ip.Spec.ClaimRef.Name))
		allErrs = append(allErrs,
			field.InternalError(
				specPath.Child("claimRef"),
				errors.Wrap(err, "failed to fetch claim"),
			),
		)
	}

	if claim.Name != "" && // only report non-matching pool if the claim exists
		!(ip.Spec.PoolRef.APIGroup != nil && claim.Spec.PoolRef.APIGroup != nil &&
			*ip.Spec.PoolRef.APIGroup == *claim.Spec.PoolRef.APIGroup &&
			ip.Spec.PoolRef.Kind == claim.Spec.PoolRef.Kind &&
			ip.Spec.PoolRef.Name == claim.Spec.PoolRef.Name) {
		allErrs = append(allErrs,
			field.Invalid(
				specPath.Child("poolRef"),
				ip.Spec.PoolRef,
				"the referenced pool is different from the pool referenced by the claim this address should fulfill",
			))
	}

	return allErrs.ToAggregate()
}
