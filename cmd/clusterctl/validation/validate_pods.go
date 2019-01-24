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
	"strings"

	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type validationError struct {
	name    string
	message string
}

func ValidatePods(w io.Writer, c client.Client, namespace string) error {
	fmt.Fprintf(w, "Validating pods in namespace %q\n", namespace)

	pods, err := getPods(c, namespace)
	if err != nil {
		return err
	}
	return validatePods(w, pods, namespace)
}

func getPods(c client.Client, namespace string) (*corev1.PodList, error) {
	pods := &corev1.PodList{}
	if err := c.List(context.TODO(), client.InNamespace(namespace), pods); err != nil {
		return nil, fmt.Errorf("Failed to get pods in namespace %q: %v", namespace, err)
	}
	return pods, nil
}

func validatePods(w io.Writer, pods *corev1.PodList, namespace string) error {
	if len(pods.Items) == 0 {
		fmt.Fprintf(w, "FAIL\n")
		fmt.Fprintf(w, "\tpods in namespace %q not exist.\n", namespace)
		return fmt.Errorf("Pods in namespace %q not exist.", namespace)
	}

	var failures []*validationError
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}

		if pod.Status.Phase == corev1.PodPending ||
			pod.Status.Phase == corev1.PodFailed ||
			pod.Status.Phase == corev1.PodUnknown {
			failures = append(failures, &validationError{
				name:    fmt.Sprintf("%q/%q", pod.Namespace, pod.Name),
				message: fmt.Sprintf("Pod %q in namespace %q is %s.", pod.Name, pod.Namespace, pod.Status.Phase),
			})
			continue
		}

		var notready []string
		for _, container := range pod.Status.ContainerStatuses {
			if !container.Ready {
				notready = append(notready, container.Name)
			}
		}
		if len(notready) != 0 {
			failures = append(failures, &validationError{
				name:    fmt.Sprintf("%q/%q", pod.Namespace, pod.Name),
				message: fmt.Sprintf("Pod %q in namespace %q is not ready (%s).", pod.Name, pod.Namespace, strings.Join(notready, ",")),
			})
		}
	}

	if len(failures) != 0 {
		fmt.Fprintf(w, "FAIL\n")
		for _, failure := range failures {
			fmt.Fprintf(w, "\t[%v]: %s\n", failure.name, failure.message)
		}
		return fmt.Errorf("Pod failures in namespace %q found.", namespace)
	}

	fmt.Fprintf(w, "PASS\n")
	return nil
}
