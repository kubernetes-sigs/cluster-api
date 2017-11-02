package main

import (
	"flag"
	"fmt"
	"time"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clusapiclnt "k8s.io/kube-deploy/cluster-api/client"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	kubeconfig = flag.String("kubeconfig", "", "path to kubeconfig file")
	kubeversion = flag.String("version", "", "The kubernetes version to be upgraded to")
)

func main() {
	flag.Parse()

	if *kubeversion == "" {
		panic("You need to specify kubernetes version being upgraded to.")
	}

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	client, err := clusapiclnt.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// Fetch Cluster object.
	clusters, err := client.Clusters().List(metav1.ListOptions{})
	if len(clusters.Items) != 1 {
		panic("There is more than one cluster returned!")
	}

	// Modify the control plane version
	clusters.Items[0].Spec.KubernetesVersion.Version = *kubeversion

	// Update the cluster.
	cluster, err := client.Clusters().Update(&clusters.Items[0])

	// Polling the cluster until new control plan is ready.
	err = wait.Poll(5*time.Second, 10*time.Minute, func() (bool, error) {
		cluster, err = client.Clusters().Get(cluster.Name, metav1.GetOptions{})
		if err == nil && cluster.Status.Ready {
			return true, err
		}
		return false, err
	})

	fmt.Println("Successfully upgraded control plan.")

	// Now continue to update all the node's state.
	machine_list, err := client.Machines().List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	// Polling the cluster until nodes are updated.
	for _, machine := range machine_list.Items {
		machine.Spec.Versions.Kubelet = *kubeversion
		new_machine, err := client.Machines().Update(&machine)
		if err != nil {
			panic(err.Error())
		}
		err = wait.Poll(5*time.Second, 10*time.Minute, func() (bool, error) {
			new_machine, err = client.Machines().Get(new_machine.Name, metav1.GetOptions{})
			if err == nil && new_machine.Status.Ready {
				return true, err
			}
			return false, err
		})
		if err != nil {
			panic(err.Error())
		}
	}

	fmt.Println("Successfully upgraded the cluster to version: ", *kubeversion)
}
