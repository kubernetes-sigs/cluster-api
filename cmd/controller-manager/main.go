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
	"flag"

	controllerlib "github.com/kubernetes-incubator/apiserver-builder/pkg/controller"
	"github.com/spf13/pflag"

	"github.com/golang/glog"
	"k8s.io/apiserver/pkg/util/logs"
	"sigs.k8s.io/cluster-api/pkg/controller"
	"sigs.k8s.io/cluster-api/pkg/controller/config"
)

func init() {
	config.ControllerConfig.AddFlags(pflag.CommandLine)
}

func main() {
	// the following line exists to make glog happy, for more information, see: https://github.com/kubernetes/kubernetes/issues/17162
	flag.CommandLine.Parse([]string{})

	// Map go flags to pflag
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

	logs.InitLogs()
	defer logs.FlushLogs()

	config, err := controllerlib.GetConfig(config.ControllerConfig.Kubeconfig)
	if err != nil {
		glog.Fatalf("Could not create Config for talking to the apiserver: %v", err)
	}

	controllers, _ := controller.GetAllControllers(config)
	controllerlib.StartControllerManager(controllers...)

	// Blockforever
	select {}
}
