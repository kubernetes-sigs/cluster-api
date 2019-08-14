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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	infrastructurev1alpha2 "sigs.k8s.io/cluster-api-provider-docker/api/v1alpha2"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	capiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/cluster-api/pkg/util"
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

var machineKind = capiv1alpha2.SchemeGroupVersion.WithKind("Machine")

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

	// Store Docker Machine early state to allow patching.
	patch := client.MergeFrom(dockerMachine.DeepCopy())

	defer func() {
		if err := r.patchMachine(ctx, dockerMachine, patch); err != nil {
			r.Log.Error(err, "Error Patching DockerMachine", "name", dockerMachine.GetName())
			if reterr == nil {
				reterr = err
			}
		}
		return
	}()

	// Get the cluster api machine
	machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)

	if err != nil {
		return ctrl.Result{}, nil
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef on DockerMachine")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Get the cluster
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Error(err, "failed to get cluster")
	}

	// If the DockerMachine doesn't have finalizer, add it.
	if !util.Contains(dockerMachine.Finalizers, capiv1alpha2.MachineFinalizer) {
		dockerMachine.Finalizers = append(dockerMachine.Finalizers, infrastructurev1alpha2.MachineFinalizer)
	}

	//reconcileDelete dockerMachine
	if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("Start reconcileDelete dockerMachine")
		return r.reconcileDelete(ctx, cluster, machine, dockerMachine)
	}

	// create docker node
	if dockerMachine.Spec.ProviderID != nil {
		return ctrl.Result{}, nil
	}

	result, err := r.create(ctx, cluster, machine, dockerMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	return result, nil
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
	r.Log.Info("Creating a machine for cluster", "cluster-name", c.Name)
	clusterExists, err := kindcluster.IsKnown(c.Name)
	if err != nil {
		r.Log.Error(err, "Error finding cluster-name", "cluster", c.Name)
		return ctrl.Result{}, err
	}
	// If there's no cluster, requeue the request until there is one
	if !clusterExists {
		r.Log.Info("There is no cluster yet, waiting for a cluster before creating machines")
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}
	r.Log.Info("Is there a cluster?", "cluster-exists", clusterExists)

	if machine.Spec.Bootstrap.Data != nil {
		var node *nodes.Node
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
	return ctrl.Result{}, nil
}

func (r *DockerMachineReconciler) createFirstControlPlaneNode(
	ctx context.Context,
	c *capiv1alpha2.Cluster,
	machine *capiv1alpha2.Machine) (*nodes.Node, error) {

	r.Log.Info("Creating first controlplane node")

	node, err := actions.CreateControlPlane(c.Name, machine.GetName(), c.Status.APIEndpoints[0].Host, *machine.Spec.Version, nil)
	if err != nil {
		r.Log.Error(err, "Error creating control plane")
		return nil, err
	}

	s, err := kubeconfigToSecret(c.Name, c.Namespace)
	if err != nil {
		r.Log.Error(err, "Error converting kubeconfig to a secret")
		return nil, err
	}
	// Save the secret to the management cluster
	if err := r.Client.Create(ctx, s); err != nil {
		r.Log.Error(err, "Error saving secret to management cluster")
		return nil, err
	}
	return node, nil
}

func kubeconfigToSecret(clusterName, namespace string) (*v1.Secret, error) {
	// open kubeconfig file
	data, err := ioutil.ReadFile(actions.KubeConfigPath(clusterName))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	allNodes, err := nodes.List(fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// TODO: Clean this up at some point
	// The Management cluster, running the NodeRef controller, needs to talk to the child clusters.
	// The management cluster and child cluster must communicate over DockerIP address/ports.
	// The load balancer listens on <docker_ip>:6443 and exposes a port on the host at some random open port.
	// Any traffic directed to the nginx container will get round-robined to a control plane node in the cluster.
	// Since the NodeRef controller is running inside a container, it must reference the child cluster load balancer
	// host by using the Docker IP address and port 6443, but us, running on the docker host, must use the localhost
	// and random port the LB is exposing to our system.
	// Right now the secret that contains the kubeconfig will work only for the node ref controller. In order for *us*
	// to interact with the child clusters via kubeconfig we must take the secret uploaded,
	// rewrite the kube-apiserver-address to be 127.0.0.1:<randomly-assigned-by-docker-port>.
	// It's not perfect but it works to at least play with cluster-api v0.1.4
	lbip, _, err := actions.GetLoadBalancerHostAndPort(allNodes)
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		if bytes.Contains(line, []byte("https://")) {
			lines[i] = []byte(fmt.Sprintf("    server: https://%s:%d", lbip, 6443))
		}
	}

	// write it to a secret
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			// TODO pull in the kubeconfig secret function from cluster api
			Name:      fmt.Sprintf("%s-kubeconfig", clusterName),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			// TODO pull in constant from cluster api
			"value": bytes.Join(lines, []byte("\n")),
		},
	}, nil
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

	if isControlPlaneMachine(machine) {
		err := actions.DeleteControlPlane(cluster.GetName(), machine.GetName())
		if err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		}
	} else {
		err := actions.DeleteWorker(cluster.GetName(), machine.GetName())
		if err != nil {
			r.Log.Error(err, "Error deleting worker node", "nodeName", machine.GetName())
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
	// TODO(ncdc): remove this once we've updated to a version of controller-runtime with
	// https://github.com/kubernetes-sigs/controller-runtime/issues/526.
	gvk := dockerMachine.GroupVersionKind()
	if err := r.Patch(ctx, dockerMachine, patchConfig); err != nil {
		return err
	}
	// TODO(ncdc): remove this once we've updated to a version of controller-runtime with
	// https://github.com/kubernetes-sigs/controller-runtime/issues/526.
	dockerMachine.SetGroupVersionKind(gvk)
	if err := r.Status().Patch(ctx, dockerMachine, patchConfig); err != nil {
		return err
	}
	return nil
}
