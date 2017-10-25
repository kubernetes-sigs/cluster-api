package main

import (
	"flag"
	"fmt"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
)

// Stand-alone tool for creating the Machines CRD on a cluster. This isn't very
// useful on its own, but was meant to demo the CreateMachinesCRD function that
// we should start using in the installer.

var kubeconfig = flag.String("kubeconfig", "", "path to kubeconfig file")

func main() {
	flag.Parse()

	cs, err := clientset()
	if err != nil {
		panic(err.Error())
	}

	_, err = machinesv1.CreateMachinesCRD(cs)
	if err != nil {
		fmt.Printf("Error creating Machines CRD: %v\n", err)
	} else {
		fmt.Printf("Machines CRD created successfully!\n")
	}
}

func clientset() (*apiextensionsclient.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return nil, err
	}

	clientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}
