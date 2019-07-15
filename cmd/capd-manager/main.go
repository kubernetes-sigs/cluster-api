/*
Copyright 2019 The Kubernetes Authors.

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
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"sigs.k8s.io/cluster-api-provider-docker/actuators"
	"sigs.k8s.io/cluster-api-provider-docker/logger"
	"sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	capicluster "sigs.k8s.io/cluster-api/pkg/controller/cluster"
	capimachine "sigs.k8s.io/cluster-api/pkg/controller/machine"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func main() {
	klog.InitFlags(flag.CommandLine)
	flag.Parse()

	cfg, err := config.GetConfig()
	if err != nil {
		panic(err)
	}

	// Setup a Manager
	syncPeriod := 10 * time.Minute
	opts := manager.Options{
		SyncPeriod: &syncPeriod,
	}

	mgr, err := manager.New(cfg, opts)
	if err != nil {
		panic(err)
	}
	k8sclientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	cs, err := clientset.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	clusterLogger := logger.Log{}
	clusterLogger.Logger = klogr.New().WithName("[cluster-actuator]")
	clusterActuator := actuators.Cluster{
		Log: clusterLogger,
	}

	machineLogger := logger.Log{}
	machineLogger.Logger = klogr.New().WithName("[machine-actuator]")

	machineActuator := actuators.Machine{
		Core:       k8sclientset.CoreV1(),
		ClusterAPI: cs.ClusterV1alpha1(),
		Log:        machineLogger,
	}

	// Register our cluster deployer (the interface is in clusterctl and we define the Deployer interface on the actuator)
	common.RegisterClusterProvisioner("docker", clusterActuator)
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		panic(err)
	}

	if err := capimachine.AddWithActuator(mgr, &machineActuator); err != nil {
		panic(err)
	}
	if err := capicluster.AddWithActuator(mgr, &clusterActuator); err != nil {
		panic(err)
	}

	klogr.New().Info("Starting the controller")

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		panic(err)
	}
}
