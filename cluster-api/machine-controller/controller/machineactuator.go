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
	"fmt"

	"github.com/golang/glog"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/cloud"
	"k8s.io/kube-deploy/cluster-api/cloud/google"
)

func newMachineActuator(cloud string, kubeadmToken string, masterIP string) (cloud.MachineActuator, error) {
	switch cloud {
	case "google":
		return google.NewMachineActuator(kubeadmToken, masterIP)
	case "test":
		return &loggingMachineActuator{}, nil
	default:
		return nil, fmt.Errorf("Not recognized cloud provider: %s\n", cloud)
	}
}

// An actuator that just logs instead of doing anything.
type loggingMachineActuator struct{}

func (a loggingMachineActuator) Create(machine *clusterv1.Machine) error {
	glog.Infof("actuator received create: %s\n", machine.ObjectMeta.Name)
	return nil
}

func (a loggingMachineActuator) Delete(machine *clusterv1.Machine) error {
	glog.Infof("actuator received delete: %s\n", machine.ObjectMeta.Name)
	return nil

}

func (a loggingMachineActuator) Get(name string) (*clusterv1.Machine, error) {
	glog.Infof("actuator received get %s\n", name)
	return &clusterv1.Machine{}, nil
}

func (a loggingMachineActuator) GetIP(machine *clusterv1.Machine) (string, error) {
	glog.Infof("actuator received GetIP: %s\n", machine.ObjectMeta.Name)
	return "", nil
}

func (a loggingMachineActuator) GetKubeConfig(master *clusterv1.Machine) (string, error) {
	glog.Infof("actuator received GetKubeConfig: %s\n", master.ObjectMeta.Name)
	return "", nil
}

func (a loggingMachineActuator) CreateMachineController(machines []*clusterv1.Machine) error {
	glog.Infof("actuator received CreateMachineController: %q\n", machines)
	return nil
}
