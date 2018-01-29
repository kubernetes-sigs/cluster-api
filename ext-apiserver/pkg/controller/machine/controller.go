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

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/kube-deploy/ext-apiserver/cloud"
	clusterv1 "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1"
	"k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/clientset"
	listers "k8s.io/kube-deploy/ext-apiserver/pkg/client/listers_generated/cluster/v1alpha1"
	cfg "k8s.io/kube-deploy/ext-apiserver/pkg/controller/config"
	"k8s.io/kube-deploy/ext-apiserver/pkg/controller/sharedinformers"
)

// +controller:group=cluster,version=v1alpha1,kind=Machine,resource=machines
type MachineControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about Machine
	lister listers.MachineLister

	// lister indexes properties about Cluster
	clusterLister listers.ClusterLister

	actuator cloud.MachineActuator

	clientSet *clientset.Clientset
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *MachineControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing machines labels
	c.lister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Machines().Lister()
	c.clusterLister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Clusters().Lister()

	clientset, err := clientset.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("error creating machine client: %v", err)
	}
	c.clientSet = clientset

	// Create machine actuator.
	// TODO: Assume default namespace for now. Maybe a separate a controller per namespace?
	machInterface := clientset.ClusterV1alpha1().Machines(apiv1.NamespaceDefault)
	var config *cfg.Configuration = &cfg.ControllerConfig
	actuator, err := cloud.NewMachineActuator(config.Cloud, config.KubeadmToken, machInterface)
	if err != nil {
		glog.Fatalf("error creating machine actuator: %v", err)
	}
	c.actuator = actuator
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
			glog.Infof("reconciling machine object %v triggers idempotent update.", machine.ObjectMeta.Name)
			err = c.update(machine)
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
	cluster, err := c.getCluster()
	if err != nil {
		return err
	}

	return c.actuator.Create(cluster, machine)
}

func (c *MachineControllerImpl) update(new_machine *clusterv1.Machine) error {
	cluster, err := c.getCluster()
	if err != nil {
		return err
	}

	// TODO: Assume single master for now.
	// TODO: Assume we never change the role for the machines. (Master->Node, Node->Master, etc)
	return c.actuator.Update(cluster, new_machine)
}

func (c *MachineControllerImpl) getCluster() (*clusterv1.Cluster, error) {
	clusters, err := c.clusterLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	switch len(clusters) {
	case 0:
		return nil, errors.New("no clusters defined")
	case 1:
		return clusters[0], nil
	default:
		return nil, errors.New("multiple clusters defined")
	}
}
