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

package options

import (
	"github.com/spf13/pflag"
	"sigs.k8s.io/cluster-api/pkg/controller/config"
)

type MachineControllerServer struct {
	CommonConfig            *config.Configuration
	KubeadmToken            string
	MachineSetupConfigsPath string
}

func NewMachineControllerServer() *MachineControllerServer {
	s := MachineControllerServer{
		CommonConfig: &config.ControllerConfig,
	}
	return &s
}

func (s *MachineControllerServer) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.KubeadmToken, "token", s.KubeadmToken, "Kubeadm token to use to join new machines")
	fs.StringVar(&s.MachineSetupConfigsPath, "machinesetup", s.MachineSetupConfigsPath, "path to machine setup configs file")

	config.ControllerConfig.AddFlags(pflag.CommandLine)
}
