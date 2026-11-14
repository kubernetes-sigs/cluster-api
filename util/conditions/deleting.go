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
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
)

// AppendDeletingMessage appends obj's own Deleting condition message to msg, if reported.
// It accepts unstructured objects and typed objects implementing Getter.
// According to the contract, obj is not required to report conditions; msg is returned unchanged in that case.
func AppendDeletingMessage(msg string, obj client.Object) string {
	if util.IsNil(obj) {
		return msg
	}

	var deleting *metav1.Condition
	switch o := obj.(type) {
	case *unstructured.Unstructured:
		c, err := UnstructuredGet(o, clusterv1.DeletingCondition)
		if err != nil {
			return msg
		}
		deleting = c
	case Getter:
		deleting = Get(o, clusterv1.DeletingCondition)
	}

	if deleting == nil || deleting.Status != metav1.ConditionTrue || deleting.Message == "" {
		return msg
	}

	return msg + ":" + indentIfMultiline(deleting.Message)
}
