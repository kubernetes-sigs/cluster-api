package main

import (
	"flag"
	"fmt"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"k8s.io/client-go/tools/clientcmd"
	clusterapiclient "k8s.io/kube-deploy/cluster-api/client"
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

	_, err = clusterapiclient.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	fmt.Println("Successfully upgraded the cluster to version: ", *kubeversion)
}

