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
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clustercommon "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/common"
	clusterv1 "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1"
	"k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/clientset"
	client "k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
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

func Home() string {
	home := os.Getenv("HOME")
	if strings.Contains(home, "root") {
		return "/root"
	}

	usr, err := user.Current()
	if err != nil {
		glog.Warningf("unable to find user: %v", err)
		return ""
	}
	return usr.HomeDir
}

func GetDefaultKubeConfigPath() string {
	localDir := fmt.Sprintf("%s/.kube", Home())
	if _, err := os.Stat(localDir); os.IsNotExist(err) {
		if err := os.Mkdir(localDir, 0777); err != nil {
			glog.Fatal(err)
		}
	}
	return fmt.Sprintf("%s/config", localDir)
}

func NewClientSet(configPath string) (*clientset.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}

	cs, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return cs, nil
}

func NewKubernetesClient(configPath string) (*kubernetes.Clientset, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("kubectl config file %s doesn't exist. Is kubectl configured to access a cluster?", configPath)
	}

	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}

	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return c, nil
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

func RoleContains(a clustercommon.MachineRole, list []clustercommon.MachineRole) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func IsMaster(machine *clusterv1.Machine) bool {
	return RoleContains(clustercommon.MasterRole, machine.Spec.Roles)
}

func IsNodeReady(node *v1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady {
			return condition.Status == v1.ConditionTrue
		}
	}

	return false
}

func Copy(m *clusterv1.Machine) *clusterv1.Machine {
	ret := &clusterv1.Machine{}
	ret.APIVersion = m.APIVersion
	ret.Kind = m.Kind
	ret.ClusterName = m.ClusterName
	ret.GenerateName = m.GenerateName
	ret.Name = m.Name
	ret.Namespace = m.Namespace
	m.Spec.DeepCopyInto(&ret.Spec)
	return ret
}

func ExecCommand(name string, args ...string) string {
	cmdOut, _ := exec.Command(name, args...).Output()
	return string(cmdOut)
}

func Filter(list []string, strToFilter string) (newList []string) {
	for _, item := range list {
		if item != strToFilter {
			newList = append(newList, item)
		}
	}
	return
}

func Contains(list []string, strToSearch string) bool {
	for _, item := range list {
		if item == strToSearch {
			return true
		}
	}
	return false
}
