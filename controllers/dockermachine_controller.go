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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	infrastructurev1alpha2 "sigs.k8s.io/cluster-api-provider-docker/api/v1alpha2"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	capiv1alpha2 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	kindcluster "sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
)

const (
	// label "set:controlplane" indicates a control plane node
	clusterAPIControlPlaneSetLabel = "controlplane"
)

// DockerMachineReconciler reconciles a DockerMachine object
type DockerMachineReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=create

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

	// Store Docker Machine early state to allow patching.
	patch := client.MergeFrom(dockerMachine.DeepCopy())
	defer func() {
		if err := r.patchMachine(ctx, dockerMachine, patch); err != nil {
			r.Log.Error(err, "Error Patching DockerMachine", "name", dockerMachine.GetName())
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// If the DockerMachine doesn't have finalizer, add it.
	if !util.Contains(dockerMachine.Finalizers, capiv1alpha2.MachineFinalizer) {
		dockerMachine.Finalizers = append(dockerMachine.Finalizers, infrastructurev1alpha2.MachineFinalizer)
	}

	// If this is the delete case, delete the container and remove the finalizer
	if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("Start reconcileDelete dockerMachine")
		return r.reconcileDelete(ctx, cluster, machine, dockerMachine)
	}

	// If the machine already has a ProviderID then there is no action to take
	if machine.Spec.ProviderID != nil {
		return ctrl.Result{}, nil
	}

	// If there is no provider id and no bootstrap data we are waiting for the bootstrap data
	if machine.Spec.Bootstrap.Data == nil {
		log.Info("Waiting for machine bootstrap")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

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

	// At this point the machine is not yet provisioned. Create if possible.
	if machine.Spec.Bootstrap.Data != nil {
		return r.create(ctx, cluster, machine, dockerMachine)
	}

	// We are waiting for bootstrap data
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func getRole(machine *capiv1alpha2.Machine) string {
	// Figure out what kind of node we're making
	labels := machine.GetLabels()
	setValue, ok := labels["set"]
	if !ok {
		setValue = constants.WorkerNodeRoleValue
	}
	return setValue
}

func (r *DockerMachineReconciler) create(
	ctx context.Context,
	c *capiv1alpha2.Cluster,
	machine *capiv1alpha2.Machine,
	dockerMachine *infrastructurev1alpha2.DockerMachine) (ctrl.Result, error) {

	log := r.Log.WithValues("cluster", c.Name)
	log.Info("Creating a machine for cluster")
	var node *nodes.Node
	var err error
	if isControlPlaneMachine(machine) {
		r.Log.Info("Adding a control plane node", "machine-name", machine.GetName(), "cluster-name", c.Name)
		node, err = actions.AddControlPlane(c.Name, machine.GetName(), *machine.Spec.Version)
		if err != nil {
			r.Log.Error(err, "Error adding control plane")
			return ctrl.Result{}, err
		}
	} else {
		r.Log.Info("Creating a new worker node")
		node, err = actions.AddWorker(c.Name, machine.GetName(), *machine.Spec.Version)
		if err != nil {
			r.Log.Error(err, "Error creating new worker node")
			return ctrl.Result{}, err
		}
	}
	// set the machine's providerID
	providerID := actions.ProviderID(node.Name())
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
	if isControlPlaneMachine(machine) {
		err := actions.DeleteControlPlane(cluster.Name, machine.Name)
		if err != nil {
			log.Error(err, "Error deleting control-plane machine")
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
	} else {
		err := actions.DeleteWorker(cluster.Name, machine.Name)
		if err != nil {
			log.Error(err, "Error deleting worker node")
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}
	}

	dockerMachine.ObjectMeta.Finalizers = util.Filter(dockerMachine.ObjectMeta.Finalizers, infrastructurev1alpha2.MachineFinalizer)

	return ctrl.Result{}, nil
}

func isControlPlaneMachine(machine *capiv1alpha2.Machine) bool {
	setValue := getRole(machine)
	if setValue == clusterAPIControlPlaneSetLabel {
		return true
	}
	return false
}

func (r *DockerMachineReconciler) patchMachine(ctx context.Context,
	dockerMachine *infrastructurev1alpha2.DockerMachine, patchConfig client.Patch) error {
	if err := r.Status().Patch(ctx, dockerMachine, patchConfig); err != nil {
		return err
	}
	if err := r.Patch(ctx, dockerMachine, patchConfig); err != nil {
		return err
	}
	return nil
}
