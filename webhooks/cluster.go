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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// SetupWebhookWithManager sets up Cluster webhooks.
func (c *Cluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		WithDefaulter(c).
		WithValidator(c).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-cluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=validation.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-cluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=default.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// Cluster implements a validating and defaulting webhook for Cluster.
type Cluster struct{}

var _ webhook.CustomDefaulter = &Cluster{}
var _ webhook.CustomValidator = &Cluster{}

// Default satisfies the defaulting webhook interface.
func (c *Cluster) Default(_ context.Context, obj runtime.Object) error {
	cluster, ok := obj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}

	// Note: The code in Default is intentionally not duplicated to avoid that we accidentally
	// implement new checks in the API package and forget to duplicate them to the webhook package.
	// The idea is to add new defaulting which requires a reader in the webhook package.
	// When we drop the method in the API package we must inline it here.
	cluster.Default()

	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (c *Cluster) ValidateCreate(_ context.Context, obj runtime.Object) error {
	cluster, ok := obj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}

	// Note: The code in ValidateCreate is intentionally not duplicated to avoid that we accidentally
	// implement new checks in the API package and forget to duplicate them to the webhook package.
	// The idea is to add new validation which requires a reader and Cluster variable/patch validation
	// in the webhook package.
	// When we drop the method in the API package we must inline it here.
	return cluster.ValidateCreate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (c *Cluster) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) error {
	cluster, ok := newObj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", newObj))
	}
	oldCluster, ok := oldObj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", oldObj))
	}

	// Note: The code in ValidateUpdate is intentionally not duplicated to avoid that we accidentally
	// implement new checks in the API package and forget to duplicate them to the webhook package.
	// The idea is to add new validation which requires a reader and Cluster variable/patch validation
	// in the webhook package.
	// When we drop the method in the API package we must inline it here.
	return cluster.ValidateUpdate(oldCluster)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (c *Cluster) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	cluster, ok := obj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}
	return cluster.ValidateDelete()
}
