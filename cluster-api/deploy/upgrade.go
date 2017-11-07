package deploy

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	clusapiclnt "k8s.io/kube-deploy/cluster-api/client"
)

func UpgradeCluster(kubeversion string, kubeconfig string) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}

	client, err := clusapiclnt.NewForConfig(config)
	if err != nil {
		return err
	}

	// Now continue to update all the node's state.
	machine_list, err := client.Machines().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Polling the cluster until nodes are updated.
	errors := make(chan error, len(machine_list.Items))
	for i, _ := range machine_list.Items {
		go func(mach *machinesv1.Machine) {
			mach.Spec.Versions.Kubelet = kubeversion
			for _, role := range mach.Spec.Roles {
				if role == "master" {
					mach.Spec.Versions.ControlPlane = kubeversion
				}
			}
			new_machine, err := client.Machines().Update(mach)
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
