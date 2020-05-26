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
package conditions

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
)

// UnstructuredGetter return a Getter object that can read conditions from an Unstructured object.
// Important. This method should be used only with types implementing Cluster API conditions.
func UnstructuredGetter(u *unstructured.Unstructured) Getter {
	return &unstructuredWrapper{Unstructured: u}
}

type unstructuredWrapper struct {
	*unstructured.Unstructured
}

// GetConditions returns the list of conditions for an Unstructured object.
//
// NOTE: Due to the constraints of JSON-unmarshal, this operation is to be considered best effort.
// In more details:
// - Errors during JSON-unmarshal are ignored and a empty collection list is returned.
// - It's not possible to detect if the object has an empty condition list or if it does not implement conditions;
//   in both cases the operation returns an empty slice is returned.
// - If the object doesn't implement conditions on under status as defined in Cluster API,
//   JSON-unmarshal matches incoming object keys to the keys; this can lead to to conditions values partially set.
func (c *unstructuredWrapper) GetConditions() clusterv1.Conditions {
	conditions := clusterv1.Conditions{}
	if err := util.UnstructuredUnmarshalField(c.Unstructured, &conditions, "status", "conditions"); err != nil {
		return nil
	}
	return conditions
}
