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

	clusterapis "github.com/openshift/cluster-api/pkg/apis"
	"github.com/openshift/cluster-api/pkg/apis/cluster/common"
	"github.com/openshift/cluster-api/pkg/client/clientset_generated/clientset"
	capicluster "github.com/openshift/cluster-api/pkg/controller/cluster"
	capimachine "github.com/openshift/cluster-api/pkg/controller/machine"
	"github.com/openshift/cluster-api/pkg/provider/example/actuators/cluster"
	"github.com/openshift/cluster-api/pkg/provider/example/actuators/machine"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	cfg := config.GetConfigOrDie()

	// Setup a Manager
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		klog.Fatalf("Failed to set up controller manager: %v", err)
	}

	cs, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Failed to create client from configuration: %v", err)
	}

	recorder := mgr.GetEventRecorderFor("clusterapi-controller")

	// Initialize cluster actuator.
	clusterActuator, _ := cluster.NewClusterActuator(cs.ClusterV1alpha1(), recorder)

	// Initialize machine actuator.
	machineActuator, _ := machine.NewMachineActuator(cs.ClusterV1alpha1(), recorder)

	// Register cluster deployer
	common.RegisterClusterProvisioner("example", clusterActuator)

	if err := clusterapis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatal(err)
	}

	capimachine.AddWithActuator(mgr, machineActuator)
	capicluster.AddWithActuator(mgr, clusterActuator)

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		klog.Fatalf("Failed to run manager: %v", err)
	}
}
