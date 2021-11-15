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
	"strings"

	"github.com/blang/semver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/version"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// SetupWebhookWithManager sets up Cluster webhooks.
func (webhook *Cluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-cluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=validation.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-cluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=default.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// Cluster implements a validating and defaulting webhook for Cluster.
type Cluster struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &Cluster{}
var _ webhook.CustomValidator = &Cluster{}

// Default satisfies the defaulting webhook interface.
func (webhook *Cluster) Default(_ context.Context, obj runtime.Object) error {
	cluster, ok := obj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}

	if cluster.Spec.InfrastructureRef != nil && len(cluster.Spec.InfrastructureRef.Namespace) == 0 {
		cluster.Spec.InfrastructureRef.Namespace = cluster.Namespace
	}

	if cluster.Spec.ControlPlaneRef != nil && len(cluster.Spec.ControlPlaneRef.Namespace) == 0 {
		cluster.Spec.ControlPlaneRef.Namespace = cluster.Namespace
	}

	// If the Cluster uses a managed topology
	if cluster.Spec.Topology != nil {
		// tolerate version strings without a "v" prefix: prepend it if it's not there
		if !strings.HasPrefix(cluster.Spec.Topology.Version, "v") {
			cluster.Spec.Topology.Version = "v" + cluster.Spec.Topology.Version
		}
	}
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Cluster) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	cluster, ok := obj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}
	return webhook.validate(ctx, nil, cluster)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Cluster) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newCluster, ok := newObj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", newObj))
	}
	oldCluster, ok := oldObj.(*clusterv1.Cluster)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", oldObj))
	}
	return webhook.validate(ctx, oldCluster, newCluster)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Cluster) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}

func (webhook *Cluster) validate(ctx context.Context, old, new *clusterv1.Cluster) error {
	var allErrs field.ErrorList
	if new.Spec.InfrastructureRef != nil && new.Spec.InfrastructureRef.Namespace != new.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "infrastructureRef", "namespace"),
				new.Spec.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if new.Spec.ControlPlaneRef != nil && new.Spec.ControlPlaneRef.Namespace != new.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "controlPlaneRef", "namespace"),
				new.Spec.ControlPlaneRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	// Validate the managed topology, if defined.
	if new.Spec.Topology != nil {
		if topologyErrs := webhook.validateTopology(ctx, old, new); len(topologyErrs) > 0 {
			allErrs = append(allErrs, topologyErrs...)
		}
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), new.Name, allErrs)
}

func (webhook *Cluster) validateTopology(ctx context.Context, old, new *clusterv1.Cluster) field.ErrorList {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the web hook
	// must prevent the usage of Cluster.Topology in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return field.ErrorList{
			field.Forbidden(
				field.NewPath("spec", "topology"),
				"can be set only if the ClusterTopology feature flag is enabled",
			),
		}
	}

	var allErrs field.ErrorList

	// class should be defined.
	if len(new.Spec.Topology.Class) == 0 {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "topology", "class"),
				new.Spec.Topology.Class,
				"cannot be empty",
			),
		)
	}

	// version should be valid.
	if !version.KubeSemver.MatchString(new.Spec.Topology.Version) {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "topology", "version"),
				new.Spec.Topology.Version,
				"must be a valid semantic version",
			),
		)
	}

	// MachineDeployment names must be unique.
	if new.Spec.Topology.Workers != nil {
		names := sets.String{}
		for _, md := range new.Spec.Topology.Workers.MachineDeployments {
			if names.Has(md.Name) {
				allErrs = append(allErrs,
					field.Invalid(
						field.NewPath("spec", "topology", "workers", "machineDeployments"),
						md,
						fmt.Sprintf("MachineDeployment names should be unique. MachineDeployment with name %q is defined more than once.", md.Name),
					),
				)
			}
			names.Insert(md.Name)
		}
	}

	if old != nil { // On update
		// Class could not be mutated.
		if new.Spec.Topology.Class != old.Spec.Topology.Class {
			allErrs = append(
				allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "class"),
					new.Spec.Topology.Class,
					"class cannot be changed",
				),
			)
		}

		// Version could only be increased.
		inVersion, err := semver.ParseTolerant(new.Spec.Topology.Version)
		if err != nil {
			allErrs = append(
				allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "version"),
					new.Spec.Topology.Version,
					"is not a valid version",
				),
			)
		}
		oldVersion, err := semver.ParseTolerant(old.Spec.Topology.Version)
		if err != nil {
			// NOTE: this should never happen. Nevertheless, handling this for extra caution.
			allErrs = append(
				allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "version"),
					new.Spec.Topology.Class,
					"cannot be compared with the old version",
				),
			)
		}
		if inVersion.NE(semver.Version{}) && oldVersion.NE(semver.Version{}) && version.Compare(inVersion, oldVersion, version.WithBuildTags()) == -1 {
			allErrs = append(
				allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "version"),
					new.Spec.Topology.Version,
					"cannot be decreased",
				),
			)
		}
	}
	// Check to see if the ClusterClass referenced in the Cluster currently exists.
	if err := webhook.Client.Get(ctx, client.ObjectKey{Namespace: new.Namespace, Name: new.Spec.Topology.Class}, &clusterv1.ClusterClass{}); err != nil {
		allErrs = append(
			allErrs, field.Invalid(
				field.NewPath("spec", "topology", "class"),
				new.Spec.Topology.Class,
				"ClusterClass could not be found"))
	}
	return allErrs
}
