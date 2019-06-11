// Copyright 2019 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package capkactuators

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/pkg/errors"
	"gitlab.com/chuckh/cluster-api-provider-kind/kind/actions"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	capierror "sigs.k8s.io/cluster-api/pkg/controller/error"
	"sigs.k8s.io/controller-runtime/pkg/patch"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
)

type Machine struct {
	Core       corev1.CoreV1Interface
	ClusterAPI v1alpha1.ClusterV1alpha1Interface
}

func NewMachineActuator(clusterapi v1alpha1.ClusterV1alpha1Interface, core corev1.CoreV1Interface) *Machine {
	return &Machine{
		Core:       core,
		ClusterAPI: clusterapi,
	}
}

// Have to print all the errors because cluster-api swallows them
func (m *Machine) Create(ctx context.Context, c *clusterv1.Cluster, machine *clusterv1.Machine) error {
	old := machine.DeepCopy()
	fmt.Printf("Creating a machine for cluster %q\n", c.Name)
	clusterExists, err := cluster.IsKnown(c.Name)
	if err != nil {
		fmt.Printf("%+v", err)
		return err
	}
	// If there's no cluster, requeue the request until there is one
	if !clusterExists {
		fmt.Println("There is no cluster yet, waiting for a cluster before creating machines")
		return &capierror.RequeueAfterError{RequeueAfter: 30 * time.Second}
	}

	controlPlanes, err := actions.ListControlPlanes(c.Name)
	if err != nil {
		fmt.Printf("%+v\n", err)
		return err
	}
	fmt.Printf("Is there a cluster? %v\n", clusterExists)
	setValue := getRole(machine)
	fmt.Printf("This node has a role of %q\n", setValue)
	if setValue == constants.ControlPlaneNodeRoleValue {
		if len(controlPlanes) > 0 {
			fmt.Println("Adding a control plane")
			controlPlaneNode, err := actions.AddControlPlane(c.Name)
			if err != nil {
				fmt.Printf("%+v", err)
				return err
			}
			setKindName(machine, controlPlaneNode.Name())
			return m.save(old, machine)
		}

		fmt.Println("Creating a brand new cluster")
		controlPlaneNode, err := actions.CreateControlPlane(c.Name)
		if err != nil {
			fmt.Printf("%+v", err)
			return err
		}
		setKindName(machine, controlPlaneNode.Name())
		if err := m.save(old, machine); err != nil {
			fmt.Printf("%+v", err)
			return err
		}
		s, err := kubeconfigToSecret(c.Name, c.Namespace)
		if err != nil {
			fmt.Printf("%+v", err)
			return err
		}
		// Save the secret to the management cluster
		if _, err := m.Core.Secrets(machine.GetNamespace()).Create(s); err != nil {
			fmt.Printf("%+v", err)
			return err
		}
		return nil
	}

	// If there are no control plane then we should hold off on joining workers
	if len(controlPlanes) == 0 {
		fmt.Printf("Sending machine %q back since there is no cluster to join\n", machine.Name)
		return &capierror.RequeueAfterError{RequeueAfter: 30 * time.Second}
	}

	fmt.Println("Creating a new worker node")
	worker, err := actions.AddWorker(c.Name)
	if err != nil {
		fmt.Printf("%+v", err)
		return err
	}
	setKindName(machine, worker.Name())
	return m.save(old, machine)
}
func (m *Machine) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	return actions.DeleteNode(cluster.Name, getKindName(machine))
}

func (m *Machine) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	fmt.Println("Update machine is not implemented yet.")
	return nil
}

func (m *Machine) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	if getKindName(machine) == "" {
		return false, nil
	}
	fmt.Println("Looking for a docker container named", getKindName(machine))
	role := getRole(machine)
	labels := []string{
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, role),
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, cluster.Name),
		fmt.Sprintf("name=^%s$", getKindName(machine)),
	}
	fmt.Printf("using labels: %v\n", labels)
	nodeList, err := nodes.List(labels...)
	if err != nil {
		return false, err
	}
	fmt.Printf("found nodes: %v\n", nodeList)
	return len(nodeList) >= 1, nil
}

func (m *Machine) save(old, new *clusterv1.Machine) error {
	fmt.Println("updating machine")
	p, err := patch.NewJSONPatch(old, new)
	if err != nil {
		fmt.Printf("%+v\n", err)
		return err
	}
	fmt.Println("Patches", p)
	if len(p) != 0 {
		pb, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			fmt.Printf("%+v\n", err)
			return err
		}
		if _, err := m.ClusterAPI.Machines(old.Namespace).Patch(new.Name, types.JSONPatchType, pb); err != nil {
			fmt.Printf("%+v\n", err)
			return err
		}
		fmt.Println("updated")
	}
	return nil
}

func setKindName(machine *clusterv1.Machine, name string) {
	a := machine.GetAnnotations()
	a["name"] = name
	machine.SetAnnotations(a)
}

func getKindName(machine *clusterv1.Machine) string {
	annotations := machine.GetAnnotations()
	return annotations["name"]
}

func getRole(machine *clusterv1.Machine) string {
	// Figure out what kind of node we're making
	annotations := machine.GetAnnotations()
	setValue, ok := annotations["set"]
	if !ok {
		setValue = constants.WorkerNodeRoleValue
	}
	return setValue
}

type Cluster struct{}

func NewClusterActuator() *Cluster {
	return &Cluster{}
}

func (c *Cluster) Reconcile(cluster *clusterv1.Cluster) error {
	elb, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, constants.ExternalLoadBalancerNodeRoleValue),
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, cluster.Name),
	)
	if err != nil {
		fmt.Printf("%+v\n", err)
		return err
	}
	fmt.Println("found external load balancers:", elb)
	// Abandon if we already have a load balancer.
	if len(elb) > 0 {
		fmt.Println("Nothing to do for this cluster.")
		return nil
	}
	fmt.Printf("The cluster named %q has been created! Setting up some infrastructure.\n", cluster.Name)
	return actions.SetUpLoadBalancer(cluster.Name)
}

func (c *Cluster) Delete(cluster *clusterv1.Cluster) error {
	fmt.Println("Cluster delete is not implemented.")
	return nil
}

func kubeconfigToSecret(clusterName, namespace string) (*v1.Secret, error) {
	// open kubeconfig file
	data, err := ioutil.ReadFile(actions.KubeConfigPath(clusterName))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// write it to a secret
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("kubeconfig-%s", clusterName),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": data,
		},
	}, nil
}
