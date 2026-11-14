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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// AppendDeletingMessage appends obj's own Deleting condition message to msg, if reported.
// According to the contract, obj is not required to report conditions; msg is returned unchanged in that case.
func AppendDeletingMessage(msg string, obj *unstructured.Unstructured) string {
	if obj == nil {
		return msg
	}

	deleting, err := UnstructuredGet(obj, clusterv1.DeletingCondition)
	if err != nil || deleting == nil || deleting.Status != metav1.ConditionTrue || deleting.Message == "" {
		return msg
	}

	return msg + ":" + indentIfMultiline(deleting.Message)
}
