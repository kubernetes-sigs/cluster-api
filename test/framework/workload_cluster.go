/*
Copyright 2019 The Kubernetes Authors.

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

package framework

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// podCondition is a type that operates a condition on a Pod
type podCondition func(p *corev1.Pod) error

// WaitForPodConditionInput is the input args for WaitForPodCondition
type WaitForPodConditionInput struct {
	Lister      Lister
	ListOptions []client.ListOption
	Condition   func(p *corev1.Pod) error
}

// WaitForPodCondition waits for the specified condition to be true for all
// pods returned from the list filter.
func WaitForPodCondition(ctx context.Context, input WaitForPodConditionInput, intervals ...interface{}) {
	By("waiting for pod condition")
	Eventually(func() (bool, error) {
		podList := &corev1.PodList{}
		if err := input.Lister.List(
			ctx,
			podList,
			input.ListOptions...,
		); err != nil {
			return false, err
		}
		satisfied := 0
		for _, pod := range podList.Items {
			if input.Condition(&pod) == nil {
				satisfied++
			}
		}
		// all pods in the list should satisfy the condition
		return satisfied == len(podList.Items), nil
	}, intervals...).Should(BeTrue())
}

// EtcdImageTagCondition returns a podCondition that ensures the pod image
// contains the specified image tag
func EtcdImageTagCondition(expectedTag string) podCondition {
	return func(p *corev1.Pod) error {
		// TODO: veriy container first
		if !strings.Contains(p.Spec.Containers[0].Image, expectedTag) {
			return errors.Errorf("pod %s/%s image does not contain the expectedTag %s", p.Namespace, p.Name, expectedTag)
		}
		return nil
	}
}
