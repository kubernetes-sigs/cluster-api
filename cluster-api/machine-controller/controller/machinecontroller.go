/*
Copyright 2017 The Kubernetes Authors.

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

package controller

import (
	"context"
	"errors"
	"reflect"

	"github.com/golang/glog"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/client"
	"k8s.io/kube-deploy/cluster-api/cloud"
	"k8s.io/kube-deploy/cluster-api/util"
)

type MachineController struct {
	config        *Configuration
	restClient    *rest.RESTClient
	kubeClientSet *kubernetes.Clientset
	clusterClient *client.ClusterAPIV1Alpha1Client
	actuator      cloud.MachineActuator
	nodeWatcher   *NodeWatcher
	machineClient client.MachinesInterface
	runner        *asyncRunner
}

func NewMachineController(config *Configuration) *MachineController {
	restClient, err := restClient(config.Kubeconfig)
	if err != nil {
		glog.Fatalf("error creating rest client: %v", err)
	}

	kubeClientSet, err := kubeClientSet(config.Kubeconfig)
	if err != nil {
		glog.Fatalf("error creating kube client set: %v", err)
	}

	clusterClient := client.New(restClient)

	machineClient, err := machineClient(config.Kubeconfig)
	if err != nil {
		glog.Fatalf("error creating machine client: %v", err)
	}

	// Determine cloud type from cluster CRD when available
	actuator, err := cloud.NewMachineActuator(config.Cloud, config.KubeadmToken, machineClient)
	if err != nil {
		glog.Fatalf("error creating machine actuator: %v", err)
	}

	nodeWatcher, err := NewNodeWatcher(config.Kubeconfig)
	if err != nil {
		glog.Fatalf("error creating node watcher: %v", err)
	}

	return &MachineController{
		config:        config,
		restClient:    restClient,
		kubeClientSet: kubeClientSet,
		clusterClient: clusterClient,
		actuator:      actuator,
		nodeWatcher:   nodeWatcher,
		machineClient: machineClient,
		runner:        newAsyncRunner(),
	}
}

func (c *MachineController) Run() error {
	glog.Infof("Running ...")

	// Run leader election

	go func() {
		c.nodeWatcher.Run()
	}()

	return c.run(context.Background())
}

func (c *MachineController) run(ctx context.Context) error {
	source := cache.NewListWatchFromClient(c.restClient, "machines", apiv1.NamespaceAll, fields.Everything())

	_, informer := cache.NewInformer(
		source,
		&clusterv1.Machine{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
			DeleteFunc: c.onDelete,
		},
	)

	informer.Run(ctx.Done())
	return nil
}

func (c *MachineController) onAdd(obj interface{}) {
	machine := obj.(*clusterv1.Machine)
	glog.Infof("object created: %s\n", machine.ObjectMeta.Name)

	if ignored(machine) {
		return
	}

	c.runner.runAsync(machine.ObjectMeta.Name, func() {
		err := c.create(machine)
		if err != nil {
			glog.Errorf("create machine %s failed: %v", machine.ObjectMeta.Name, err)
		} else {
			glog.Infof("create machine %s succeded.", machine.ObjectMeta.Name)
		}
	})
}

func (c *MachineController) onUpdate(oldObj, newObj interface{}) {
	oldMachine := oldObj.(*clusterv1.Machine)
	newMachine := newObj.(*clusterv1.Machine)
	glog.Infof("object updated: %s\n", oldMachine.ObjectMeta.Name)
	glog.Infof("  old k8s version: %s, new: %s\n", oldMachine.Spec.Versions.Kubelet, newMachine.Spec.Versions.Kubelet)

	if !c.requiresUpdate(newMachine, oldMachine) {
		glog.Infof("machine %s change does not require any update action to be taken.", oldMachine.ObjectMeta.Name)
		return
	}

	c.runner.runAsync(newMachine.ObjectMeta.Name, func() {
		err := c.update(oldMachine, newMachine)
		if err != nil {
			glog.Errorf("update machine %s failed: %v", newMachine.ObjectMeta.Name, err)
		} else {
			glog.Infof("update machine %s succeded.", newMachine.ObjectMeta.Name)
		}
	})
}

func (c *MachineController) onDelete(obj interface{}) {
	machine := obj.(*clusterv1.Machine)
	glog.Infof("object deleted: %s\n", machine.ObjectMeta.Name)

	if ignored(machine) {
		return
	}

	c.runner.runAsync(machine.ObjectMeta.Name, func() {
		err := c.delete(machine)
		if err != nil {
			glog.Errorf("delete machine %s failed: %v", machine.ObjectMeta.Name, err)
		} else {
			glog.Infof("delete machine %s succeded.", machine.ObjectMeta.Name)
		}
	})
}

func ignored(machine *clusterv1.Machine) bool {
	if util.IsMaster(machine) {
		glog.Infof("Ignoring master machine\n")
		return true
	}
	return false
}

// The two machines differ in a way that requires an update
func (c *MachineController) requiresUpdate(a *clusterv1.Machine, b *clusterv1.Machine) bool {
	// Do not want status changes. Do want changes that impact machine provisioning
	return !reflect.DeepEqual(a.Spec.ObjectMeta, b.Spec.ObjectMeta) ||
		!reflect.DeepEqual(a.Spec.ProviderConfig, b.Spec.ProviderConfig) ||
		!reflect.DeepEqual(a.Spec.Roles, b.Spec.Roles) ||
		!reflect.DeepEqual(a.Spec.Versions, b.Spec.Versions) ||
		a.ObjectMeta.Name != b.ObjectMeta.Name
}

func (c *MachineController) create(machine *clusterv1.Machine) error {
	cluster, err := c.getCluster()
	if err != nil {
		return err
	}

	// Sometimes old events get replayed even though they have already been processed by this
	// controller. Temporarily work around this by checking if the machine CRD actually exists
	// on create.
	_, err = c.machineClient.Get(machine.ObjectMeta.Name, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("Skipping machine create due to error getting machine %v: %v\n", machine.ObjectMeta.Name, err)
		return err
	}

	return c.actuator.Create(cluster, machine)
}

func (c *MachineController) delete(machine *clusterv1.Machine) error {
	c.kubeClientSet.CoreV1().Nodes().Delete(machine.ObjectMeta.Name, &metav1.DeleteOptions{})
	if err := c.actuator.Delete(machine); err != nil {
		return err
	}
	// Do a second node cleanup after the delete completes in case the node joined the cluster
	// while the deletion of the machine was mid-way.
	c.kubeClientSet.CoreV1().Nodes().Delete(machine.ObjectMeta.Name, &metav1.DeleteOptions{})
	return nil
}

func (c *MachineController) update(old_machine *clusterv1.Machine, new_machine *clusterv1.Machine) error {
	cluster, err := c.getCluster()
	if err != nil {
		return err
	}

	// TODO: Assume single master for now.
	// TODO: Assume we never change the role for the machines. (Master->Node, Node->Master, etc)
	return c.actuator.Update(cluster, old_machine, new_machine)
}

//TODO: we should cache this locally and update with an informer
func (c *MachineController) getCluster() (*clusterv1.Cluster, error) {
	clusters, err := c.clusterClient.Clusters().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	switch len(clusters.Items) {
	case 0:
		return nil, errors.New("no clusters defined")
	case 1:
		return &clusters.Items[0], nil
	default:
		return nil, errors.New("multiple clusters defined")
	}
}
