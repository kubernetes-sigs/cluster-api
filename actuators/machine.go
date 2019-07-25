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

package actuators

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	capiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha2"
	capierror "sigs.k8s.io/cluster-api/pkg/errors"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
)

const (
	// kind uses 'control-plane' and cluster-api uses 'controlplane'. Both use 'worker'.

	clusterAPIControlPlaneSetLabel = "controlplane"
)

// Machine defines a machine actuator type
type Machine struct {
	Core       corev1.CoreV1Interface
	ClusterAPI v1alpha2.ClusterV1alpha2Interface
	Log        logr.Logger
}

// Create creates a machine for a given cluster
// Note: have to print all the errors because cluster-api swallows them
func (m *Machine) Create(ctx context.Context, c *capiv1alpha2.Cluster, machine *capiv1alpha2.Machine) error {
	_ = machine.DeepCopy()
	m.Log.Info("Creating a machine for cluster", "cluster-name", c.Name)
	clusterExists, err := cluster.IsKnown(c.Name)
	if err != nil {
		m.Log.Error(err, "Error finding cluster-name", "cluster", c.Name)
		return err
	}
	// If there's no cluster, requeue the request until there is one
	if !clusterExists {
		m.Log.Info("There is no cluster yet, waiting for a cluster before creating machines")
		return &capierror.RequeueAfterError{RequeueAfter: 30 * time.Second}
	}

	controlPlanes, err := actions.ListControlPlanes(c.Name)
	if err != nil {
		m.Log.Error(err, "Error listing control planes")
		return err
	}
	m.Log.Info("Is there a cluster?", "cluster-exists", clusterExists)
	setValue := getRole(machine)
	m.Log.Info("This node has a role", "role", setValue)
	if setValue == clusterAPIControlPlaneSetLabel {
		if len(controlPlanes) > 0 {
			m.Log.Info("Adding a control plane")
			controlPlaneNode, err := actions.AddControlPlane(c.Name, machine.GetName(), *machine.Spec.Version)
			if err != nil {
				m.Log.Error(err, "Error adding control plane")
				return err
			}
			providerID := actions.ProviderID(controlPlaneNode.Name())
			machine.Spec.ProviderID = &providerID
			return nil
		}

		m.Log.Info("Creating a brand new cluster")
		elb, err := getExternalLoadBalancerNode(c.Name, m.Log)
		if err != nil {
			m.Log.Error(err, "Error getting external load balancer node")
			return err
		}
		lbipv4, _, err := elb.IP()
		if err != nil {
			m.Log.Error(err, "Error getting node IP address")
			return err
		}
		controlPlaneNode, err := actions.CreateControlPlane(c.Name, machine.GetName(), lbipv4, *machine.Spec.Version, nil)
		if err != nil {
			m.Log.Error(err, "Error creating control plane")
			return err
		}
		// set the machine's providerID
		providerID := actions.ProviderID(controlPlaneNode.Name())
		machine.Spec.ProviderID = &providerID

		s, err := kubeconfigToSecret(c.Name, c.Namespace)
		if err != nil {
			m.Log.Error(err, "Error converting kubeconfig to a secret")
			return err
		}
		// Save the secret to the management cluster
		if _, err := m.Core.Secrets(machine.GetNamespace()).Create(s); err != nil {
			m.Log.Error(err, "Error saving secret to management cluster")
			return err
		}
		return nil
	}

	// If there are no control plane then we should hold off on joining workers
	if len(controlPlanes) == 0 {
		m.Log.Info("Sending machine back since there is no cluster to join", "machine", machine.Name)
		return &capierror.RequeueAfterError{RequeueAfter: 30 * time.Second}
	}

	m.Log.Info("Creating a new worker node")
	worker, err := actions.AddWorker(c.Name, machine.GetName(), *machine.Spec.Version)
	if err != nil {
		m.Log.Error(err, "Error creating new worker node")
		return err
	}
	providerID := actions.ProviderID(worker.Name())
	machine.Spec.ProviderID = &providerID
	return nil
}

// Delete returns nil when the machine no longer exists or when a successful delete has happened.
func (m *Machine) Delete(ctx context.Context, cluster *capiv1alpha2.Cluster, machine *capiv1alpha2.Machine) error {
	exists, err := m.Exists(ctx, cluster, machine)
	if err != nil {
		return err
	}
	if exists {
		setValue := getRole(machine)
		if setValue == clusterAPIControlPlaneSetLabel {
			m.Log.Info("Deleting a control plane", "machine", machine.GetName())
			return actions.DeleteControlPlane(cluster.Name, machine.GetName())
		}
		m.Log.Info("Deleting a worker", "machine", machine.GetName())
		return actions.DeleteWorker(cluster.Name, machine.GetName())
	}
	return nil
}

// Update updates a machine
func (m *Machine) Update(ctx context.Context, cluster *capiv1alpha2.Cluster, machine *capiv1alpha2.Machine) error {
	m.Log.Info("Update machine is not implemented yet")
	return nil
}

// Exists returns true if a machine exists in the cluster
func (m *Machine) Exists(ctx context.Context, cluster *capiv1alpha2.Cluster, machine *capiv1alpha2.Machine) (bool, error) {
	if machine.Spec.ProviderID != nil {
		return true, nil
	}

	role := getRole(machine)
	kindRole := CAPIroleToKindRole(role)
	labels := []string{
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, kindRole),
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, cluster.Name),
		fmt.Sprintf("name=^%s$", machine.GetName()),
	}
	m.Log.Info("using labels", "labels", labels)
	nodeList, err := nodes.List(labels...)
	if err != nil {
		return false, err
	}
	m.Log.Info("found nodes", "nodes", nodeList)
	return len(nodeList) >= 1, nil
}

// patches the object and saves the status.

// CAPIroleToKindRole converts a CAPI role to kind role
// TODO there is a better way to do this.
func CAPIroleToKindRole(CAPIRole string) string {
	if CAPIRole == clusterAPIControlPlaneSetLabel {
		return constants.ControlPlaneNodeRoleValue
	}
	return CAPIRole
}
