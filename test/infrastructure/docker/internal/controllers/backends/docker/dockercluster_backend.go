/*
Copyright 2024 The Kubernetes Authors.

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

// Package docker implements docker backends for DevClusters and DevMachines.
package docker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ClusterBackEndReconciler reconciles a DockerCluster object.
type ClusterBackEndReconciler struct {
	client.Client
	ContainerRuntime container.Runtime
}

// ReconcileNormal handle docker backend for DevCluster not yet deleted.
func (r *ClusterBackEndReconciler) ReconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster) (ctrl.Result, error) {
	if dockerCluster.Spec.Backend.Docker == nil {
		return ctrl.Result{}, errors.New("DockerBackendReconciler can't be called for DevCluster without a Docker backend")
	}

	// Support FailureDomains
	// In cloud providers this would likely look up which failure domains are supported and set the status appropriately.
	// In the case of Docker, failure domains don't mean much so we simply copy the Spec into the Status.
	dockerCluster.Status.FailureDomains = dockerCluster.Spec.Backend.Docker.FailureDomains

	// Create a helper for managing a docker container hosting the loadbalancer.
	externalLoadBalancer, err := docker.NewLoadBalancer(ctx, cluster,
		dockerCluster.Spec.Backend.Docker.LoadBalancer.ImageRepository,
		dockerCluster.Spec.Backend.Docker.LoadBalancer.ImageTag,
		strconv.Itoa(dockerCluster.Spec.ControlPlaneEndpoint.Port))
	if err != nil {
		v1beta1conditions.MarkFalse(dockerCluster, infrav1.LoadBalancerAvailableCondition, infrav1.LoadBalancerProvisioningFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		conditions.Set(dockerCluster, metav1.Condition{
			Type:    infrav1.DevClusterDockerLoadBalancerAvailableV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevClusterDockerLoadBalancerNotAvailableV1Beta2Reason,
			Message: fmt.Sprintf("Failed to create helper for managing the externalLoadBalancer: %v", err),
		})
		return ctrl.Result{}, errors.Wrapf(err, "failed to create helper for managing the externalLoadBalancer")
	}

	// Create the docker container hosting the load balancer.
	if err := externalLoadBalancer.Create(ctx); err != nil {
		v1beta1conditions.MarkFalse(dockerCluster, infrav1.LoadBalancerAvailableCondition, infrav1.LoadBalancerProvisioningFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		conditions.Set(dockerCluster, metav1.Condition{
			Type:    infrav1.DevClusterDockerLoadBalancerAvailableV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevClusterDockerLoadBalancerNotAvailableV1Beta2Reason,
			Message: fmt.Sprintf("Failed to create load balancer: %v", err),
		})
		return ctrl.Result{}, errors.Wrap(err, "failed to create load balancer")
	}

	// Set APIEndpoints with the load balancer IP so the Cluster API Cluster Controller can pull it
	lbIP, err := externalLoadBalancer.IP(ctx)
	if err != nil {
		v1beta1conditions.MarkFalse(dockerCluster, infrav1.LoadBalancerAvailableCondition, infrav1.LoadBalancerProvisioningFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		conditions.Set(dockerCluster, metav1.Condition{
			Type:    infrav1.DevClusterDockerLoadBalancerAvailableV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevClusterDockerLoadBalancerNotAvailableV1Beta2Reason,
			Message: fmt.Sprintf("Failed to get ip for the load balancer: %v", err),
		})
		return ctrl.Result{}, errors.Wrap(err, "failed to get ip for the load balancer")
	}

	if dockerCluster.Spec.ControlPlaneEndpoint.Host == "" {
		// Surface the control plane endpoint
		// Note: the control plane port is already set by the user or defaulted by the dockerCluster webhook.
		dockerCluster.Spec.ControlPlaneEndpoint.Host = lbIP
	}

	// Mark the dockerCluster ready
	dockerCluster.Status.Ready = true
	v1beta1conditions.MarkTrue(dockerCluster, infrav1.LoadBalancerAvailableCondition)
	conditions.Set(dockerCluster, metav1.Condition{
		Type:   infrav1.DevClusterDockerLoadBalancerAvailableV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: infrav1.DevClusterDockerLoadBalancerAvailableV1Beta2Reason,
	})

	return ctrl.Result{}, nil
}

// ReconcileDelete handle docker backend for delete DevMachines.
func (r *ClusterBackEndReconciler) ReconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster) (ctrl.Result, error) {
	if dockerCluster.Spec.Backend.Docker == nil {
		return ctrl.Result{}, errors.New("DockerBackendReconciler can't be called for DevClusters without a Docker backend")
	}

	// Create a helper for managing a docker container hosting the loadbalancer.
	externalLoadBalancer, err := docker.NewLoadBalancer(ctx, cluster,
		dockerCluster.Spec.Backend.Docker.LoadBalancer.ImageRepository,
		dockerCluster.Spec.Backend.Docker.LoadBalancer.ImageTag,
		strconv.Itoa(dockerCluster.Spec.ControlPlaneEndpoint.Port))
	if err != nil {
		v1beta1conditions.MarkFalse(dockerCluster, infrav1.LoadBalancerAvailableCondition, infrav1.LoadBalancerProvisioningFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		conditions.Set(dockerCluster, metav1.Condition{
			Type:    infrav1.DevClusterDockerLoadBalancerAvailableV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevClusterDockerLoadBalancerNotAvailableV1Beta2Reason,
			Message: fmt.Sprintf("Failed to create helper for managing the externalLoadBalancer: %v", err),
		})

		return ctrl.Result{}, errors.Wrapf(err, "failed to create helper for managing the externalLoadBalancer")
	}

	// Set the LoadBalancerAvailableCondition reporting delete is started, and requeue in order to make
	// this visible to the users.
	// TODO (v1beta2): test for v1beta2 conditions
	if v1beta1conditions.GetReason(dockerCluster, infrav1.LoadBalancerAvailableCondition) != clusterv1.DeletingV1Beta1Reason {
		v1beta1conditions.MarkFalse(dockerCluster, infrav1.LoadBalancerAvailableCondition, clusterv1.DeletingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
		conditions.Set(dockerCluster, metav1.Condition{
			Type:   infrav1.DevClusterDockerLoadBalancerAvailableV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevClusterDockerLoadBalancerDeletingV1Beta2Reason,
		})
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	// Delete the docker container hosting the load balancer
	if err := externalLoadBalancer.Delete(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to delete load balancer")
	}

	// Cluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(dockerCluster, infrav1.ClusterFinalizer)

	return ctrl.Result{}, nil
}

// PatchDevCluster patch a DevCluster.
func (r *ClusterBackEndReconciler) PatchDevCluster(ctx context.Context, patchHelper *patch.Helper, dockerCluster *infrav1.DevCluster) error {
	if dockerCluster.Spec.Backend.Docker == nil {
		return errors.New("DockerBackendReconciler can't be called for DevClusters without a Docker backend")
	}

	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding it during the deletion process).
	v1beta1conditions.SetSummary(dockerCluster,
		v1beta1conditions.WithConditions(
			infrav1.LoadBalancerAvailableCondition,
		),
		v1beta1conditions.WithStepCounterIf(dockerCluster.DeletionTimestamp.IsZero()),
	)
	if err := conditions.SetSummaryCondition(dockerCluster, dockerCluster, infrav1.DevClusterReadyV1Beta2Condition,
		conditions.ForConditionTypes{
			infrav1.DevClusterDockerLoadBalancerAvailableV1Beta2Condition,
		},
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				// Use custom reasons.
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					infrav1.DevClusterNotReadyV1Beta2Reason,
					infrav1.DevClusterReadyUnknownV1Beta2Reason,
					infrav1.DevClusterReadyV1Beta2Reason,
				)),
			),
		},
	); err != nil {
		return errors.Wrapf(err, "failed to set %s condition", infrav1.DevClusterReadyV1Beta2Condition)
	}

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		dockerCluster,
		patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyV1Beta1Condition,
			infrav1.LoadBalancerAvailableCondition,
		}},
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			infrav1.DevClusterReadyV1Beta2Condition,
			infrav1.DevClusterDockerLoadBalancerAvailableV1Beta2Condition,
		}},
	)
}
