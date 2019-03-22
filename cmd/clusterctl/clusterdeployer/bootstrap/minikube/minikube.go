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

package minikube

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog"
)

const (
	minikubeClusterName = "clusterapi"
)

type Minikube struct {
	kubeconfigpath string
	options        []string
	// minikubeExec implemented as function variable for testing hooks
	minikubeExec func(env []string, args ...string) (string, error)
}

func New() *Minikube {
	return WithOptions([]string{})
}

func WithOptions(options []string) *Minikube {
	return WithOptionsAndKubeConfigPath(options, "")
}

func WithOptionsAndKubeConfigPath(options []string, kubeconfigpath string) *Minikube {
	if kubeconfigpath == "" {
		// Arbitrary file name. Can potentially be randomly generated.
		kubeconfigpath = "minikube.kubeconfig"
	}

	// Set profile if it is not provided.
	if func() bool {
		for _, opt := range options {
			if strings.HasPrefix(opt, "p=") {
				return false
			}
		}
		return true
	}() {
		options = append(options, fmt.Sprintf("p=%s", minikubeClusterName))
	}

	return &Minikube{
		minikubeExec:   minikubeExec,
		options:        options,
		kubeconfigpath: kubeconfigpath,
	}
}

var minikubeExec = func(env []string, args ...string) (string, error) {
	const executable = "minikube"
	klog.V(3).Infof("Running: %v %v", executable, args)
	cmd := exec.Command(executable, args...)
	cmd.Env = env
	cmdOut, err := cmd.CombinedOutput()
	klog.V(2).Infof("Ran: %v %v Output: %v", executable, args, string(cmdOut))
	if err != nil {
		err = errors.Wrapf(err, "error running command '%v %v'", executable, strings.Join(args, " "))
	}
	return string(cmdOut), err
}

func (m *Minikube) Create() error {
	args := []string{"start", "--bootstrapper=kubeadm"}
	for _, opt := range m.options {
		args = append(args, fmt.Sprintf("--%v", opt))
	}
	_, err := m.exec(args...)
	return err
}

func (m *Minikube) Delete() error {
	_, err := m.exec("delete")
	os.Remove(m.kubeconfigpath)
	return err
}

func (m *Minikube) GetKubeconfig() (string, error) {
	b, err := ioutil.ReadFile(m.kubeconfigpath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (m *Minikube) exec(args ...string) (string, error) {
	// Override kubeconfig environment variable in call
	// so that minikube will generate and reference
	// the kubeconfig in the desired location.
	// Note that the last value set for a key is the final value.
	const kubeconfigEnvVar = "KUBECONFIG"
	env := append(os.Environ(), fmt.Sprintf("%v=%v", kubeconfigEnvVar, m.kubeconfigpath))
	return m.minikubeExec(env, args...)
}
