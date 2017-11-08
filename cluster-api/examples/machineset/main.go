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

package main

import (
	"flag"
	"fmt"
	"os/user"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	clusterapiclient "k8s.io/kube-deploy/cluster-api/client"
)

var kubeconfig = flag.String("kubeconfig", "", "path to kubeconfig file")
var label = flag.String("label", "", "label selector to define MachineSet")
var replicas = flag.Int("replicas", -1, "number of replicas to scale to")

// This is an example client-side implementation of MachineSets. You can
// specify a label selector to define the set, and a number of replicas to
// scale to. If there are fewer Machines that match the label selector than
// --replicas, it will create more Machines by cloning entries in the set. If
// there are more Machines than --replicas, it will randomly delete Machines
// down to the correct number.
//
//   $ machineset --label role=master --replicas 3
//   $ machineset --label role=node   --replicas 10
//
// At least one Machine must exist that matches the label selector.

func main() {
	flag.Parse()

	if *kubeconfig == "" {
		user, err := user.Current()
		if err != nil {
			panic(err.Error())
		}
		*kubeconfig = user.HomeDir + "/.kube/config"
	}

	if *label == "" {
		panic("--label cannot be empty")
	}

	if *replicas < 0 {
		panic("--replicas must be >= 0")
	}

	machines, err := machinesClient()
	if err != nil {
		panic(err.Error())
	}

	if err := scale(machines, *label, *replicas); err != nil {
		panic(err.Error())
	}
}

func machinesClient() (clusterapiclient.MachinesInterface, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return nil, err
	}

	client, err := clusterapiclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return client.Machines(), nil
}

func scale(machines clusterapiclient.MachinesInterface, labelSelector string, replicas int) error {
	list, err := machines.List(metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return err
	}

	if len(list.Items) == 0 {
		return fmt.Errorf("could not find existing machines with label %s", labelSelector)
	}

	if len(list.Items) == replicas {
		fmt.Printf("Already have %d machines. Nothing to do.\n", replicas)
		return nil
	}

	if len(list.Items) > replicas {
		numToDelete := len(list.Items) - replicas
		fmt.Printf("Scaling down from %d to %d; deleting %d machines.\n", len(list.Items), replicas, numToDelete)
		for _, machine := range list.Items[0:numToDelete] {
			if err := machines.Delete(machine.ObjectMeta.Name, nil); err != nil {
				return err
			} else {
				fmt.Printf("  %s\n", machine.ObjectMeta.Name)
			}
		}

		return nil
	}

	if len(list.Items) < replicas {
		numToCreate := replicas - len(list.Items)
		fmt.Printf("Scaling up from %d to %d; creating %d machines.\n", len(list.Items), replicas, numToCreate)

		newMachine := clone(list.Items[0])

		for i := 0; i < numToCreate; i++ {
			created, err := machines.Create(newMachine)
			if err != nil {
				return err
			} else {
				fmt.Printf("  %s\n", created.ObjectMeta.Name)
			}
		}

		return nil
	}

	return nil
}

func clone(old clusterv1.Machine) *clusterv1.Machine {
	// Make sure we get the full Spec
	newMachine := old.DeepCopy()

	// but sanitize the metadata so we only use meaningful fields.
	// TODO: set GenerateName ourselves if the target object doesn't have one.
	newMachine.ObjectMeta = metav1.ObjectMeta{}
	newMachine.ObjectMeta.GenerateName = old.ObjectMeta.GenerateName
	newMachine.ObjectMeta.Labels = old.ObjectMeta.Labels
	newMachine.ObjectMeta.Annotations = old.ObjectMeta.Annotations
	newMachine.ObjectMeta.ClusterName = old.ObjectMeta.ClusterName

	// Completely wipe out the status as well
	newMachine.Status = clusterv1.MachineStatus{}
	return newMachine
}
