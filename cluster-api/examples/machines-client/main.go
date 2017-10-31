package main

import (
	"flag"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	clusterapiclient "k8s.io/kube-deploy/cluster-api/client"
)

var kubeconfig = flag.String("kubeconfig", "", "path to kubeconfig file")

func main() {
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	client, err := clusterapiclient.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	machine := &machinesv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "mygcehost",
			Labels: map[string]string{"mylabel": "test"},
		},
		Spec: machinesv1.MachineSpec{
			ProviderConfig: "n1-standard-1",
		},
	}

	_, err = client.Machines().Create(machine)
	if err != nil {
		panic(err.Error())
	}
}
