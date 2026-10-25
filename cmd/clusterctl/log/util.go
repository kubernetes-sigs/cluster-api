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

package log

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// UnstructuredToValues provide a utility function for creation values describing an Unstructured objects. e.g.
// Deployment="capd-controller-manager" Namespace="capd-system"  (<Kind>=<name> Namespace=<Namespace>)
// CustomResourceDefinition="dockerclusters.infrastructure.cluster.x-k8s.io" (omit Namespace if it does not apply).
func UnstructuredToValues(obj unstructured.Unstructured) []interface{} {
	values := []interface{}{
		obj.GetKind(), obj.GetName(),
	}
	if obj.GetNamespace() != "" {
		values = append(values, "Namespace", obj.GetNamespace())
	}
	return values
}
