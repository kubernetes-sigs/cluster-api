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

func ValidatePods(w io.Writer, c client.Client) error {
	fmt.Fprintf(w, "Validating pods\n")

	pods, err := getPods(c, "kube-system")
	if err != nil {
		return err
	}
	return validatePods(w, pods)
}

func getPods(c client.Client, namespace string) (*corev1.PodList, error) {
	pods := &corev1.PodList{}
	if err := c.List(context.TODO(), client.InNamespace(namespace), pods); err != nil {
		return nil, fmt.Errorf("Failed to get pods in namespace %q: %v", namespace, err)
	}

	return pods, nil
}

func validatePods(w io.Writer, pods *corev1.PodList) error {
	for _, pod := range pods.Items {
		fmt.Fprintf(w, "Checking pod %q in namespace %q...", pod.Name, pod.Namespace)

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

	if len(pods.Items) == 0 {
		fmt.Fprintf(w, "FAIL\n")
		fmt.Fprintf(w, "\tpods not exist.\n")
		return fmt.Errorf("Pods not exist.")
	}

	fmt.Fprintf(w, "PASS\n")
	return nil
}
