/*
Copyright 2020 The Kubernetes Authors.

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

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodListCondition is a type that operates a condition on a Pod.
type PodListCondition func(p *corev1.PodList) error

// WaitForPodListConditionInput is the input args for WaitForPodListCondition.
type WaitForPodListConditionInput struct {
	Lister      Lister
	ListOptions *client.ListOptions
	Condition   PodListCondition
}

// WaitForPodListCondition waits for the specified condition to be true for all
// pods returned from the list filter.
func WaitForPodListCondition(ctx context.Context, input WaitForPodListConditionInput, intervals ...interface{}) {
	Eventually(func() (bool, error) {
		podList := &corev1.PodList{}
		if err := input.Lister.List(ctx, podList, input.ListOptions); err != nil {
			return false, err
		}
		Expect(len(podList.Items)).ToNot(BeZero())

		// all pods in the list should satisfy the condition
		err := input.Condition(podList)
		if err != nil {
			return false, err
		}
		return true, nil
	}, intervals...).Should(BeTrue())
}

// EtcdImageTagCondition returns a podListCondition that ensures the pod image
// contains the specified image tag.
func EtcdImageTagCondition(expectedTag string, expectedCount int) PodListCondition {
	return func(pl *corev1.PodList) error {
		countWithCorrectTag := 0
		for _, pod := range pl.Items {
			if strings.Contains(pod.Spec.Containers[0].Image, expectedTag) {
				countWithCorrectTag++
			}
		}
		if countWithCorrectTag != expectedCount {
			return errors.Errorf("etcdImageTagCondition: expected %d pods to have image tag %q, got %d", expectedCount, expectedTag, countWithCorrectTag)
		}

		// This check is to ensure that if there are three controlplane nodes,
		// then there are only three etcd pods running. Currently, we create a
		// new etcd pod before deleting the previous one. So we can have a
		// case where there are three etcd pods with the correct tag and one
		// left over that has yet to be deleted.
		if len(pl.Items) != expectedCount {
			return errors.Errorf("etcdImageTagCondition: expected %d pods, got %d", expectedCount, len(pl.Items))
		}
		return nil
	}
}

// PhasePodCondition is a podListCondition ensuring that pods are in the expected
// pod phase.
func PhasePodCondition(expectedPhase corev1.PodPhase) PodListCondition {
	return func(pl *corev1.PodList) error {
		for _, pod := range pl.Items {
			if pod.Status.Phase != expectedPhase {
				return errors.Errorf("pod %q is not %s", pod.Name, expectedPhase)
			}
		}
		return nil
	}
}
