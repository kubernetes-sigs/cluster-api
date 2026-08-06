/*
Copyright 2026 The Kubernetes Authors.

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

package conditions

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// WaitingForDeletionMessage computes the message for the Deleting condition of an object while
// it is waiting for one of its child objects (e.g. ControlPlane, InfraCluster, InfraMachine,
// BootstrapConfig) to be deleted.
// If the child object surfaces a Deleting condition with a message, the message is appended to
// provide more details about why deletion is not completed yet; otherwise the base message is
// returned unchanged (according to the contract child objects are not required to report conditions).
func WaitingForDeletionMessage(kind string, child *unstructured.Unstructured) string {
	baseMessage := fmt.Sprintf("Waiting for %s to be deleted", kind)
	if child == nil {
		return baseMessage
	}

	childDeleting, err := UnstructuredGet(child, clusterv1.DeletingCondition)
	if err != nil || childDeleting == nil || childDeleting.Status != metav1.ConditionTrue || childDeleting.Message == "" {
		return baseMessage
	}

	return baseMessage + ":" + indentIfMultiline(childDeleting.Message)
}
