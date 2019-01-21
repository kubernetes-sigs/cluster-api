/*
Copyright 2018 The Kubernetes Authors.

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

package validation

import (
	"fmt"
	"io"

	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ValidatePods(w io.Writer, c client.Client, clusterName string) error {
	fmt.Fprintf(w, "Validating pods\n")

	return validatePods(w, c, clusterName, "kube-system")
}

func validatePods(w io.Writer, c client.Client, clusterName, namespace string) error {
	fmt.Fprintf(w, "Checking pods in namespace %s for cluster %s...", namespace, clusterName)

	pods := &corev1.PodList{}
	if err := c.List(context.TODO(), client.InNamespace(namespace), pods); err != nil {
		return fmt.Errorf("Failed to get pods in namespace %q: %v", namespace, err)
	}

	if len(pods.Items) == 0 {
		fmt.Fprintf(w, "FAIL\n")
		fmt.Fprintf(w, "\tNo pod exists in namespace %q.\n", namespace)
		return fmt.Errorf("Pods are not found in namespace %q.", namespace)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}

		if pod.Status.Phase == corev1.PodPending ||
			pod.Status.Phase == corev1.PodFailed ||
			pod.Status.Phase == corev1.PodUnknown {

			fmt.Fprintf(w, "FAIL\n")
			fmt.Fprintf(w, "\t[pod %v]: %s\n", pod.Name, pod.Status.Reason)
			return fmt.Errorf("Pod %s in namespace %s is not ready.", pod.Name, pod.Namespace)
		}

		for _, container := range pod.Status.ContainerStatuses {
			if !container.Ready {
				fmt.Fprintf(w, "FAIL\n")
				fmt.Fprintf(w, "\t[container %v in pod %v]: not ready.\n", container.Name, pod.Name)
				return fmt.Errorf("Pod %s in namespace %s has container %s which is not ready.", pod.Name, pod.Namespace, container.Name)
			}
		}
	}

	fmt.Fprintf(w, "PASS\n")
	return nil
}
