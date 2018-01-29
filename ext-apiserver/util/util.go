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

package util

import (
	"fmt"
	"os/exec"
	"math/rand"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	clusterv1 "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1"
	client "k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	TypeMaster = "Master"
)

const (
	CharSet = "0123456789abcdefghijklmnopqrstuvwxyz"
)

var (
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func RandomToken() string {
	return fmt.Sprintf("%s.%s", RandomString(6), RandomString(16))
}

func RandomString(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = CharSet[r.Intn(len(CharSet))]
	}
	return string(result)
}

func GetMaster(machines []*clusterv1.Machine) *clusterv1.Machine {
	for _, machine := range machines {
		if IsMaster(machine) {
			return machine
		}
	}
	return nil
}

func MachineP(machines []clusterv1.Machine) []*clusterv1.Machine {
	// Convert to list of pointers
	var ret []*clusterv1.Machine
	for _, machine := range machines {
		ret = append(ret, machine.DeepCopy())
	}
	return ret
}

func NewClientSet(configPath string) (*apiextensionsclient.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}

	cs, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return cs, nil
}

func GetCurrentMachineIfExists(machineClient client.MachineInterface, machine *clusterv1.Machine) (*clusterv1.Machine, error) {
	return GetMachineIfExists(machineClient, machine.ObjectMeta.Name, machine.ObjectMeta.UID)
}

func GetMachineIfExists(machineClient client.MachineInterface, name string, uid types.UID) (*clusterv1.Machine, error) {
	if machineClient == nil {
		// Being called before k8s is setup as part of master VM creation
		return nil, nil
	}

	// Machines are identified by name and UID
	machine, err := machineClient.Get(name, metav1.GetOptions{})
	if err != nil {
		// TODO: Use formal way to check for not found
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}

	if machine.ObjectMeta.UID != uid {
		return nil, nil
	}
	return machine, nil
}

func Contains(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func IsMaster(machine *clusterv1.Machine) bool {
	return Contains(TypeMaster, machine.Spec.Roles)
}

func ExecCommand(name string, args ...string) string {
	cmdOut, _ := exec.Command(name, args...).Output()
	return string(cmdOut)
}