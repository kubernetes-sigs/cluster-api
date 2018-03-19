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

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kube-deploy/ext-apiserver/cloud"
	clusterv1 "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1"
	"k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/clientset"
	"k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	listers "k8s.io/kube-deploy/ext-apiserver/pkg/client/listers_generated/cluster/v1alpha1"
	cfg "k8s.io/kube-deploy/ext-apiserver/pkg/controller/config"
	"k8s.io/kube-deploy/ext-apiserver/pkg/controller/sharedinformers"
	"k8s.io/kube-deploy/ext-apiserver/util"
)

// +controller:group=cluster,version=v1alpha1,kind=Machine,resource=machines
type MachineControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about Machine
	lister listers.MachineLister

	actuator cloud.MachineActuator

	kubernetesClientSet *kubernetes.Clientset
	clientSet           *clientset.Clientset
	machineClient       v1alpha1.MachineInterface
	linkedNodes         map[string]bool
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *MachineControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing machines labels
	c.lister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Machines().Lister()

	clientset, err := clientset.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("error creating machine client: %v", err)
	}
	c.clientSet = clientset
	c.kubernetesClientSet = arguments.GetSharedInformers().KubernetesClientSet

	c.linkedNodes = make(map[string]bool)

	// Create machine actuator.
	// TODO: Assume default namespace for now. Maybe a separate a controller per namespace?
	c.machineClient = clientset.ClusterV1alpha1().Machines(corev1.NamespaceDefault)
	var config *cfg.Configuration = &cfg.ControllerConfig
	actuator, err := cloud.NewMachineActuator(config.Cloud, config.KubeadmToken, c.machineClient)
	if err != nil {
		glog.Fatalf("error creating machine actuator: %v", err)
	}
	c.actuator = actuator

	// Start watching for Node resource. It will effectively create a new worker queue, and
	// reconcileNode() will be invoked in a loop to handle the reconciling.
	ni := arguments.GetSharedInformers().KubernetesFactory.Core().V1().Nodes()
	arguments.GetSharedInformers().Watch("NodeWatcher", ni.Informer(), nil, c.reconcileNode)
}

// Reconcile handles enqueued messages. The delete will be handled by finalizer.
func (c *MachineControllerImpl) Reconcile(machine *clusterv1.Machine) error {
	// Implement controller logic here
	glog.Infof("Running reconcile Machine for %s\n", machine.Name)
	exist, err := c.actuator.Exists(machine)
	if err == nil {
		if !exist {
			// Machine resource created. Machine does not yet exist.
			glog.Infof("reconciling machine object %v triggers idempotent create.", machine.ObjectMeta.Name)
			err = c.create(machine)
		} else {

			if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
				// no-op if finalizer has been removed.
				if !util.Contains(machine.ObjectMeta.Finalizers, clusterv1.MachineFinalizer) {
					glog.Infof("reconciling machine object %v causes a no-op as there is no finalizer.", machine.ObjectMeta.Name)
					return nil
				}
				if cfg.ControllerConfig.InCluster && util.IsMaster(machine) {
					glog.Infof("skipping reconciling master machine object %v", machine.ObjectMeta.Name)
				} else {
					glog.Infof("reconciling machine object %v triggers delete.", machine.ObjectMeta.Name)
					err = c.delete(machine)
				}
			} else {
				glog.Infof("reconciling machine object %v triggers idempotent update.", machine.ObjectMeta.Name)
				err = c.update(machine)
			}
		}
	}
	if err != nil {
		glog.Errorf("reconciling failed with err: %v", err)
	}
	return err
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
	return c.actuator.Delete(machine)
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
