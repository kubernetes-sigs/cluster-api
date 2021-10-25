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

func (c *ClusterClass) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.ClusterClass{}).
		WithDefaulter(c).
		WithValidator(c).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-clusterclass,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusterclasses,versions=v1beta1,name=validation.clusterclass.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-clusterclass,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusterclasses,versions=v1beta1,name=default.clusterclass.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ClusterClass implements a validation and defaulting webhook for ClusterClass.
type ClusterClass struct{}

var _ webhook.CustomDefaulter = &ClusterClass{}
var _ webhook.CustomValidator = &ClusterClass{}

// Default implements defaulting for ClusterClass create and update.
func (c *ClusterClass) Default(_ context.Context, obj runtime.Object) error {
	clusterClass, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}

	// Note: The code in Default is intentionally not duplicated to avoid that we accidentally
	// implement new checks in the API package and forget to duplicate them to the webhook package.
	// The idea is to add new defaulting which requires a reader in the webhook package.
	// When we drop the method in the API package we must inline it here.
	clusterClass.Default()

	return nil
}

// ValidateCreate implements validation for ClusterClass create.
func (c *ClusterClass) ValidateCreate(_ context.Context, obj runtime.Object) error {
	clusterClass, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}

	// Note: The code in ValidateCreate is intentionally not duplicated to avoid that we accidentally
	// implement new checks in the API package and forget to duplicate them to the webhook package.
	// The idea is to add new validation which requires a reader and ClusterClass variable/patch validation
	// in the webhook package.
	// When we drop the method in the API package we must inline it here.
	return clusterClass.ValidateCreate()
}

// ValidateUpdate implements validation for ClusterClass update.
func (c *ClusterClass) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) error {
	clusterClass, ok := newObj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", newObj))
	}
	oldClusterClass, ok := oldObj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", oldObj))
	}

	// Note: The code in ValidateUpdate is intentionally not duplicated to avoid that we accidentally
	// implement new checks in the API package and forget to duplicate them to the webhook package.
	// The idea is to add new validation which requires a reader and ClusterClass variable/patch validation
	// in the webhook package.
	// When we drop the method in the API package we must inline it here.
	return clusterClass.ValidateUpdate(oldClusterClass)
}

// ValidateDelete implements validation for ClusterClass delete.
func (c *ClusterClass) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	clusterClass, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}

	return clusterClass.ValidateDelete()
}
