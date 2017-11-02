package controller

import (
	"context"

	"github.com/golang/glog"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/cloud"
)

type MachineController struct {
	config     *Configuration
	restClient *rest.RESTClient
	actuator cloud.MachineActuator
}

func NewMachineController(config *Configuration) *MachineController{
	restClient, _, err := restClient(config.Kubeconfig)
	if err != nil {
		glog.Fatalf("error creating rest client: %v", err)
	}

	masterIP, err := host(config.Kubeconfig)
	if err != nil {
		glog.Fatalf("error getting master IP from rest client: %v", err)
	}

	// Determine cloud type from cluster CRD when available
	actuator, err := newMachineActuator(config.Cloud, config.KubeadmToken, masterIP)
	if err != nil {
		glog.Fatalf("error creating machine actuator: %v", err)
	}

	return &MachineController{
			config:config,
			restClient:restClient,
		actuator:actuator,
		}
}

func (c *MachineController) Run () error {
	glog.Infof("Running ...")

	// Run leader election

	return c.run(context.Background())
}

func (c *MachineController) run(ctx context.Context) error {
	source := cache.NewListWatchFromClient(c.restClient, "machines", apiv1.NamespaceAll, fields.Everything())

	_, informer := cache.NewInformer(
		source,
		&machinesv1.Machine{},
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
	machine := obj.(*machinesv1.Machine)
	glog.Infof("object created: %s\n", machine.ObjectMeta.Name)
	err := c.actuator.Create(machine)
	if err != nil {
		glog.Errorf("create machine %s failed: %v", machine.ObjectMeta.Name, err)
	}
	//TODO: wait for machine to become a node
}

func (c *MachineController) onUpdate(oldObj, newObj interface{}) {
	oldMachine := oldObj.(*machinesv1.Machine)
	newMachine := newObj.(*machinesv1.Machine)
	glog.Infof("object updated: %s\n", oldMachine.ObjectMeta.Name)
	glog.Infof("  old k8s version: %s, new: %s\n", oldMachine.Spec.Versions.Kubelet, newMachine.Spec.Versions.Kubelet)
	//TODO: remove (and possibly drain) node
	err := c.actuator.Delete(oldMachine)
	if err != nil {
		glog.Errorf("delete machine %s for update failed: %v", oldMachine.ObjectMeta.Name, err)
		return
	}
	err = c.actuator.Create(newMachine)
	if err != nil {
		glog.Errorf("create machine %s for update failed: %v", newMachine.ObjectMeta.Name, err)
	}
	//TODO: wait for machine to become a node
}

func (c *MachineController) onDelete(obj interface{}) {
	machine := obj.(*machinesv1.Machine)
	glog.Infof("object deleted: %s\n", machine.ObjectMeta.Name)
	//TODO: remove (and possibly drain) node
	err := c.actuator.Delete(machine)
	if err != nil {
		glog.Errorf("delete machine %s failed: %v", machine.ObjectMeta.Name, err)
	}
}

