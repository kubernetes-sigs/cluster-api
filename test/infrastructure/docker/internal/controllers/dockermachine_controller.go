/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	utilexp "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/labels"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// DockerMachineReconciler reconciles a DockerMachine object.
type DockerMachineReconciler struct {
	client.Client
	ContainerRuntime container.Runtime
	Tracker          *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines/status;dockermachines/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machinesets;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile handles DockerMachine events.
func (r *DockerMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)
	ctx = container.RuntimeInto(ctx, r.ContainerRuntime)

	// Fetch the DockerMachine instance.
	dockerMachine := &infrav1.DockerMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// AddOwners adds the owners of DockerMachine as k/v pairs to the logger.
	// Specifically, it will add KubeadmControlPlane, MachineSet and MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, dockerMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef on DockerMachine")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Machine", klog.KObj(machine))
	ctx = ctrl.LoggerInto(ctx, log)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("DockerMachine owner Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, dockerMachine) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	if cluster.Spec.InfrastructureRef == nil {
		log.Info("Cluster infrastructureRef is not available yet")
		return ctrl.Result{}, nil
	}

	// Fetch the Docker Cluster.
	dockerCluster := &infrav1.DockerCluster{}
	dockerClusterName := client.ObjectKey{
		Namespace: dockerMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, dockerClusterName, dockerCluster); err != nil {
		log.Info("DockerCluster is not available yet")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerMachine, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Always attempt to Patch the DockerMachine object and status after each reconciliation.
	defer func() {
		if err := patchDockerMachine(ctx, patchHelper, dockerMachine); err != nil {
			log.Error(err, "Failed to patch DockerMachine")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if dockerMachine.ObjectMeta.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(dockerMachine, infrav1.MachineFinalizer) {
		controllerutil.AddFinalizer(dockerMachine, infrav1.MachineFinalizer)
		return ctrl.Result{}, nil
	}

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
		return ctrl.Result{}, errors.Wrapf(err, "failed to create helper for managing the externalMachine")
	}

	// Create a helper for managing a docker container hosting the loadbalancer.
	// NB. the machine controller has to manage the cluster load balancer because the current implementation of the
	// docker load balancer does not support auto-discovery of control plane nodes, so CAPD should take care of
	// updating the cluster load balancer configuration when control plane machines are added/removed
	externalLoadBalancer, err := docker.NewLoadBalancer(ctx, cluster, dockerCluster)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create helper for managing the externalLoadBalancer")
	}

	// Handle deleted machines
	if !dockerMachine.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, dockerCluster, machine, dockerMachine, externalMachine, externalLoadBalancer)
	}

	// Handle non-deleted machines
	res, err := r.reconcileNormal(ctx, cluster, dockerCluster, machine, dockerMachine, externalMachine, externalLoadBalancer)
	// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
	// the current cluster because of concurrent access.
	if errors.Is(err, remote.ErrClusterLocked) {
		log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return res, err
}

func patchDockerMachine(ctx context.Context, patchHelper *patch.Helper, dockerMachine *infrav1.DockerMachine) error {
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

func (r *DockerMachineReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, dockerCluster *infrav1.DockerCluster, machine *clusterv1.Machine, dockerMachine *infrav1.DockerMachine, externalMachine *docker.Machine, externalLoadBalancer *docker.LoadBalancer) (res ctrl.Result, retErr error) {
	log := ctrl.LoggerFrom(ctx)

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
		if err := externalMachine.Create(ctx, dockerMachine.Spec.CustomImage, role, machine.Spec.Version, docker.FailureDomainLabel(machine.Spec.FailureDomain), dockerMachine.Spec.ExtraMounts); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to create worker DockerMachine")
		}
	}

	// Preload images into the container
	if len(dockerMachine.Spec.PreLoadImages) > 0 {
		if err := externalMachine.PreloadLoadImages(ctx, dockerMachine.Spec.PreLoadImages); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to pre-load images into the DockerMachine")
		}
	}

	// if the machine is a control plane update the load balancer configuration
	// we should only do this once, as reconfiguration more or less ensures
	// node ref setting fails
	if util.IsControlPlaneMachine(machine) && !dockerMachine.Status.LoadBalancerConfigured {
		unsafeLoadBalancerConfigTemplate, err := r.getUnsafeLoadBalancerConfigTemplate(ctx, dockerCluster)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to retrieve HAProxy configuration from CustomHAProxyConfigTemplateRef")
		}
		if err := externalLoadBalancer.UpdateConfiguration(ctx, unsafeLoadBalancerConfigTemplate); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update DockerCluster.loadbalancer configuration")
		}
		dockerMachine.Status.LoadBalancerConfigured = true
	}

	// Update the ContainerProvisionedCondition condition
	// NOTE: it is required to create the patch helper before this change otherwise it wont surface if
	// we issue a patch down in the code (because if we create patch helper after this point the ContainerProvisionedCondition=True exists both on before and after).
	patchHelper, err := patch.NewHelper(dockerMachine, r.Client)
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
		if err := patchDockerMachine(ctx, patchHelper, dockerMachine); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to patch DockerMachine")
		}
	}

	// if the machine isn't bootstrapped, only then run bootstrap scripts
	if !dockerMachine.Spec.Bootstrapped {
		var bootstrapTimeout metav1.Duration
		if dockerMachine.Spec.BootstrapTimeout != nil {
			bootstrapTimeout = *dockerMachine.Spec.BootstrapTimeout
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
			if err := externalMachine.ExecBootstrap(timeoutCtx, bootstrapData, format, version, dockerMachine.Spec.CustomImage); err != nil {
				conditions.MarkFalse(dockerMachine, infrav1.BootstrapExecSucceededCondition, infrav1.BootstrapFailedReason, clusterv1.ConditionSeverityWarning, "Repeating bootstrap")
				return ctrl.Result{}, errors.Wrap(err, "failed to exec DockerMachine bootstrap")
			}

			// Check for bootstrap success
			if err := externalMachine.CheckForBootstrapSuccess(timeoutCtx, true); err != nil {
				conditions.MarkFalse(dockerMachine, infrav1.BootstrapExecSucceededCondition, infrav1.BootstrapFailedReason, clusterv1.ConditionSeverityWarning, "Repeating bootstrap")
				return ctrl.Result{}, errors.Wrap(err, "failed to check for existence of bootstrap success file at /run/cluster-api/bootstrap-success.complete")
			}
		}
		dockerMachine.Spec.Bootstrapped = true
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
	remoteClient, err := r.Tracker.GetClient(ctx, client.ObjectKeyFromObject(cluster))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to generate workload cluster client")
	}
	if err := externalMachine.CloudProviderNodePatch(ctx, remoteClient, dockerMachine); err != nil {
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

func (r *DockerMachineReconciler) reconcileDelete(ctx context.Context, dockerCluster *infrav1.DockerCluster, machine *clusterv1.Machine, dockerMachine *infrav1.DockerMachine, externalMachine *docker.Machine, externalLoadBalancer *docker.LoadBalancer) error {
	// Set the ContainerProvisionedCondition reporting delete is started, and issue a patch in order to make
	// this visible to the users.
	// NB. The operation in docker is fast, so there is the chance the user will not notice the status change;
	// nevertheless we are issuing a patch so we can test a pattern that will be used by other providers as well
	patchHelper, err := patch.NewHelper(dockerMachine, r.Client)
	if err != nil {
		return err
	}
	conditions.MarkFalse(dockerMachine, infrav1.ContainerProvisionedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	if err := patchDockerMachine(ctx, patchHelper, dockerMachine); err != nil {
		return errors.Wrap(err, "failed to patch DockerMachine")
	}

	// delete the machine
	if err := externalMachine.Delete(ctx); err != nil {
		return errors.Wrap(err, "failed to delete DockerMachine")
	}

	// if the deleted machine is a control-plane node, remove it from the load balancer configuration;
	if util.IsControlPlaneMachine(machine) {
		unsafeLoadBalancerConfigTemplate, err := r.getUnsafeLoadBalancerConfigTemplate(ctx, dockerCluster)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve HAProxy configuration from CustomHAProxyConfigTemplateRef")
		}
		if err := externalLoadBalancer.UpdateConfiguration(ctx, unsafeLoadBalancerConfigTemplate); err != nil {
			return errors.Wrap(err, "failed to update DockerCluster.loadbalancer configuration")
		}
	}

	// Machine is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(dockerMachine, infrav1.MachineFinalizer)
	return nil
}

// SetupWithManager will add watches for this controller.
func (r *DockerMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToDockerMachines, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrav1.DockerMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.DockerMachine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("DockerMachine"))),
		).
		Watches(
			&infrav1.DockerCluster{},
			handler.EnqueueRequestsFromMapFunc(r.DockerClusterToDockerMachines),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToDockerMachines),
			builder.WithPredicates(
				predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
			),
		).Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

// DockerClusterToDockerMachines is a handler.ToRequestsFunc to be used to enqueue
// requests for reconciliation of DockerMachines.
func (r *DockerMachineReconciler) DockerClusterToDockerMachines(ctx context.Context, o client.Object) []ctrl.Request {
	result := []ctrl.Request{}
	c, ok := o.(*infrav1.DockerCluster)
	if !ok {
		panic(fmt.Sprintf("Expected a DockerCluster but got a %T", o))
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, c.ObjectMeta)
	switch {
	case apierrors.IsNotFound(err) || cluster == nil:
		return result
	case err != nil:
		return result
	}

	labels := map[string]string{clusterv1.ClusterNameLabel: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, machineList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil
	}
	for _, m := range machineList.Items {
		if m.Spec.InfrastructureRef.Name == "" {
			continue
		}
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}

func (r *DockerMachineReconciler) getBootstrapData(ctx context.Context, namespace string, dataSecretName string) (string, bootstrapv1.Format, error) {
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

func (r *DockerMachineReconciler) getUnsafeLoadBalancerConfigTemplate(ctx context.Context, dockerCluster *infrav1.DockerCluster) (string, error) {
	if dockerCluster.Spec.LoadBalancer.CustomHAProxyConfigTemplateRef == nil {
		return "", nil
	}
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      dockerCluster.Spec.LoadBalancer.CustomHAProxyConfigTemplateRef.Name,
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
func setMachineAddress(ctx context.Context, dockerMachine *infrav1.DockerMachine, externalMachine *docker.Machine) error {
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
