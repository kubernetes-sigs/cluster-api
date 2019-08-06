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
	"sigs.k8s.io/kind/pkg/cluster"
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
func (r *DockerMachineReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
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

	// Find the owner reference
	var machineRef *metav1.OwnerReference
	for _, ref := range dockerMachine.OwnerReferences {
		if ref.Kind == machineKind.Kind && ref.APIVersion == machineKind.GroupVersion().String() {
			machineRef = &ref
			break
		}
	}
	if machineRef == nil {
		log.Info("did not find matching machine reference")
		return ctrl.Result{}, nil
	}

	// Get the cluster api machine
	machine := &capiv1alpha2.Machine{}
	machineKey := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      machineRef.Name,
	}

	if err := r.Get(ctx, machineKey, machine); err != nil {
		log.Error(err, "failed to get machine")
		return ctrl.Result{}, err
	}

	if machine.Labels[capiv1alpha2.MachineClusterLabelName] == "" {
		return ctrl.Result{}, errors.New("machine has no associated cluster")
	}

	// Get the cluster
	cluster := &capiv1alpha2.Cluster{}
	clusterKey := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      machine.Labels[capiv1alpha2.MachineClusterLabelName],
	}
	if err := r.Get(ctx, clusterKey, cluster); err != nil {
		log.Error(err, "failed to get cluster")
		return ctrl.Result{}, err
	}

	// create docker node
	if dockerMachine.Spec.ProviderID != nil {
		return ctrl.Result{}, nil
	}

	result, err := r.create(ctx, cluster, machine, dockerMachine)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Client.Update(ctx, dockerMachine); err != nil {
		return ctrl.Result{}, err
	}

	// TODO should the cluster be deleted?

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
	dockerMachine *infrastructurev1alpha2.DockerMachine,
) (ctrl.Result, error) {
	r.Log.Info("Creating a machine for cluster", "cluster-name", c.Name)
	clusterExists, err := cluster.IsKnown(c.Name)
	if err != nil {
		r.Log.Error(err, "Error finding cluster-name", "cluster", c.Name)
		return ctrl.Result{}, err
	}
	// If there's no cluster, requeue the request until there is one
	if !clusterExists {
		r.Log.Info("There is no cluster yet, waiting for a cluster before creating machines")
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}
	controlPlanes, err := actions.ListControlPlanes(c.Name)
	if err != nil {
		r.Log.Error(err, "Error listing control planes")
		return ctrl.Result{}, err
	}
	r.Log.Info("Is there a cluster?", "cluster-exists", clusterExists)
	setValue := getRole(machine)
	r.Log.Info("This node has a role", "role", setValue)
	if setValue == clusterAPIControlPlaneSetLabel {
		// joining controlplane
		if len(controlPlanes) > 0 {
			r.Log.Info("Adding a control plane node", "machine-name", machine.GetName(), "cluster-name", c.Name)
			controlPlaneNode, err := actions.AddControlPlane(c.Name, machine.GetName(), *machine.Spec.Version)
			if err != nil {
				r.Log.Error(err, "Error adding control plane")
				return ctrl.Result{}, err
			}
			providerID := actions.ProviderID(controlPlaneNode.Name())
			dockerMachine.Spec.ProviderID = &providerID
			dockerMachine.Status.Ready = true
			return ctrl.Result{}, nil
		}

		r.Log.Info("Creating a brand new controlplane node")
		elb, err := actions.GetExternalLoadBalancerNode(c.Name)
		if err != nil {
			r.Log.Error(err, "Error getting external load balancer node")
			return ctrl.Result{}, err
		}
		if elb == nil {
			r.Log.Info("Cluster has no ELB yet, waiting for cluster network to be reconciled")
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}

		lbipv4, _, err := elb.IP()
		if err != nil {
			r.Log.Error(err, "Error getting IP address for ELB")
			return ctrl.Result{}, err
		}
		controlPlaneNode, err := actions.CreateControlPlane(c.Name, machine.GetName(), lbipv4, *machine.Spec.Version, nil)
		if err != nil {
			r.Log.Error(err, "Error creating control plane")
			return ctrl.Result{}, err
		}
		// set the machine's providerID
		providerID := actions.ProviderID(controlPlaneNode.Name())
		dockerMachine.Spec.ProviderID = &providerID
		dockerMachine.Status.Ready = true

		s, err := kubeconfigToSecret(c.Name, c.Namespace)
		if err != nil {
			r.Log.Error(err, "Error converting kubeconfig to a secret")
			return ctrl.Result{}, err
		}
		// Save the secret to the management cluster
		if err := r.Client.Create(ctx, s); err != nil {
			r.Log.Error(err, "Error saving secret to management cluster")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// If there are no control plane then we should hold off on joining workers
	if len(controlPlanes) == 0 {
		r.Log.Info("Sending machine back since there is no cluster to join", "machine", machine.Name)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	r.Log.Info("Creating a new worker node")
	worker, err := actions.AddWorker(c.Name, machine.GetName(), *machine.Spec.Version)
	if err != nil {
		r.Log.Error(err, "Error creating new worker node")
		return ctrl.Result{}, err
	}
	providerID := actions.ProviderID(worker.Name())
	dockerMachine.Spec.ProviderID = &providerID
	dockerMachine.Status.Ready = true
	return ctrl.Result{}, nil
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
