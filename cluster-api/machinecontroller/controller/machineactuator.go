package controller

import (
	"github.com/golang/glog"

	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/machinecontroller/cloud"
	"k8s.io/kube-deploy/cluster-api/machinecontroller/cloud/google"
	"fmt"
)

func newMachineActuator(cloud string) (cloud.MachineActuator, error) {
	switch cloud {
	case "google":
		return google.NewMachineActuator()
	case "test":
		return &loggingMachineActuator{}, nil
	default:
		return nil, fmt.Errorf("Not recognized cloud provider: %s\n", cloud)
	}
}

// An actuator that just logs instead of doing anything.
type loggingMachineActuator struct {}

func (a loggingMachineActuator) Create(machine *machinesv1.Machine) error{
	glog.Infof("actuator received create: %s\n", machine.ObjectMeta.Name)
	return nil
}

func (a loggingMachineActuator) Delete(machine *machinesv1.Machine) error{
	glog.Infof("actuator received delete: %s\n", machine.ObjectMeta.Name)
	return nil

}

func (a loggingMachineActuator) Get(name string) (*machinesv1.Machine, error){
	glog.Infof("actuator received get %s\n", name)
	return &machinesv1.Machine{}, nil
}
