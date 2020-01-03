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

package conversion

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
)

const (
	DataAnnotation = "cluster.x-k8s.io/conversion-data"
)

// MarshalData stores the source object as json data in the destination object annotations map.
func MarshalData(src metav1.Object, dst metav1.Object) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	if dst.GetAnnotations() == nil {
		dst.SetAnnotations(map[string]string{})
	}
	dst.GetAnnotations()[DataAnnotation] = string(data)
	return nil
}

// UnmarshalData tries to retrieve the data from the annotation and unmarshals it into the object passed as input.
func UnmarshalData(from metav1.Object, to interface{}) (bool, error) {
	data, ok := from.GetAnnotations()[DataAnnotation]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal([]byte(data), to); err != nil {
		return false, err
	}
	delete(from.GetAnnotations(), DataAnnotation)
	return true, nil
}
