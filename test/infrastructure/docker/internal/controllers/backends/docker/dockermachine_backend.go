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
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	utilexp "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/labels"
	"sigs.k8s.io/cluster-api/util/patch"
)

// MachineBackendReconciler reconciles a DockerMachine object.
type MachineBackendReconciler struct {
	client.Client
	ContainerRuntime container.Runtime
	ClusterCache     clustercache.ClusterCache

	NewPatchHelperFunc  func(obj client.Object, crClient client.Client) (*patch.Helper, error)
	PatchDevMachineFunc func(ctx context.Context, patchHelper *patch.Helper, dockerMachine *infrav1.DevMachine, _ bool) error
}

// ReconcileNormal handle docker backend for DevMachines not yet deleted.
func (r *MachineBackendReconciler) ReconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster, machine *clusterv1.Machine, dockerMachine *infrav1.DevMachine) (res ctrl.Result, retErr error) {
	if dockerMachine.Spec.Backend.Docker == nil {
		return ctrl.Result{}, errors.New("DockerBackendReconciler can't be called for DevMachines without a Docker backend")
	}
	if dockerCluster.Spec.Backend.Docker == nil {
		return ctrl.Result{}, errors.New("DockerBackendReconciler can't be called for DevCluster without a Docker backend")
	}
	log := ctrl.LoggerFrom(ctx)

	externalMachine, externalLoadBalancer, err := r.getExternalObjects(ctx, cluster, dockerCluster, machine, dockerMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if the infrastructure is ready, otherwise return and wait for the cluster object to be updated
	if !cluster.Status.InfrastructureReady {
		log.Info("Waiting for DockerCluster Controller to create cluster infrastructure")
		conditions.MarkFalse(dockerMachine, infrav1.ContainerProvisionedCondition, infrav1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	var dataSecretName *string
	var version *string

	if labels.IsMachinePoolOwned(dockerMachine) {
		machinePool, err := utilexp.GetMachinePoolByLabels(ctx, r.Client, dockerMachine.GetNamespace(), dockerMachine.Labels)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get machine pool for DockerMachine %s/%s", dockerMachine.GetNamespace(), dockerMachine.GetName())
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

	// if the corresponding machine is deleted but the docker machine not yet, update load balancer configuration to divert all traffic from this instance
	if util.IsControlPlaneMachine(machine) && !machine.DeletionTimestamp.IsZero() && dockerMachine.DeletionTimestamp.IsZero() {
		if _, ok := dockerMachine.Annotations["dockermachine.infrastructure.cluster.x-k8s.io/weight"]; !ok {
			if err := r.reconcileLoadBalancerConfiguration(ctx, cluster, dockerCluster, externalLoadBalancer); err != nil {
				return ctrl.Result{}, err
			}
		}
		if dockerMachine.Annotations == nil {
			dockerMachine.Annotations = map[string]string{}
		}
		dockerMachine.Annotations["dockermachine.infrastructure.cluster.x-k8s.io/weight"] = "0"
	}

	// if the machine is already provisioned, return
	if dockerMachine.Spec.ProviderID != nil {
		// ensure ready state is set.
		// This is required after move, because status is not moved to the target cluster.
		dockerMachine.Status.Ready = true

		if externalMachine.Exists() {
			conditions.MarkTrue(dockerMachine, infrav1.ContainerProvisionedCondition)
			// Setting machine address is required after move, because status.Address field is not retained during move.
			if err := setMachineAddress(ctx, dockerMachine, externalMachine); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to set the machine address")
			}
		} else {
			conditions.MarkFalse(dockerMachine, infrav1.ContainerProvisionedCondition, infrav1.ContainerDeletedReason, clusterv1.ConditionSeverityError, fmt.Sprintf("Container %s does not exists anymore", externalMachine.Name()))
		}
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data is available and populated.
	if dataSecretName == nil {
		if !util.IsControlPlaneMachine(machine) && !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
			log.Info("Waiting for the control plane to be initialized")
			conditions.MarkFalse(dockerMachine, infrav1.ContainerProvisionedCondition, clusterv1.WaitingForControlPlaneAvailableReason, clusterv1.ConditionSeverityInfo, "")
			return ctrl.Result{}, nil
		}

		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		conditions.MarkFalse(dockerMachine, infrav1.ContainerProvisionedCondition, infrav1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// Create the docker container hosting the machine
	role := constants.WorkerNodeRoleValue
	if util.IsControlPlaneMachine(machine) {
		role = constants.ControlPlaneNodeRoleValue
	}

	// Create the machine if not existing yet
	if !externalMachine.Exists() {
		// NOTE: FailureDomains don't mean much in CAPD since it's all local, but we are setting a label on
		// each container, so we can check placement.
		if err := externalMachine.Create(ctx, dockerMachine.Spec.Backend.Docker.CustomImage, role, machine.Spec.Version, docker.FailureDomainLabel(machine.Spec.FailureDomain), dockerMachine.Spec.Backend.Docker.ExtraMounts); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to create worker DockerMachine")
		}
	}

	// Preload images into the container
	if len(dockerMachine.Spec.Backend.Docker.PreLoadImages) > 0 {
		if err := externalMachine.PreloadLoadImages(ctx, dockerMachine.Spec.Backend.Docker.PreLoadImages); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to pre-load images into the DockerMachine")
		}
	}

	// if the machine is a control plane update the load balancer configuration
	// we should only do this once, as reconfiguration more or less ensures
	// node ref setting fails
	if util.IsControlPlaneMachine(machine) && (dockerMachine.Status.Backend == nil || dockerMachine.Status.Backend.Docker == nil || !dockerMachine.Status.Backend.Docker.LoadBalancerConfigured) {
		if err := r.reconcileLoadBalancerConfiguration(ctx, cluster, dockerCluster, externalLoadBalancer); err != nil {
			return ctrl.Result{}, err
		}
		if dockerMachine.Status.Backend == nil {
			dockerMachine.Status.Backend = &infrav1.DevMachineBackendStatus{}
		}
		if dockerMachine.Status.Backend.Docker == nil {
			dockerMachine.Status.Backend.Docker = &infrav1.DockerMachineBackendStatus{}
		}
		dockerMachine.Status.Backend.Docker.LoadBalancerConfigured = true
	}

	// Update the ContainerProvisionedCondition condition
	// NOTE: it is required to create the patch helper before this change otherwise it wont surface if
	// we issue a patch down in the code (because if we create patch helper after this point the ContainerProvisionedCondition=True exists both on before and after).
	newPatchHelperFunc := r.NewPatchHelperFunc
	if newPatchHelperFunc == nil {
		newPatchHelperFunc = patch.NewHelper
	}
	patchHelper, err := newPatchHelperFunc(dockerMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	conditions.MarkTrue(dockerMachine, infrav1.ContainerProvisionedCondition)

	// At, this stage, we are ready for bootstrap. However, if the BootstrapExecSucceededCondition is missing we add it and we
	// issue an patch so the user can see the change of state before the bootstrap actually starts.
	// NOTE: usually controller should not rely on status they are setting, but on the observed state; however
	// in this case we are doing this because we explicitly want to give a feedback to users.
	if !conditions.Has(dockerMachine, infrav1.BootstrapExecSucceededCondition) {
		conditions.MarkFalse(dockerMachine, infrav1.BootstrapExecSucceededCondition, infrav1.BootstrappingReason, clusterv1.ConditionSeverityInfo, "")
		patchDevMachineFunc := r.PatchDevMachineFunc
		if patchDevMachineFunc == nil {
			patchDevMachineFunc = r.PatchDevMachine
		}
		if err := patchDevMachineFunc(ctx, patchHelper, dockerMachine, util.IsControlPlaneMachine(machine)); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to patch DockerMachine")
		}
	}

	// if the machine isn't bootstrapped, only then run bootstrap scripts
	if !dockerMachine.Spec.Backend.Docker.Bootstrapped {
		var bootstrapTimeout metav1.Duration
		if dockerMachine.Spec.Backend.Docker.BootstrapTimeout != nil {
			bootstrapTimeout = *dockerMachine.Spec.Backend.Docker.BootstrapTimeout
		} else {
			bootstrapTimeout = metav1.Duration{Duration: 3 * time.Minute}
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, bootstrapTimeout.Duration)
		defer cancel()

		// Check for bootstrap success
		// We have to check here to make this reentrant for cases where the bootstrap works
		// but bootstrapped is never set on the object. We only try to bootstrap if the machine
		// is not already bootstrapped.
		if err := externalMachine.CheckForBootstrapSuccess(timeoutCtx, false); err != nil {
			// We know the bootstrap data is not nil because we checked above.
			bootstrapData, format, err := r.getBootstrapData(timeoutCtx, dockerMachine.Namespace, *dataSecretName)
			if err != nil {
				return ctrl.Result{}, err
			}

			// Setup a go routing to check for the machine being deleted while running bootstrap as a
			// synchronous process, e.g. due to remediation. The routine stops when timeoutCtx is Done
			// (either because canceled intentionally due to machine deletion or canceled by the defer cancel()
			// call when exiting from this func).
			go func() {
				for {
					select {
					case <-timeoutCtx.Done():
						return
					default:
						updatedDockerMachine := &infrav1.DockerMachine{}
						if err := r.Client.Get(ctx, client.ObjectKeyFromObject(dockerMachine), updatedDockerMachine); err == nil &&
							!updatedDockerMachine.DeletionTimestamp.IsZero() {
							log.Info("Cancelling Bootstrap because the underlying machine has been deleted")
							cancel()
							return
						}
						time.Sleep(5 * time.Second)
					}
				}
			}()

			// Run the bootstrap script. Simulates cloud-init/Ignition.
			if err := externalMachine.ExecBootstrap(timeoutCtx, bootstrapData, format, version, dockerMachine.Spec.Backend.Docker.CustomImage); err != nil {
				conditions.MarkFalse(dockerMachine, infrav1.BootstrapExecSucceededCondition, infrav1.BootstrapFailedReason, clusterv1.ConditionSeverityWarning, "Repeating bootstrap")
				return ctrl.Result{}, errors.Wrap(err, "failed to exec DockerMachine bootstrap")
			}

			// Check for bootstrap success
			if err := externalMachine.CheckForBootstrapSuccess(timeoutCtx, true); err != nil {
				conditions.MarkFalse(dockerMachine, infrav1.BootstrapExecSucceededCondition, infrav1.BootstrapFailedReason, clusterv1.ConditionSeverityWarning, "Repeating bootstrap")
				return ctrl.Result{}, errors.Wrap(err, "failed to check for existence of bootstrap success file at /run/cluster-api/bootstrap-success.complete")
			}
		}
		dockerMachine.Spec.Backend.Docker.Bootstrapped = true
	}

	// Update the BootstrapExecSucceededCondition condition
	conditions.MarkTrue(dockerMachine, infrav1.BootstrapExecSucceededCondition)

	if err := setMachineAddress(ctx, dockerMachine, externalMachine); err != nil {
		log.Error(err, "Failed to set the machine address")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// If the Cluster is using a control plane and the control plane is not yet initialized, there is no API server
	// to contact to get the ProviderID for the Node hosted on this machine, so return early.
	// NOTE: We are using RequeueAfter with a short interval in order to make test execution time more stable.
	// NOTE: If the Cluster doesn't use a control plane, the ControlPlaneInitialized condition is only
	// set to true after a control plane machine has a node ref. If we would requeue here in this case, the
	// Machine will never get a node ref as ProviderID is required to set the node ref, so we would get a deadlock.
	if cluster.Spec.ControlPlaneRef != nil &&
		!conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Usually a cloud provider will do this, but there is no docker-cloud provider.
	// Requeue if there is an error, as this is likely momentary load balancer
	// state changes during control plane provisioning.
	remoteClient, err := r.ClusterCache.GetClient(ctx, client.ObjectKeyFromObject(cluster))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to generate workload cluster client")
	}
	if err := externalMachine.CloudProviderNodePatch(ctx, remoteClient, dockerMachine.Status.Addresses); err != nil {
		if errors.As(err, &docker.ContainerNotRunningError{}) {
			return ctrl.Result{}, errors.Wrap(err, "failed to patch the Kubernetes node with the machine providerID")
		}
		log.Error(err, "Failed to patch the Kubernetes node with the machine providerID")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	// Set ProviderID so the Cluster API Machine Controller can pull it
	providerID := externalMachine.ProviderID()
	dockerMachine.Spec.ProviderID = &providerID
	dockerMachine.Status.Ready = true
	conditions.MarkTrue(dockerMachine, infrav1.ContainerProvisionedCondition)

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
		return nil, nil, errors.Wrapf(err, "failed to create helper for managing the externalMachine")
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
		strconv.Itoa(dockerCluster.Spec.ControlPlaneEndpoint.Port))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create helper for managing the externalLoadBalancer")
	}
	return externalMachine, externalLoadBalancer, err
}

// ReconcileDelete handle docker backend for deleted DevMachines.
func (r *MachineBackendReconciler) ReconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster, machine *clusterv1.Machine, dockerMachine *infrav1.DevMachine) (ctrl.Result, error) {
	if dockerMachine.Spec.Backend.Docker == nil {
		return ctrl.Result{}, errors.New("DockerBackendReconciler can't be called for DevMachines without a Docker backend")
	}
	if dockerCluster.Spec.Backend.Docker == nil {
		return ctrl.Result{}, errors.New("DockerBackendReconciler can't be called for DevCluster without a Docker backend")
	}

	externalMachine, externalLoadBalancer, err := r.getExternalObjects(ctx, cluster, dockerCluster, machine, dockerMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Set the ContainerProvisionedCondition reporting delete is started, and issue a patch in order to make
	// this visible to the users.
	// NB. The operation in docker is fast, so there is the chance the user will not notice the status change;
	// nevertheless we are issuing a patch so we can test a pattern that will be used by other providers as well
	newPatchHelperFunc := r.NewPatchHelperFunc
	if newPatchHelperFunc == nil {
		newPatchHelperFunc = patch.NewHelper
	}
	patchHelper, err := newPatchHelperFunc(dockerMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	conditions.MarkFalse(dockerMachine, infrav1.ContainerProvisionedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")

	patchDevMachineFunc := r.PatchDevMachineFunc
	if patchDevMachineFunc == nil {
		patchDevMachineFunc = r.PatchDevMachine
	}
	if err := patchDevMachineFunc(ctx, patchHelper, dockerMachine, util.IsControlPlaneMachine(machine)); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to patch DockerMachine")
	}

	// delete the machine
	if err := externalMachine.Delete(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to delete DockerMachine")
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
		return errors.New("DockerBackendReconciler can't be called for DevMachines without a Docker backend")
	}

	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding the step counter during the deletion process).
	conditions.SetSummary(dockerMachine,
		conditions.WithConditions(
			infrav1.ContainerProvisionedCondition,
			infrav1.BootstrapExecSucceededCondition,
		),
		conditions.WithStepCounterIf(dockerMachine.ObjectMeta.DeletionTimestamp.IsZero() && dockerMachine.Spec.ProviderID == nil),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		dockerMachine,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.ContainerProvisionedCondition,
			infrav1.BootstrapExecSucceededCondition,
		}},
	)
}

func (r *MachineBackendReconciler) reconcileLoadBalancerConfiguration(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DevCluster, externalLoadBalancer *docker.LoadBalancer) error {
	controlPlaneWeight := map[string]int{}

	controlPlaneMachineList := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, controlPlaneMachineList, client.InNamespace(cluster.Namespace), client.MatchingLabels{
		clusterv1.MachineControlPlaneLabel: "",
		clusterv1.ClusterNameLabel:         cluster.Name,
	}); err != nil {
		return errors.Wrap(err, "failed to list control plane machines")
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
		return errors.Wrap(err, "failed to retrieve HAProxy configuration from CustomHAProxyConfigTemplateRef")
	}
	if err := externalLoadBalancer.UpdateConfiguration(ctx, controlPlaneWeight, unsafeLoadBalancerConfigTemplate); err != nil {
		return errors.Wrap(err, "failed to update DockerCluster.loadbalancer configuration")
	}
	return nil
}

func (r *MachineBackendReconciler) getBootstrapData(ctx context.Context, namespace string, dataSecretName string) (string, bootstrapv1.Format, error) {
	s := &corev1.Secret{}
	key := client.ObjectKey{Namespace: namespace, Name: dataSecretName}
	if err := r.Client.Get(ctx, key, s); err != nil {
		return "", "", errors.Wrapf(err, "failed to retrieve bootstrap data secret %s", dataSecretName)
	}

	value, ok := s.Data["value"]
	if !ok {
		return "", "", errors.New("error retrieving bootstrap data: secret value key is missing")
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
		return "", errors.Wrapf(err, "failed to retrieve custom HAProxy configuration ConfigMap %s", key)
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
