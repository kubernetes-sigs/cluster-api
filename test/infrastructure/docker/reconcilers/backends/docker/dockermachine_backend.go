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

package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/internal/util/taints"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/labels"
	"sigs.k8s.io/cluster-api/util/patch"
)

// MachineBackendReconciler reconciles a DockerMachine object.
type MachineBackendReconciler struct {
	client.Client
	ContainerRuntime container.Runtime
	ClusterCache     clustercache.ClusterCache
	TaskManager      *TaskManager

	DeferNextReconcileForObject func(obj metav1.Object, reconcileAfter time.Time)
}

// ReconcileNormal handle docker backend for DevMachines not yet deleted.
func (r *MachineBackendReconciler) ReconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster, machine *clusterv1.Machine, dockerMachine *infrav1.DevMachine) (res ctrl.Result, retErr error) {
	if dockerMachine.Spec.Backend.Docker == nil {
		return ctrl.Result{}, pkgerrors.New("DockerBackendReconciler can't be called for DevMachines without a Docker backend")
	}
	if dockerCluster.Spec.Backend.Docker == nil {
		return ctrl.Result{}, pkgerrors.New("DockerBackendReconciler can't be called for DevCluster without a Docker backend")
	}

	externalMachine, externalLoadBalancer, err := r.getExternalObjects(ctx, cluster, dockerCluster, machine, dockerMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	log := ctrl.LoggerFrom(ctx).WithValues("container", externalMachine.ContainerName())
	ctx = ctrl.LoggerInto(ctx, log)

	var dataSecretName *string
	var version string

	if labels.IsMachinePoolOwned(dockerMachine) {
		machinePool, err := util.GetMachinePoolByLabels(ctx, r.Client, dockerMachine.GetNamespace(), dockerMachine.Labels)
		if err != nil {
			return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to get machine pool for DockerMachine %s/%s", dockerMachine.GetNamespace(), dockerMachine.GetName())
		}
		if machinePool == nil {
			log.Info("No MachinePool matching labels found, returning without error")
			return ctrl.Result{}, nil
		}

		dataSecretName = machinePool.Spec.Template.Spec.Bootstrap.DataSecretName
		version = machinePool.Spec.Template.Spec.Version
	} else {
		dataSecretName = machine.Spec.Bootstrap.DataSecretName
		version = machine.Spec.Version
	}

	// Reconcile the docker container for this machine.
	if result, err := r.reconcileContainer(ctx, cluster, machine, dataSecretName, dockerMachine, externalMachine); err != nil || !result.IsZero() {
		return result, err
	}

	// If this is a control plane machine, reconcile the load balancer configuration for this machine.
	if util.IsControlPlaneMachine(machine) {
		if result, err := r.reconcileLoadBalancer(ctx, cluster, dockerCluster, machine, dockerMachine, externalLoadBalancer); err != nil || !result.IsZero() {
			return result, err
		}
	}

	// Reconcile CGroups in the docker container for this machine are started, which is a requirement for completing machine provisioning.
	// Note: the operation is executed asynchronously so we don't block the reconcile loop.
	if result, err := r.reconcileCGroups(ctx, dockerMachine, externalMachine); err != nil || !result.IsZero() {
		return result, err
	}

	// Reconcile preloaded images into docker container for this machine.
	// Note: the operation is executed asynchronously so we don't block the reconcile loop.
	if result, err := r.reconcilePreLoadedImages(ctx, dockerMachine, externalMachine); err != nil || !result.IsZero() {
		return result, err
	}

	// If provisioning of docker container for this machine is completed, surface it.
	if conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerContainerProvisionedCondition) &&
		conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerCGroupsReadyCondition) &&
		conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerPreLoadedImagesReadyCondition) {
		dockerMachine.Spec.ProviderID = externalMachine.ProviderID()
		dockerMachine.Status.Initialization.Provisioned = ptr.To(true)
	}

	// Following task do not exist in a real infrastructure provider, they are specific of the docker backend only.
	// Note: In order to mimic the behavior of a real infrastructure provider, following tasks are not considering
	// when determining if provisioning of docker container for this machine is completed.

	// Since there is no boostrap script that runs inside the container on startup,
	// it is required to implement an alternative way for running the code that transforms the docker container into
	// a Kubernetes node (e.g. kubeadm init or kubeadm join).
	// The following function solves this problem by parsing the bootstrap data secret and extracting from it
	// a sequence of "docker exec" commands that will be executed asynchronously so we don't block the reconcile loop.
	if result, err := r.reconcileBootstrap(ctx, machine, dataSecretName, version, dockerMachine, externalMachine); err != nil || !result.IsZero() {
		return result, err
	}

	// Since we are not deploying CPI component on Clusters using the docker backend,
	// it is required to implement an alternative way for performing the tasks that this component usually takes care of.
	// The following function solves this problem taking care of setting spec.providerID and managing the uninitialized
	// taint on the Kubernetes node that is hosted on the docker container.
	if result, err := r.reconcileNode(ctx, cluster, dockerMachine, externalMachine); err != nil || !result.IsZero() {
		return result, err
	}
	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileContainer(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, dataSecretName *string, dockerMachine *infrav1.DevMachine, externalMachine *docker.Machine) (ctrl.Result, error) { //nolint:unparam
	log := ctrl.LoggerFrom(ctx)

	// if the machine has been already provisioned, but the container not running anymore, surface it
	if dockerMachine.Spec.ProviderID != "" && !externalMachine.IsRunning() {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineDockerContainerProvisionedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerContainerNotProvisionedReason,
			Message: fmt.Sprintf("Container %s is not running anymore", externalMachine.ContainerName()),
		})
		return ctrl.Result{}, nil
	}

	// no-op if already completed.
	if conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerContainerProvisionedCondition) {
		return ctrl.Result{}, nil
	}

	// Check if the cluster infrastructure is ready, if not, wait.
	if !ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
		log.Info("Waiting for DockerCluster Controller to create cluster infrastructure")
		v1beta1conditions.MarkFalse(dockerMachine, infrav1.ContainerProvisionedV1Beta1Condition, infrav1.WaitingForClusterInfrastructureV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerContainerProvisionedCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineDockerContainerWaitingForClusterInfrastructureReadyReason,
		})
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data is available and populated.
	if dataSecretName == nil {
		if !util.IsControlPlaneMachine(machine) && !conditions.IsTrue(cluster, clusterv1.ClusterControlPlaneInitializedCondition) {
			log.Info("Waiting for the control plane to be initialized")
			conditions.Set(dockerMachine, metav1.Condition{
				Type:   infrav1.DevMachineDockerContainerProvisionedCondition,
				Status: metav1.ConditionFalse,
				Reason: infrav1.DevMachineDockerContainerWaitingForControlPlaneInitializedReason,
			})
			return ctrl.Result{}, nil
		}

		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerContainerProvisionedCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineDockerContainerWaitingForBootstrapDataReason,
		})
		return ctrl.Result{}, nil
	}

	// Handle best effort failures in creating the container.
	if externalMachine.Exists() && !externalMachine.IsRunning() {
		// This deletes the machine and results in re-creating it below.
		if err := externalMachine.Delete(ctx); err != nil {
			return ctrl.Result{}, pkgerrors.Wrap(err, "Failed to delete not running DockerMachine")
		}
	}

	// Create the machine if not existing yet
	if !externalMachine.Exists() {
		role := constants.WorkerNodeRoleValue
		if util.IsControlPlaneMachine(machine) {
			role = constants.ControlPlaneNodeRoleValue
		}

		// NOTE: FailureDomains don't mean much in CAPD since it's all local, but we are setting a label on
		// each container, so we can check placement.
		log.Info("Creating container")
		if err := externalMachine.Create(ctx, dockerMachine.Spec.Backend.Docker.CustomImage, role, machine.Spec.Version, docker.FailureDomainLabel(machine.Spec.FailureDomain), dockerMachine.Spec.Backend.Docker.ExtraMounts); err != nil {
			return ctrl.Result{}, pkgerrors.Wrap(err, "failed to create worker DockerMachine")
		}

		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineDockerContainerProvisionedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerContainerNotProvisionedReason,
			Message: fmt.Sprintf("Creating container %s", externalMachine.ContainerName()),
		})
		return ctrl.Result{}, nil
	}

	conditions.Set(dockerMachine, metav1.Condition{
		Type:   infrav1.DevMachineDockerContainerProvisionedCondition,
		Status: metav1.ConditionTrue,
		Reason: infrav1.DevMachineDockerContainerProvisionedReason,
	})

	// Surface machine address.
	if err := setMachineAddress(ctx, dockerMachine, externalMachine); err != nil {
		return ctrl.Result{}, pkgerrors.Wrap(err, "failed to set the machine address")
	}

	// Surface failure domain.
	// TODO: read failure domains from container labels instead of mirroring from spec.
	dockerMachine.Status.FailureDomain = machine.Spec.FailureDomain

	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileLoadBalancer(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster, machine *clusterv1.Machine, dockerMachine *infrav1.DevMachine, externalLoadBalancer *docker.LoadBalancer) (ctrl.Result, error) { //nolint:unparam
	// if container is not ready yet, wait
	if !conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerContainerProvisionedCondition) {
		return ctrl.Result{}, nil
	}

	// Update the load balancer configuration for the docker container.
	// Note: the annotation is used to track when configuration is updated:
	// - not set or empty -> the docker container is not yet configured as a load balancer backend
	// - devmachine.infrastructure.cluster.x-k8s.io/weight: 100 -> the docker container is configured as load balancer backend
	// - devmachine.infrastructure.cluster.x-k8s.io/weight: 0 -> the docker container is configured as load balancer backend, but stopped to receive traffic (machine deleting)
	// Note: reconcileLoadBalancerConfiguration does not use the annotation, it looks at machine list.
	if dockerMachine.Annotations[infrav1.LoadbalancerWeightAnnotation] == "" {
		if err := r.reconcileLoadBalancerConfiguration(ctx, cluster, dockerCluster, externalLoadBalancer); err != nil {
			return ctrl.Result{}, err
		}
		if dockerMachine.Annotations == nil {
			dockerMachine.Annotations = map[string]string{}
		}
		dockerMachine.Annotations[infrav1.LoadbalancerWeightAnnotation] = "100"
	}

	// if the corresponding machine is deleted but the docker machine not yet, update load balancer configuration to divert all traffic from the docker container.
	// Note: The condition below express "the dockerMachine is going to be deleted", and we want to make this change (divert traffic) as soon as possible,
	// without waiting for the actual dockerMachine deletion that only happens at the end of the delete sequence, after drain, detach volumes, deletion hooks etc.
	if !machine.DeletionTimestamp.IsZero() && dockerMachine.DeletionTimestamp.IsZero() {
		if dockerMachine.Annotations[infrav1.LoadbalancerWeightAnnotation] == "100" {
			if err := r.reconcileLoadBalancerConfiguration(ctx, cluster, dockerCluster, externalLoadBalancer); err != nil {
				return ctrl.Result{}, err
			}
		}
		if dockerMachine.Annotations == nil {
			dockerMachine.Annotations = map[string]string{}
		}
		dockerMachine.Annotations[infrav1.LoadbalancerWeightAnnotation] = "0"
	}
	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileCGroups(ctx context.Context, dockerMachine *infrav1.DevMachine, externalMachine *docker.Machine) (ctrl.Result, error) { //nolint:unparam
	const WaitCGroupsTask = "WaitCGroups"

	// no-op if already completed.
	if conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerCGroupsReadyCondition) {
		return ctrl.Result{}, nil
	}

	// if provisioning already completed, no-op
	// Note: this prevents to re-run the operation after clusterctl move / backup-restore.
	if dockerMachine.Spec.ProviderID != "" {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerCGroupsReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.DevMachineDockerCGroupsReadyReason,
		})
		return ctrl.Result{}, nil
	}

	// if container is not ready yet, wait
	if !conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerContainerProvisionedCondition) {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerCGroupsReadyCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineDockerCGroupsReadyWaitingForContainerReason,
		})
		return ctrl.Result{}, nil
	}

	// run the WaitCGroupsTask or wait for its completion.
	taskState := r.TaskManager.GetStatus(dockerMachine, WaitCGroupsTask)
	switch {
	case taskState == nil:
		// WaitCGroupsTask not yet performed, start it.
		var err error
		taskState, err = r.TaskManager.RegisterTask(ctx, dockerMachine, WaitCGroupsTask, []Operation{
			{
				Description: "Waiting for cgroups ready",
				F: func(ctx context.Context) error {
					log := ctrl.LoggerFrom(ctx)
					log.Info("Checking if the container performed the multi-user systemd target")
					return externalMachine.WaitForMultiUserTarget(ctx, r.ContainerRuntime)
				},
			},
		}, 30*time.Second)
		if err != nil {
			conditions.Set(dockerMachine, metav1.Condition{
				Type:    infrav1.DevMachineDockerCGroupsReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineDockerCGroupsReadyInternalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return ctrl.Result{}, err
		}
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineDockerCGroupsReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerCGroupsNotReadyReason,
			Message: taskState.String(),
		})
	case !taskState.Completed && taskState.Err == nil:
		// WaitCGroupsTask in progress, report state.
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineDockerCGroupsReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerCGroupsNotReadyReason,
			Message: taskState.String(),
		})
	case taskState.Err != nil:
		// WaitCGroupsTask failed, retry.
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineDockerCGroupsReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerCGroupsNotReadyReason,
			Message: taskState.String(),
		})
		r.TaskManager.ResetStatus(dockerMachine, WaitCGroupsTask)
		r.DeferNextReconcileForObject(dockerMachine, time.Now().Add(5*time.Second))
	case taskState.Completed:
		// WaitCGroupsTask completed, report state.
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerCGroupsReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.DevMachineDockerCGroupsReadyReason,
		})
	}
	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcilePreLoadedImages(ctx context.Context, dockerMachine *infrav1.DevMachine, externalMachine *docker.Machine) (ctrl.Result, error) { //nolint:unparam
	const PreLoadImagesTask = "PreLoadImages"

	// no-op if already completed.
	if conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerPreLoadedImagesReadyCondition) {
		return ctrl.Result{}, nil
	}

	// if provisioning already completed, no-op
	// Note: this prevents to re-run the operation after clusterctl move / backup-restore.
	if dockerMachine.Spec.ProviderID != "" {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.DevMachineDockerPreLoadedImagesReadyReason,
		})
		return ctrl.Result{}, nil
	}

	// if container is not ready yet, wait
	if !conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerContainerProvisionedCondition) {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineDockerPreLoadedImagesReadyWaitingForContainerReason,
		})
		return ctrl.Result{}, nil
	}

	// if CGroups are not ready yet, wait
	if !conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerCGroupsReadyCondition) {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineDockerPreLoadedImagesReadyWaitingForCGroupsReason,
		})
		return ctrl.Result{}, nil
	}

	if len(dockerMachine.Spec.Backend.Docker.PreLoadImages) == 0 {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.DevMachineDockerPreLoadedImagesReadyReason,
		})
		return ctrl.Result{}, nil
	}

	// run the PreLoadImagesTask or wait for its completion.
	taskState := r.TaskManager.GetStatus(dockerMachine, PreLoadImagesTask)
	switch {
	case taskState == nil:
		// PreLoadImagesTask not it performed, start it.
		operations := make([]Operation, 0, len(dockerMachine.Spec.Backend.Docker.PreLoadImages))
		for _, image := range dockerMachine.Spec.Backend.Docker.PreLoadImages {
			operations = append(operations, Operation{
				Description: fmt.Sprintf("Pre-loading image %s", image),
				F: func(ctx context.Context) error {
					log := ctrl.LoggerFrom(ctx)
					log.Info("Pre-loading image", "image", image)
					return externalMachine.PreloadLoadImage(ctx, image)
				},
			})
		}
		var err error
		taskState, err = r.TaskManager.RegisterTask(ctx, dockerMachine, PreLoadImagesTask, operations, 5*time.Minute)
		if err != nil {
			conditions.Set(dockerMachine, metav1.Condition{
				Type:    infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineDockerPreLoadedImagesReadyInternalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return ctrl.Result{}, err
		}
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerPreLoadedImagesNotReadyReason,
			Message: taskState.String(),
		})
	case !taskState.Completed && taskState.Err == nil:
		// PreLoadImagesTask in progress, report state.
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerPreLoadedImagesNotReadyReason,
			Message: taskState.String(),
		})
	case taskState.Err != nil:
		// PreLoadImagesTask failed, retry.
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerPreLoadedImagesNotReadyReason,
			Message: taskState.String(),
		})
		r.TaskManager.ResetStatus(dockerMachine, PreLoadImagesTask)
		r.DeferNextReconcileForObject(dockerMachine, time.Now().Add(5*time.Second))
	case taskState.Completed:
		// PreLoadImagesTask completed, report state.
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.DevMachineDockerPreLoadedImagesReadyReason,
		})
	}
	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileBootstrap(ctx context.Context, machine *clusterv1.Machine, dataSecretName *string, version string, dockerMachine *infrav1.DevMachine, externalMachine *docker.Machine) (ctrl.Result, error) { //nolint:unparam
	const CloudInitOrIgnitionTask = "CloudInitOrIgnition"
	log := ctrl.LoggerFrom(ctx)

	// no-op if already completed or failed.
	if conditions.IsTrue(dockerMachine, infrav1.DevMachineBootstrapCompletedCondition) ||
		conditions.GetReason(dockerMachine, infrav1.DevMachineBootstrapCompletedCondition) == infrav1.DevMachineDockerBootstrapFailedReason {
		return ctrl.Result{}, nil
	}

	// if node ref already set, no-op
	// Note: this prevents to re-run the operation after clusterctl move / backup-restore,
	// even if this mechanism does not work if node ref is not set, but this limitation is considered acceptable for CAPdev.
	if machine.Status.NodeRef.Name != "" {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineBootstrapCompletedCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.DevMachineDockerBootstrapCompletedReason,
		})
		return ctrl.Result{}, nil
	}

	// if container is not ready yet, wait
	if !conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerContainerProvisionedCondition) {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineBootstrapCompletedCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineDockerBootstrapCompletedWaitingForContainerReason,
		})
		return ctrl.Result{}, nil
	}

	// if CGroups are not ready yet, wait
	if !conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerCGroupsReadyCondition) {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineBootstrapCompletedCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineDockerBootstrapCompletedWaitingForCGroupsReason,
		})
		return ctrl.Result{}, nil
	}

	// if preloaded images are not ready yet, wait
	if !conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerPreLoadedImagesReadyCondition) {
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineBootstrapCompletedCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineDockerBootstrapCompletedWaitingForPreloadedImagesReason,
		})
		return ctrl.Result{}, nil
	}

	// run the CloudInitOrIgnitionTask or wait for its completion.
	taskState := r.TaskManager.GetStatus(dockerMachine, CloudInitOrIgnitionTask)
	switch {
	case taskState == nil:
		log.Info("Checking for sentinel file")

		// Before creating the command, check if the sentinel file already exists into the container to prevent running the bootstrap twice.
		// Note: this is required for the clusterctl move scenario, where there is a race between this controller
		// and the machine controller surfacing the NodeRef which is the other signal used to prevent executing bootstrap twice.
		sentinelFileExists, err := externalMachine.CheckForSentinelFile(ctx)
		if err != nil {
			conditions.Set(dockerMachine, metav1.Condition{
				Type:    infrav1.DevMachineBootstrapCompletedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineDockerBootstrapCompletedInternalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return ctrl.Result{}, err
		}
		if sentinelFileExists {
			log.Info("Sentinel file already exists, machine has been already bootstrapped")
			conditions.Set(dockerMachine, metav1.Condition{
				Type:   infrav1.DevMachineBootstrapCompletedCondition,
				Status: metav1.ConditionTrue,
				Reason: infrav1.DevMachineDockerBootstrapCompletedReason,
			})
			return ctrl.Result{}, nil
		}
		log.Info("Sentinel file does not exist, bootstrapping the machine for the first time")

		// CloudInitOrIgnitionTask not it performed, start it.

		// Fetch boostrap data for this machine.
		bootstrapData, format, err := r.getBootstrapData(ctx, dockerMachine.Namespace, *dataSecretName)
		if err != nil {
			conditions.Set(dockerMachine, metav1.Condition{
				Type:    infrav1.DevMachineBootstrapCompletedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineDockerBootstrapCompletedInternalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return ctrl.Result{}, err
		}

		// Parsing the bootstrap data and extract a sequence of "docker exec" commands from it.
		commands, err := externalMachine.GetBootstrapCommands(ctx, bootstrapData, format, version, dockerMachine.Spec.Backend.Docker.CustomImage)
		if err != nil {
			conditions.Set(dockerMachine, metav1.Condition{
				Type:    infrav1.DevMachineBootstrapCompletedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineDockerBootstrapCompletedInternalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return ctrl.Result{}, err
		}

		// Execute boostrap commands asynchronously.
		operations := make([]Operation, 0, len(commands))
		for _, command := range commands {
			operations = append(operations, boostrapCommandOperation(externalMachine, command))
		}

		timeout := 5 * time.Minute
		if dockerMachine.Spec.Backend.Docker.BootstrapTimeout != nil {
			timeout = dockerMachine.Spec.Backend.Docker.BootstrapTimeout.Duration
		}
		taskState, err = r.TaskManager.RegisterTask(ctx, dockerMachine, CloudInitOrIgnitionTask, operations, timeout)
		if err != nil {
			conditions.Set(dockerMachine, metav1.Condition{
				Type:    infrav1.DevMachineBootstrapCompletedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineDockerBootstrapCompletedInternalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return ctrl.Result{}, err
		}
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineBootstrapCompletedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerPreLoadedImagesNotReadyReason,
			Message: taskState.String(),
		})
	case !taskState.Completed && taskState.Err == nil:
		// CloudInitOrIgnitionTask in progress, report state.
		// Note: It is not possible to re-run kubeadm init/join, also we would like to triage issue in this case.
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineBootstrapCompletedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerPreLoadedImagesNotReadyReason,
			Message: taskState.String(),
		})
	case taskState.Err != nil:
		// CloudInitOrIgnitionTask failed.
		// Note: when bootstrap fails on a Machine, there is no retry.
		cmdErr := &cmdError{}
		if pkgerrors.As(taskState.Err, &cmdErr) {
			conditions.Set(dockerMachine, metav1.Condition{
				Type:   infrav1.DevMachineBootstrapCompletedCondition,
				Status: metav1.ConditionFalse,
				Reason: infrav1.DevMachineDockerBootstrapFailedReason,
				Message: fmt.Sprintf("%s\n", taskState.CurrentOperationDescription) +
					fmt.Sprintf("ERROR: %s\n", cmdErr.Err.Error()) +
					fmt.Sprintf("STDOUT: %s\n", indentIfMultiline(cmdErr.Stdout, 10)) +
					fmt.Sprintf("STDERR: %s", indentIfMultiline(cmdErr.Stderr, 5)),
			})
			return ctrl.Result{}, nil
		}
		conditions.Set(dockerMachine, metav1.Condition{
			Type:    infrav1.DevMachineBootstrapCompletedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.DevMachineDockerBootstrapNotCompletedReason,
			Message: taskState.String(),
		})
	case taskState.Completed:
		// CloudInitOrIgnitionTask completed, report state.
		conditions.Set(dockerMachine, metav1.Condition{
			Type:   infrav1.DevMachineBootstrapCompletedCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.DevMachineDockerBootstrapCompletedReason,
		})
	}
	return ctrl.Result{}, nil
}

func boostrapCommandOperation(externalMachine *docker.Machine, command provisioning.Cmd) Operation {
	commandMsg := strings.Join(append([]string{command.Cmd}, command.Args...), " ")
	return Operation{
		Description: fmt.Sprintf("Running boostrap command [ %s ]", commandMsg),
		F: func(ctx context.Context) error {
			log := ctrl.LoggerFrom(ctx)

			log.Info("Running boostrap command", "command", commandMsg)
			retry := 0
			for {
				var outErr bytes.Buffer
				var outStd bytes.Buffer

				cmd := externalMachine.Command(command.Cmd, command.Args...)
				cmd.SetStderr(&outErr)
				cmd.SetStdout(&outStd)
				if command.Stdin != "" {
					cmd.SetStdin(strings.NewReader(command.Stdin))
				}
				if err := cmd.Run(ctx); err != nil {
					stdout := outStd.String()
					stderr := outErr.String()
					log.Info("Failed running boostrap command", "command", commandMsg, "error", err.Error(), "stdout", stdout, "stderr", stderr)
					if retry < command.Retry {
						time.Sleep(5 * time.Second)
						retry++
						log.Info("Running boostrap command", "command", commandMsg, "retry", fmt.Sprintf("%d/%d", retry, command.Retry))
						continue
					}
					externalMachine.LogContainerDebugInfo(ctx)
					return &cmdError{
						Err:    pkgerrors.WithStack(err),
						Stdout: stdout,
						Stderr: stderr,
					}
				}
				break
			}
			return nil
		},
	}
}

func (r *MachineBackendReconciler) reconcileNode(ctx context.Context, cluster *clusterv1.Cluster, dockerMachine *infrav1.DevMachine, externalMachine *docker.Machine) (ctrl.Result, error) {
	cloudProviderTaint := corev1.Taint{Key: "node.cloudprovider.kubernetes.io/uninitialized", Effect: corev1.TaintEffectNoSchedule}

	nodeName := externalMachine.ContainerName()
	log := ctrl.LoggerFrom(ctx).WithValues("Node", klog.KRef("", nodeName))

	// If container is not yet provisioned, wait.
	if !conditions.IsTrue(dockerMachine, infrav1.DevMachineDockerContainerProvisionedCondition) {
		return ctrl.Result{}, nil
	}

	// If the Cluster is using a control plane and the control plane is not yet initialized, there is no API server
	// to contact to get the ProviderID for the Node hosted on this machine, so return early.
	// NOTE: We are using RequeueAfter with a short interval in order to make test execution time more stable.
	// NOTE: If the Cluster doesn't use a control plane, the ControlPlaneInitialized condition is only
	// set to true after a control plane machine has a node ref. If we would requeue here in this case, the
	// Machine will never get a node ref as ProviderID is required to set the node ref, so we would get a deadlock.
	if cluster.Spec.ControlPlaneRef.IsDefined() &&
		!conditions.IsTrue(cluster, clusterv1.ClusterControlPlaneInitializedCondition) {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Usually a cloud provider will do this, but there is no docker-cloud provider.
	// Requeue if there is an error, as this is likely momentary load balancer
	// state changes during control plane provisioning.
	remoteClient, err := r.ClusterCache.GetClient(ctx, client.ObjectKeyFromObject(cluster))
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrap(err, "failed to generate workload cluster client")
	}

	node := &corev1.Node{}
	if err = remoteClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return ctrl.Result{}, pkgerrors.Wrap(err, "failed to get node")
	}

	if node.Spec.ProviderID != "" && !taints.HasTaint(node.Spec.Taints, cloudProviderTaint) {
		return ctrl.Result{}, err
	}

	patchHelper, err := patch.NewHelper(node, remoteClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Set the providerID on the node.
	if node.Spec.ProviderID == "" {
		log.Info("Setting providerID", "providerID", externalMachine.ProviderID())
		node.Spec.ProviderID = externalMachine.ProviderID()
	}

	// If the node is managed by an external cloud provider - e.g. in dualstack tests - add the
	// machine addresses on the node and remove the cloudProviderTaint.
	if taints.HasTaint(node.Spec.Taints, cloudProviderTaint) {
		// The machine addresses must retain their order - i.e. new addresses should only be appended to the list.
		// This is what Kubelet expects when setting new IPs for pods using the host network.
		nodeAddressMap := map[corev1.NodeAddress]bool{}
		for _, addr := range node.Status.Addresses {
			nodeAddressMap[addr] = true
		}
		log.Info("Setting Kubernetes node IP Addresses")
		for _, addr := range dockerMachine.Status.Addresses {
			if _, ok := nodeAddressMap[corev1.NodeAddress{Address: addr.Address, Type: corev1.NodeAddressType(addr.Type)}]; ok {
				continue
			}
			// Set the addresses in the Node `.status.addresses`
			// Only add "InternalIP" type addresses.
			// Node "ExternalIP" addresses are not well-defined in Kubernetes across different cloud providers.
			// This keeps parity with what is done for dual-stack nodes in Kind.
			if addr.Type != clusterv1.MachineInternalIP {
				continue
			}
			node.Status.Addresses = append(node.Status.Addresses, corev1.NodeAddress{
				Type:    corev1.NodeAddressType(addr.Type),
				Address: addr.Address,
			})
		}
		// Remove the cloud provider taint on the node - if it exists - to initialize it.
		if taints.RemoveNodeTaint(node, cloudProviderTaint) {
			log.Info("Removing the cloudprovider taint")
		}
	}

	if err := patchHelper.Patch(ctx, node); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) getExternalObjects(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster, machine *clusterv1.Machine, dockerMachine *infrav1.DevMachine) (*docker.Machine, *docker.LoadBalancer, error) {
	// Create a helper for managing the Docker container hosting the machine.
	// The DockerMachine needs a way to know the name of the Docker container, before MachinePool Machines were implemented, it used the name of the owner Machine.
	// But since the DockerMachine type is used for both MachineDeployment Machines and MachinePool Machines, we need to accommodate both:
	// - For MachineDeployments/Control Planes, we continue using the name of the Machine so that it is backwards compatible in the CAPI version upgrade scenario .
	// - For MachinePools, the order of creation is Docker container -> DockerMachine -> Machine. Since the Docker container is created first, and the Machine name is
	//   randomly generated by the MP controller, we create the DockerMachine with the same name as the container to maintain this association.
	name := machine.Name
	if labels.IsMachinePoolOwned(dockerMachine) {
		name = dockerMachine.Name
	}

	externalMachine, err := docker.NewMachine(ctx, cluster, name, nil)
	if err != nil {
		return nil, nil, pkgerrors.Wrapf(err, "failed to create helper for managing the externalMachine")
	}

	// Create a helper for managing a docker container hosting the loadbalancer.
	// NB. the machine controller has to manage the cluster load balancer because the current implementation of the
	// docker load balancer does not support auto-discovery of control plane nodes, so CAPD should take care of
	// updating the cluster load balancer configuration when control plane machines are added/removed
	var imageRepository, imageTag string
	if dockerCluster.Spec.Backend.Docker != nil {
		imageRepository = dockerCluster.Spec.Backend.Docker.LoadBalancer.ImageRepository
		imageTag = dockerCluster.Spec.Backend.Docker.LoadBalancer.ImageTag
	}
	externalLoadBalancer, err := docker.NewLoadBalancer(ctx, cluster,
		imageRepository,
		imageTag,
		strconv.Itoa(int(dockerCluster.Spec.ControlPlaneEndpoint.Port)))
	if err != nil {
		return nil, nil, pkgerrors.Wrapf(err, "failed to create helper for managing the externalLoadBalancer")
	}
	return externalMachine, externalLoadBalancer, err
}

// ReconcileDelete handle docker backend for deleted DevMachines.
func (r *MachineBackendReconciler) ReconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster, machine *clusterv1.Machine, dockerMachine *infrav1.DevMachine) (ctrl.Result, error) {
	if dockerMachine.Spec.Backend.Docker == nil {
		return ctrl.Result{}, pkgerrors.New("DockerBackendReconciler can't be called for DevMachines without a Docker backend")
	}
	if dockerCluster.Spec.Backend.Docker == nil {
		return ctrl.Result{}, pkgerrors.New("DockerBackendReconciler can't be called for DevCluster without a Docker backend")
	}

	// Cancel all the provisioning tasks for this machine.
	r.TaskManager.Cancel(dockerMachine)

	externalMachine, externalLoadBalancer, err := r.getExternalObjects(ctx, cluster, dockerCluster, machine, dockerMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Set the ContainerProvisionedCondition reporting delete is started, and issue a patch in order to make
	// this visible to the users.
	// NB. The operation in docker is fast, so there is the chance the user will not notice the status change;
	// nevertheless we are issuing a patch so we can test a pattern that will be used by other providers as well
	if conditions.GetReason(dockerMachine, infrav1.DevMachineDockerContainerProvisionedCondition) != infrav1.DevMachineDockerContainerDeletingReason {
		v1beta1conditions.MarkFalse(dockerMachine, infrav1.ContainerProvisionedV1Beta1Condition, clusterv1.DeletingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
		conditions.Set(dockerCluster, metav1.Condition{
			Type:   infrav1.DevMachineDockerContainerProvisionedCondition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineDockerContainerDeletingReason,
		})
	}

	// delete the machine
	if err := externalMachine.Delete(ctx); err != nil {
		return ctrl.Result{}, pkgerrors.Wrap(err, "failed to delete DockerMachine")
	}

	// if the deleted machine is a control-plane node, remove it from the load balancer configuration;
	if util.IsControlPlaneMachine(machine) {
		if err := r.reconcileLoadBalancerConfiguration(ctx, cluster, dockerCluster, externalLoadBalancer); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Machine is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(dockerMachine, infrav1.MachineFinalizer)
	return ctrl.Result{}, nil
}

// PatchDevMachine patch a DevMachine.
func (r *MachineBackendReconciler) PatchDevMachine(ctx context.Context, patchHelper *patch.Helper, dockerMachine *infrav1.DevMachine, _ bool) error {
	if dockerMachine.Spec.Backend.Docker == nil {
		return pkgerrors.New("DockerBackendReconciler can't be called for DevMachines without a Docker backend")
	}

	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding the step counter during the deletion process).
	v1beta1conditions.SetSummary(dockerMachine,
		v1beta1conditions.WithConditions(
			infrav1.ContainerProvisionedV1Beta1Condition,
			infrav1.BootstrapExecSucceededV1Beta1Condition,
		),
		v1beta1conditions.WithStepCounterIf(dockerMachine.DeletionTimestamp.IsZero() && dockerMachine.Spec.ProviderID == ""),
	)
	if err := conditions.SetSummaryCondition(dockerMachine, dockerMachine, infrav1.DevMachineReadyCondition,
		conditions.ForConditionTypes{
			infrav1.DevMachineDockerCGroupsReadyCondition,
			infrav1.DevMachineDockerContainerProvisionedCondition,
			infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			// Note: on real infrastructure providers usually it is not possible to have visibility in the bootstrap process
			// but for docker machine it is, and so we surface this info to help in triaging issue.
			infrav1.DevMachineBootstrapCompletedCondition,
		},
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				// Use custom reasons.
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					infrav1.DevMachineNotReadyReason,
					infrav1.DevMachineReadyUnknownReason,
					infrav1.DevMachineReadyReason,
				)),
			),
		},
	); err != nil {
		return pkgerrors.Wrapf(err, "failed to set %s condition", infrav1.DevMachineReadyCondition)
	}

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		dockerMachine,
		patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyV1Beta1Condition,
			infrav1.ContainerProvisionedV1Beta1Condition,
			infrav1.BootstrapExecSucceededV1Beta1Condition,
		}},
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			infrav1.DevMachineReadyCondition,
			infrav1.DevMachineDockerCGroupsReadyCondition,
			infrav1.DevMachineDockerContainerProvisionedCondition,
			infrav1.DevMachineDockerPreLoadedImagesReadyCondition,
			infrav1.DevMachineBootstrapCompletedCondition,
		}},
	)
}

func (r *MachineBackendReconciler) reconcileLoadBalancerConfiguration(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster, externalLoadBalancer *docker.LoadBalancer) error {
	controlPlaneWeight := map[string]int{}

	controlPlaneMachineList := &clusterv1.MachineList{}
	if err := r.List(ctx, controlPlaneMachineList, client.InNamespace(cluster.Namespace), client.MatchingLabels{
		clusterv1.MachineControlPlaneLabel: "",
		clusterv1.ClusterNameLabel:         cluster.Name,
	}); err != nil {
		return pkgerrors.Wrap(err, "failed to list control plane machines")
	}

	for _, m := range controlPlaneMachineList.Items {
		containerName := docker.MachineContainerName(cluster.Name, m.Name)
		controlPlaneWeight[containerName] = 100
		if !m.DeletionTimestamp.IsZero() && len(controlPlaneMachineList.Items) > 1 {
			controlPlaneWeight[containerName] = 0
		}
	}

	unsafeLoadBalancerConfigTemplate, err := r.getUnsafeLoadBalancerConfigTemplate(ctx, dockerCluster)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to retrieve HAProxy configuration from CustomHAProxyConfigTemplateRef")
	}
	if err := externalLoadBalancer.UpdateConfiguration(ctx, controlPlaneWeight, unsafeLoadBalancerConfigTemplate); err != nil {
		return pkgerrors.Wrap(err, "failed to update DockerCluster.loadbalancer configuration")
	}
	return nil
}

func (r *MachineBackendReconciler) getBootstrapData(ctx context.Context, namespace string, dataSecretName string) (string, bootstrapv1.Format, error) {
	s := &corev1.Secret{}
	key := client.ObjectKey{Namespace: namespace, Name: dataSecretName}
	if err := r.Get(ctx, key, s); err != nil {
		return "", "", pkgerrors.Wrapf(err, "failed to retrieve bootstrap data secret %s", dataSecretName)
	}

	value, ok := s.Data["value"]
	if !ok {
		return "", "", pkgerrors.New("error retrieving bootstrap data: secret value key is missing")
	}

	format := s.Data["format"]
	if len(format) == 0 {
		format = []byte(bootstrapv1.CloudConfig)
	}

	return base64.StdEncoding.EncodeToString(value), bootstrapv1.Format(format), nil
}

func (r *MachineBackendReconciler) getUnsafeLoadBalancerConfigTemplate(ctx context.Context, dockerCluster *infrav1.DevCluster) (string, error) {
	if dockerCluster.Spec.Backend.Docker.LoadBalancer.CustomHAProxyConfigTemplateRef == nil {
		return "", nil
	}
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      dockerCluster.Spec.Backend.Docker.LoadBalancer.CustomHAProxyConfigTemplateRef.Name,
		Namespace: dockerCluster.Namespace,
	}
	if err := r.Get(ctx, key, cm); err != nil {
		return "", pkgerrors.Wrapf(err, "failed to retrieve custom HAProxy configuration ConfigMap %s", key)
	}
	template, ok := cm.Data["value"]
	if !ok {
		return "", fmt.Errorf("expected key \"value\" to exist in ConfigMap %s", key)
	}
	return template, nil
}

// setMachineAddress gets the address from the container corresponding to a docker node and sets it on the Machine object.
func setMachineAddress(ctx context.Context, dockerMachine *infrav1.DevMachine, externalMachine *docker.Machine) error {
	machineAddresses, err := externalMachine.Address(ctx)
	if err != nil {
		return err
	}
	dockerMachine.Status.Addresses = []clusterv1.MachineAddress{{
		Type:    clusterv1.MachineHostName,
		Address: externalMachine.ContainerName()},
	}

	for _, addr := range machineAddresses {
		dockerMachine.Status.Addresses = append(dockerMachine.Status.Addresses,
			clusterv1.MachineAddress{
				Type:    clusterv1.MachineInternalIP,
				Address: addr,
			},
			clusterv1.MachineAddress{
				Type:    clusterv1.MachineExternalIP,
				Address: addr,
			})
	}

	return nil
}

type cmdError struct {
	Err    error
	Stderr string
	Stdout string
}

func (e *cmdError) Error() string {
	return e.Err.Error()
}

func indentIfMultiline(m string, maxLines int) string {
	msg := ""
	m = strings.TrimSuffix(m, "\n")
	if strings.Contains(m, "\n") {
		msg += "\n"
		lines := strings.Split(fmt.Sprintf("...\n%s", m), "\n")
		start := len(lines) - maxLines
		if start < 0 {
			start = 0
		}
		lines = lines[start:]
		prefix := "  * "
		for i, l := range lines {
			lines[i] = prefix + l
		}
		msg += strings.Join(lines, "\n")
	} else {
		msg += m
	}
	return msg
}
