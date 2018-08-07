/*
Copyright 2018 The Kubernetes Authors.

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

package machine

import (
	"errors"
	"os"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
	"sigs.k8s.io/cluster-api/pkg/util"
)

const NodeNameEnvVar = "NODE_NAME"

// +controller:group=cluster,version=v1alpha1,kind=Machine,resource=machines
type MachineControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about Machine
	lister listers.MachineLister

	actuator Actuator

	kubernetesClientSet kubernetes.Interface
	clientSet           clientset.Interface
	linkedNodes         map[string]bool
	cachedReadiness     map[string]bool

	// nodeName is the name of the node on which the machine controller is running, if not present, it is loaded from NODE_NAME.
	nodeName string
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *MachineControllerImpl) Init(arguments sharedinformers.ControllerInitArguments, actuator Actuator) {
	// Use the lister for indexing machines labels
	c.lister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Machines().Lister()

	if c.nodeName == "" {
		c.nodeName = os.Getenv(NodeNameEnvVar)
		if c.nodeName == "" {
			glog.Warningf("environment variable %v is not set, this controller will not protect against deleting its own machine", NodeNameEnvVar)
		}
	}
	clientset, err := clientset.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("error creating machine client: %v", err)
	}
	c.clientSet = clientset
	c.kubernetesClientSet = arguments.GetSharedInformers().KubernetesClientSet

	c.linkedNodes = make(map[string]bool)
	c.cachedReadiness = make(map[string]bool)

	c.actuator = actuator

	// Start watching for Node resource. It will effectively create a new worker queue, and
	// reconcileNode() will be invoked in a loop to handle the reconciling.
	ni := arguments.GetSharedInformers().KubernetesFactory.Core().V1().Nodes()
	arguments.GetSharedInformers().Watch("NodeWatcher", ni.Informer(), nil, c.reconcileNode)
}

// Reconcile handles enqueued messages. The delete will be handled by finalizer.
func (c *MachineControllerImpl) Reconcile(machine *clusterv1.Machine) error {
	// Deep-copy otherwise we are mutating our cache.
	m := machine.DeepCopy()
	// Implement controller logic here
	name := m.Name
	glog.Infof("Running reconcile Machine for %s\n", name)

	if !m.ObjectMeta.DeletionTimestamp.IsZero() {
		// no-op if finalizer has been removed.
		if !util.Contains(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer) {
			glog.Infof("reconciling machine object %v causes a no-op as there is no finalizer.", name)
			return nil
		}
		if !c.isDeleteAllowed(machine) {
			glog.Infof("Skipping reconciling of machine object %v", name)
			return nil
		}
		glog.Infof("reconciling machine object %v triggers delete.", name)
		if err := c.delete(m); err != nil {
			glog.Errorf("Error deleting machine object %v; %v", name, err)
			return err
		}

		// Remove finalizer on successful deletion.
		glog.Infof("machine object %v deletion successful, removing finalizer.", name)
		m.ObjectMeta.Finalizers = util.Filter(m.ObjectMeta.Finalizers, clusterv1.MachineFinalizer)
		if _, err := c.clientSet.ClusterV1alpha1().Machines(m.Namespace).Update(m); err != nil {
			glog.Errorf("Error removing finalizer from machine object %v; %v", name, err)
			return err
		}
		return nil
	}

	cluster, err := c.getCluster(m)
	if err != nil {
		return err
	}

	exist, err := c.actuator.Exists(cluster, m)
	if err != nil {
		glog.Errorf("Error checking existence of machine instance for machine object %v; %v", name, err)
		return err
	}
	if exist {
		glog.Infof("Reconciling machine object %v triggers idempotent update.", name)
		return c.update(m)
	}
	// Machine resource created. Machine does not yet exist.
	glog.Infof("Reconciling machine object %v triggers idempotent create.", m.ObjectMeta.Name)
	if err := c.create(m); err != nil {
		glog.Warningf("unable to create machine %v: %v", name, err)
		return err
	}
	return nil
}

func (c *MachineControllerImpl) Get(namespace, name string) (*clusterv1.Machine, error) {
	return c.lister.Machines(namespace).Get(name)
}

func (c *MachineControllerImpl) create(machine *clusterv1.Machine) error {
	cluster, err := c.getCluster(machine)
	if err != nil {
		return err
	}

	return c.actuator.Create(cluster, machine)
}

func (c *MachineControllerImpl) update(new_machine *clusterv1.Machine) error {
	cluster, err := c.getCluster(new_machine)
	if err != nil {
		return err
	}

	// TODO: Assume single master for now.
	// TODO: Assume we never change the role for the machines. (Master->Node, Node->Master, etc)
	return c.actuator.Update(cluster, new_machine)
}

func (c *MachineControllerImpl) delete(machine *clusterv1.Machine) error {
	cluster, err := c.getCluster(machine)
	if err != nil {
		return err
	}

	return c.actuator.Delete(cluster, machine)
}

func (c *MachineControllerImpl) getCluster(machine *clusterv1.Machine) (*clusterv1.Cluster, error) {
	clusterList, err := c.clientSet.ClusterV1alpha1().Clusters(machine.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	switch len(clusterList.Items) {
	case 0:
		return nil, errors.New("no clusters defined")
	case 1:
		return &clusterList.Items[0], nil
	default:
		return nil, errors.New("multiple clusters defined")
	}
}

func (c *MachineControllerImpl) isDeleteAllowed(machine *clusterv1.Machine) bool {
	if c.nodeName == "" || machine.Status.NodeRef == nil {
		return true
	}
	if machine.Status.NodeRef.Name != c.nodeName {
		return true
	}
	node, err := c.kubernetesClientSet.CoreV1().Nodes().Get(c.nodeName, metav1.GetOptions{})
	if err != nil {
		glog.Infof("unable to determine if controller's node is associated with machine '%v', error getting node named '%v': %v", machine.Name, c.nodeName, err)
		return true
	}
	// When the UID of the machine's node reference and this controller's actual node match then then the request is to
	// delete the machine this machine-controller is running on. Return false to not allow machine controller to delete its
	// own machine.
	return node.UID != machine.Status.NodeRef.UID
}
