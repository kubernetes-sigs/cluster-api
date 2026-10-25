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

package e2e

import (
	"fmt"

	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type controllerMatch struct {
	kind  string
	owner metav1.Object
}

func (m *controllerMatch) Match(actual interface{}) (success bool, err error) {
	actualMeta, err := meta.Accessor(actual)
	if err != nil {
		return false, fmt.Errorf("unable to read meta for %T: %w", actual, err)
	}

	owner := metav1.GetControllerOf(actualMeta)
	if owner == nil {
		return false, fmt.Errorf("no controller found (owner ref with controller = true) for object %#v", actual)
	}

	match := owner.Kind == m.kind &&
		owner.Name == m.owner.GetName() && owner.UID == m.owner.GetUID()

	return match, nil
}

func (m *controllerMatch) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected\n\t%#v to have a controller reference pointing to %s/%s (%v)", actual, m.kind, m.owner.GetName(), m.owner.GetUID())
}

func (m *controllerMatch) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected\n\t%#v to not have a controller reference pointing to %s/%s (%v)", actual, m.kind, m.owner.GetName(), m.owner.GetUID())
}

func HaveControllerRef(kind string, owner metav1.Object) types.GomegaMatcher {
	return &controllerMatch{kind, owner}
}
