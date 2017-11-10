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

package deploy

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	clusapiclnt "k8s.io/kube-deploy/cluster-api/client"
	"fmt"
	"k8s.io/kube-deploy/cluster-api/util"
)

func UpgradeCluster(kubeversion string, kubeconfig string) error {
	if kubeconfig == "" {
		kubeconfig = util.GetDefaultKubeConfigPath()
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}

	client, err := clusapiclnt.NewForConfig(config)
	if err != nil {
		return err
	}

	machine_list, err := client.Machines().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Update the control plan first. It assumes single master.
	master_found := false
	for _, mach := range machine_list.Items {
		for _, role := range mach.Spec.Roles {
			if role == "master" {
				mach.Spec.Versions.Kubelet = kubeversion
				mach.Spec.Versions.ControlPlane = kubeversion
				new_machine, err := client.Machines().Update(&mach)
				if err == nil {
					err = wait.Poll(5*time.Second, 10*time.Minute, func() (bool, error) {
						new_machine, err = client.Machines().Get(new_machine.Name, metav1.GetOptions{})
						//if err == nil && new_machine.Status.Ready {
						//	return true, err
						//}
						//return false, err
						if err != nil {
							return false, err
						}
						return true, nil
					})
				}
				master_found = true
				break
			}
		}
	}

	if err == nil && !master_found {
		err = fmt.Errorf("No master is found.")
	}
	if err != nil {
		return err
	}

	// Continue to update all the nodes.
	errors := make(chan error, len(machine_list.Items))
	for i, _ := range machine_list.Items {
		go func(mach *clusterv1.Machine) {
			mach.Spec.Versions.Kubelet = kubeversion
			new_machine, err := client.Machines().Update(mach)
			if err == nil {
				// Polling the cluster until nodes are updated.
				err = wait.Poll(5*time.Second, 10*time.Minute, func() (bool, error) {
					new_machine, err = client.Machines().Get(new_machine.Name, metav1.GetOptions{})
					//if err == nil && new_machine.Status.Ready {
					//	return true, err
					//}
					//return false, err
					if err != nil {
						return false, err
					}
					return true, nil
				})
			}
			errors <- err
		}(&machine_list.Items[i])
	}

	for i := 0; i < len(machine_list.Items); i++ {
		if err = <-errors; err != nil {
			return err
		}
	}
	return nil
}
