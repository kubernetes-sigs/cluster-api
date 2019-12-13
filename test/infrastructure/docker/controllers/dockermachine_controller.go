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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/kind/pkg/cluster/constants"
)

const (
	machineControllerName = "DockerMachine-controller"
)

// DockerMachineReconciler reconciles a DockerMachine object
type DockerMachineReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

// Reconcile handles DockerMachine events
func (r *DockerMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, rerr error) {
	ctx := context.Background()
	log := r.Log.WithName(machineControllerName).WithValues("docker-machine", req.NamespacedName)

	// Fetch the DockerMachine instance.
	dockerMachine := &infrav1.DockerMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
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

	log = log.WithValues("machine", machine.Name)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("DockerMachine owner Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine with a cluster using the label %s: <name of cluster>", clusterv1.ClusterLabelName))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	// Make sure infrastructure is ready
	if !cluster.Status.InfrastructureReady {
		log.Info("Waiting for DockerCluster Controller to create cluster infrastructure")
		return ctrl.Result{}, nil
	}

	// Fetch the Docker Cluster.
	dockerCluster := &infrav1.DockerCluster{}
	dockerClusterName := types.NamespacedName{
		Namespace: dockerMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, dockerClusterName, dockerCluster); err != nil {
		log.Info("DockerCluster is not available yet")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("docker-cluster", dockerCluster.Name)

	// Create a helper for managing the docker container hosting the machine.
	externalMachine, err := docker.NewMachine(cluster.Name, machine.Name, dockerMachine.Spec.CustomImage, log)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create helper for managing the externalMachine")
	}

	// Create a helper for managing a docker container hosting the loadbalancer.
	// NB. the machine controller has to manage the cluster load balancer because the current implementation of the
	// docker load balancer does not support auto-discovery of control plane nodes, so CAPD should take care of
	// updating the cluster load balancer configuration when control plane machines are added/removed
	externalLoadBalancer, err := docker.NewLoadBalancer(cluster.Name, log)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create helper for managing the externalLoadBalancer")
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerMachine, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Always attempt to Patch the DockerMachine object and status after each reconciliation.
	defer func() {
		if err := patchHelper.Patch(ctx, dockerMachine); err != nil {
			log.Error(err, "failed to patch DockerMachine")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Handle deleted machines
	if !dockerMachine.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(machine, dockerMachine, externalMachine, externalLoadBalancer)
	}

	// Handle non-deleted machines
	return r.reconcileNormal(ctx, machine, dockerMachine, externalMachine, externalLoadBalancer, log)
}

func (r *DockerMachineReconciler) reconcileNormal(ctx context.Context, machine *clusterv1.Machine, dockerMachine *infrav1.DockerMachine, externalMachine *docker.Machine, externalLoadBalancer *docker.LoadBalancer, log logr.Logger) (ctrl.Result, error) {
	// If the DockerMachine doesn't have finalizer, add it.
	if !util.Contains(dockerMachine.Finalizers, infrav1.MachineFinalizer) {
		dockerMachine.Finalizers = append(dockerMachine.Finalizers, infrav1.MachineFinalizer)
	}

	// if the machine is already provisioned, return
	if dockerMachine.Spec.ProviderID != nil {
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data is available and populated.
	if machine.Spec.Bootstrap.DataSecretName == nil {
		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		return ctrl.Result{}, nil
	}

	//Create the docker container hosting the machine
	role := constants.WorkerNodeRoleValue
	if util.IsControlPlaneMachine(machine) {
		role = constants.ControlPlaneNodeRoleValue
	}

	if err := externalMachine.Create(role, machine.Spec.Version); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to create worker DockerMachine")
	}

	// if the machine is a control plane added, update the load balancer configuration
	if util.IsControlPlaneMachine(machine) {
		if err := externalLoadBalancer.UpdateConfiguration(); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update DockerCluster.loadbalancer configuration")
		}
	}

	bootstrapData, err := r.getBootstrapData(ctx, machine)
	if err != nil {
		r.Log.Error(err, "failed to get bootstrap data")
		return ctrl.Result{}, nil
	}
	// exec bootstrap
	// NB. this step is necessary to mimic the behaviour of cloud-init that is embedded in the base images
	// for other cloud providers
	if err := externalMachine.ExecBootstrap(bootstrapData); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to exec DockerMachine bootstrap")
	}

	// Set the provider ID on the Kubernetes node corresponding to the external machine
	// NB. this step is necessary because there is no a cloud controller for docker that executes this step
	if err := externalMachine.SetNodeProviderID(); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to patch the Kubernetes node with the machine providerID")
	}

	// Set ProviderID so the Cluster API Machine Controller can pull it
	providerID := externalMachine.ProviderID()
	dockerMachine.Spec.ProviderID = &providerID

	// Mark the dockerMachine ready
	dockerMachine.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *DockerMachineReconciler) reconcileDelete(machine *clusterv1.Machine, dockerMachine *infrav1.DockerMachine, externalMachine *docker.Machine, externalLoadBalancer *docker.LoadBalancer) (ctrl.Result, error) {
	// if the deleted machine is a control-plane node, exec kubeadm reset so the etcd member hosted
	// on the machine gets removed in a controlled way
	if util.IsControlPlaneMachine(machine) {
		if err := externalMachine.KubeadmReset(); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to execute kubeadm reset")
		}
	}

	// delete the machine
	if err := externalMachine.Delete(); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to delete DockerMachine")
	}

	// if the deleted machine is a control-plane node, remove it from the load balancer configuration;
	if util.IsControlPlaneMachine(machine) {
		if err := externalLoadBalancer.UpdateConfiguration(); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update DockerCluster.loadbalancer configuration")
		}
	}

	// Machine is deleted so remove the finalizer.
	dockerMachine.Finalizers = util.Filter(dockerMachine.Finalizers, infrav1.MachineFinalizer)

	return ctrl.Result{}, nil
}

// SetupWithManager will add watches for this controller
func (r *DockerMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.DockerMachine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("DockerMachine")),
			},
		).
		Watches(
			&source.Kind{Type: &infrav1.DockerCluster{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: handler.ToRequestsFunc(r.DockerClusterToDockerMachines),
			},
		).
		Complete(r)
}

// DockerClusterToDockerMachines is a handler.ToRequestsFunc to be used to enqeue
// requests for reconciliation of DockerMachines.
func (r *DockerMachineReconciler) DockerClusterToDockerMachines(o handler.MapObject) []ctrl.Request {
	result := []ctrl.Request{}
	c, ok := o.Object.(*infrav1.DockerCluster)
	if !ok {
		r.Log.Error(errors.Errorf("expected a DockerCluster but got a %T", o.Object), "failed to get DockerMachine for DockerCluster")
		return nil
	}
	log := r.Log.WithValues("DockerCluster", c.Name, "Namespace", c.Namespace)

	cluster, err := util.GetOwnerCluster(context.TODO(), r.Client, c.ObjectMeta)
	switch {
	case apierrors.IsNotFound(err) || cluster == nil:
		return result
	case err != nil:
		log.Error(err, "failed to get owning cluster")
		return result
	}

	labels := map[string]string{clusterv1.ClusterLabelName: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(context.TODO(), machineList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
		log.Error(err, "failed to list DockerMachines")
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

func (r *DockerMachineReconciler) getBootstrapData(ctx context.Context, machine *clusterv1.Machine) (string, error) {
	if machine.Spec.Bootstrap.DataSecretName == nil {
		return "", errors.New("error retrieving bootstrap data: linked Machine's bootstrap.dataSecretName is nil")
	}

	s := &corev1.Secret{}
	key := types.NamespacedName{Namespace: machine.GetNamespace(), Name: *machine.Spec.Bootstrap.DataSecretName}
	if err := r.Client.Get(ctx, key, s); err != nil {
		return "", errors.Wrapf(err, "failed to retrieve bootstrap data secret for DockerMachine %s/%s", machine.GetNamespace(), machine.GetName())
	}

	value, ok := s.Data["value"]
	if !ok {
		return "", errors.New("error retrieving bootstrap data: secret value key is missing")
	}

	return base64.StdEncoding.EncodeToString(value), nil
}
