// Copyright 2019 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"time"

	"gitlab.com/chuckh/cluster-api-provider-kind/capkactuators"
	"k8s.io/klog"
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
	cs, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Failed to create client from configuration: %v", err)
	}

	clusterActuator := capkactuators.NewClusterActuator()
	machineActuator := capkactuators.NewMachineActuator("/kubeconfigs", cs.ClusterV1alpha1())

	// Register our cluster deployer (the interface is in clusterctl and we define the Deployer interface on the actuator)
	common.RegisterClusterProvisioner("aws", clusterActuator)
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		panic(err)
	}

	if err := capimachine.AddWithActuator(mgr, machineActuator); err != nil {
		panic(err)
	}
	if err := capicluster.AddWithActuator(mgr, clusterActuator); err != nil {
		panic(err)
	}
	fmt.Println("starting the controller...!")

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		panic(err)
	}
}
