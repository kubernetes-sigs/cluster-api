package validation

import (
	"fmt"
	"io"

	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ValidateKubeClusterObjects(w io.Writer, c client.Client, clusterName string) error {
	fmt.Fprintf(w, "Validating Kube Cluster objects\n")

	return validatePods(w, c, clusterName)
}

func validatePods(w io.Writer, c client.Client, clusterName string) error {
	fmt.Fprintf(w, "Checking pods in cluster %s...", clusterName)

	pods := &corev1.PodList{}
	if err := c.List(context.TODO(), client.InNamespace("kube-system"), pods); err != nil {
		return fmt.Errorf("The pods in namespace kube-system are not found: %v", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		if pod.Status.Phase == corev1.PodPending {
			fmt.Fprintf(w, "FAIL\n")
			fmt.Fprintf(w, "\t[%v]: %s\n", pod.Name, pod.Status.Reason)
			return fmt.Errorf("Pod %s in namespace %s is not ready.", pod.Name, pod.Namespace)
		}
		for _, container := range pod.Status.ContainerStatuses {
			if !container.Ready {
				fmt.Fprintf(w, "FAIL\n")
				fmt.Fprintf(w, "\tContainer %s in pod %s is not ready.\n", container.Name, pod.Name)
				return fmt.Errorf("Pod %s in namespace %s has container %s which is not ready.", pod.Name, pod.Namespace, container.Name)
			}
		}
	}
	fmt.Fprintf(w, "PASS\n")
	return nil
}
