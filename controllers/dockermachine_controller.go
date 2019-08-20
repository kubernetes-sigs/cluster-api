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

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	infrastructurev1alpha2 "sigs.k8s.io/cluster-api-provider-docker/api/v1alpha2"
	"sigs.k8s.io/cluster-api-provider-docker/docker"
	"sigs.k8s.io/cluster-api-provider-docker/docker/actions"
	capiv1alpha2 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	kindcluster "sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/constants"
)

// DockerMachineReconciler reconciles a DockerMachine object
type DockerMachineReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch

// Reconcile handles DockerMachine events
func (r *DockerMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	log := r.Log.WithValues("dockermachine", req.NamespacedName)

	dockerMachine := &infrastructurev1alpha2.DockerMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get dockerMachine")
		return ctrl.Result{}, err
	}

	// Get the Cluster API Machine
	machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef on DockerMachine")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Get the Cluster API Cluster
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get cluster for docker machine")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine with a cluster using the label %s: <name of cluster>", capiv1alpha2.MachineClusterLabelName))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerMachine, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Always attempt to Patch the DockerMachine object and status after each reconciliation.
	defer func() {
		if err := patchHelper.Patch(ctx, dockerMachine); err != nil {
			log.Error(err, "failed to patch DockerCluster")
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// If the DockerMachine doesn't have finalizer, add it.
	if !util.Contains(dockerMachine.Finalizers, capiv1alpha2.MachineFinalizer) {
		dockerMachine.Finalizers = append(dockerMachine.Finalizers, infrastructurev1alpha2.MachineFinalizer)
	}
	state := getState(machine, dockerMachine)
	log = log.WithValues("state", state.String())
	// TODO: consider setting the key pieces in this function, such as ProviderID and deleting finalizers
	switch state {
	case Deleted:
		log.Info("Start reconcileDelete dockerMachine")
		return r.reconcileDelete(ctx, cluster, machine, dockerMachine)
	case Provisioned:
		return ctrl.Result{}, nil
	case Pending:
		log.Info("Waiting for machine bootstrap")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	case Provisioning:
		log.Info("Provisioning machine")
		// Ensuring cluster is ready for joining
		clusterExists, err := kindcluster.IsKnown(cluster.Name)
		if err != nil {
			log.Error(err, "Error finding cluster-name", "cluster", cluster.Name)
			return ctrl.Result{}, err
		}

		// If there's no cluster, requeue the request until there is one
		if !clusterExists {
			r.Log.Info("There is no cluster yet, waiting for a cluster before creating machines")
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}
		log.Info("Creating machine")
		return r.create(ctx, cluster, machine, dockerMachine)
	default:
		log.Info("Unknown state", "state", state)
		return ctrl.Result{}, nil
	}
}

// State is the state our machine object is in
type State int

const (
	// Provisioned is the state that happens after the machine is part of a cluster.
	// Technically it should be Running but this controller sees no difference between the two.
	Provisioned State = iota
	// Pending is when the machine is waiting for bootstrap data to exist.
	Pending
	// Deleted is when the machine has been deleted.
	Deleted
	// Provisioning is when bootstrap data exists but it's not finished being provisioned.
	Provisioning
)

// String is a helper function to print a human-readable form of the state.
func (s State) String() string {
	switch s {
	case Provisioning:
		return "Provisioning"
	case Deleted:
		return "Deleted"
	case Provisioned:
		return "Provisioned"
	case Pending:
		return "Pending"
	default:
		return "Unknown"
	}
}

func getState(machine *capiv1alpha2.Machine, dockerMachine *infrastructurev1alpha2.DockerMachine) State {
	// Deleted takes precedence
	if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
		return Deleted
	}
	if dockerMachine.Spec.ProviderID != nil {
		return Provisioned
	}
	if machine.Spec.Bootstrap.Data == nil {
		return Pending
	}
	return Provisioning
}

func (r *DockerMachineReconciler) create(
	ctx context.Context,
	c *capiv1alpha2.Cluster,
	machine *capiv1alpha2.Machine,
	dockerMachine *infrastructurev1alpha2.DockerMachine) (ctrl.Result, error) {

	log := r.Log.WithName("machine-create").WithValues("machine", machine.Name)

	role := constants.WorkerNodeRoleValue
	if util.IsControlPlaneMachine(machine) {
		role = constants.ControlPlaneNodeRoleValue
	}
	node, err := docker.NewNode(c.Name, machine.Name, role, *machine.Spec.Version, log)
	if err != nil {
		// TODO: This log line is confusing.
		log.Error(err, "Failed to initialize a node")
		return ctrl.Result{}, err
	}
	// Data must be populated if we've made it this far
	cloudConfig, err := base64.StdEncoding.DecodeString(*machine.Spec.Bootstrap.Data)
	if err != nil {
		log.Error(err, "Failed to decode machine's bootstrap data")
	}

	newNode, err := node.Create(cloudConfig)
	if err != nil {
		log.Error(err, "Failed to create node", "stacktrace", fmt.Sprintf("%+v", err))
		return ctrl.Result{}, err
	}
	log.Info("Setting the providerID", "provider-id", newNode.Name())
	// set the machine's providerID
	providerID := actions.ProviderID(newNode.Name())
	dockerMachine.Spec.ProviderID = &providerID
	dockerMachine.Status.Ready = true
	return ctrl.Result{}, nil
}

// SetupWithManager will add watches for this controller
func (r *DockerMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha2.DockerMachine{}).
		Watches(
			&source.Kind{Type: &capiv1alpha2.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.MachineToInfrastructureMapFunc(infrastructurev1alpha2.GroupVersion.WithKind("DockerMachine")),
			},
		).
		Complete(r)
}

func (r *DockerMachineReconciler) reconcileDelete(
	ctx context.Context,
	cluster *capiv1alpha2.Cluster,
	machine *capiv1alpha2.Machine,
	dockerMachine *infrastructurev1alpha2.DockerMachine,
) (ctrl.Result, error) {
	log := r.Log.WithValues("cluster", cluster.Name, "machine", machine.Name)

	role := constants.WorkerNodeRoleValue
	if util.IsControlPlaneMachine(machine) {
		role = constants.ControlPlaneNodeRoleValue
	}
	node, err := docker.NewNode(cluster.Name, machine.Name, role, *machine.Spec.Version, log)
	if err != nil {
		// TODO: This log line is confusing.
		log.Error(err, "Failed to initialize a node")
		return ctrl.Result{}, err
	}
	if err := node.Delete(); err != nil {
		log.Error(err, "Error deleting a node")
		return ctrl.Result{}, err
	}
	// Remove the finalizer
	dockerMachine.ObjectMeta.Finalizers = util.Filter(dockerMachine.ObjectMeta.Finalizers, infrastructurev1alpha2.MachineFinalizer)
	return ctrl.Result{}, nil
}
