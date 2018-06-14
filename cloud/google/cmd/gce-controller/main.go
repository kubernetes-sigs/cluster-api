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

package main

import (
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/util/logs"
	"sigs.k8s.io/cluster-api/cloud/google/cmd/gce-controller/cluster-controller-app"
	clusteroptions "sigs.k8s.io/cluster-api/cloud/google/cmd/gce-controller/cluster-controller-app/options"
	"sigs.k8s.io/cluster-api/cloud/google/cmd/gce-controller/machine-controller-app"
	machineoptions "sigs.k8s.io/cluster-api/cloud/google/cmd/gce-controller/machine-controller-app/options"
	"sigs.k8s.io/cluster-api/pkg/controller/config"
	"flag"
)

func main() {

	fs := pflag.CommandLine
	var controllerType, machineSetupConfigsPath string
	fs.StringVar(&controllerType, "controller", controllerType, "specify whether this should run the machine or cluster controller")
	fs.StringVar(&machineSetupConfigsPath, "machinesetup", machineSetupConfigsPath, "path to machine setup configs file")
	config.ControllerConfig.AddFlags(pflag.CommandLine)
	// the following line exists to make glog happy, for more information, see: https://github.com/kubernetes/kubernetes/issues/17162
	flag.CommandLine.Parse([]string{})
	pflag.Parse()

	logs.InitLogs()
	defer logs.FlushLogs()

	if controllerType == "machine" {
		machineServer := machineoptions.NewMachineControllerServer(machineSetupConfigsPath)
		if err := machine_controller_app.RunMachineController(machineServer); err != nil {
			glog.Errorf("Failed to start machine controller. Err: %v", err)
		}
	} else if controllerType == "cluster" {
		clusterServer := clusteroptions.NewClusterControllerServer()
		if err := cluster_controller_app.RunClusterController(clusterServer); err != nil {
			glog.Errorf("Failed to start cluster controller. Err: %v", err)
		}
	} else {
		glog.Errorf("Failed to start controller, `controller` flag must be either `machine` or `cluster` but was %v.", controllerType)
	}
}
