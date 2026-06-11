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

package api

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/yaml"
)

type Conditions []map[string]any

func (d *Conditions) DeepCopy() *Conditions {
	return d // FIXME: probably make this a real deep copy sooner or later
}

func (in *Conditions) DeepCopyInto(out *Conditions) {
	*out = *in // FIXME: probably make this a real deep copy sooner or later
}

func (u *Conditions) UnmarshalJSON(b []byte) error {
	conditions := []any{}
	if err := yaml.Unmarshal(b, &conditions); err != nil {
		return err
	}
	for _, c := range conditions {
		cMap, ok := c.(map[string]any)
		if !ok {
			return fmt.Errorf("TODO error 3")
		}
		conditionType, ok := cMap["type"]
		if !ok {
			return fmt.Errorf("TODO error 2")
		}
		conditionTypeString, ok := conditionType.(string)
		if !ok {
			return fmt.Errorf("TODO error 3")
		}
		if conditionTypeString != "Ready" {
			continue
		}

		*u = append(*u, cMap)
	}
	return nil
}

func (u *Conditions) AsAnyArray() []any {
	if len(*u) == 0 {
		return nil
	}

	var conditions []any
	for _, condition := range *u {
		conditions = append(conditions, condition)
	}
	return conditions
}
