/*
Copyright 2021 The Kubernetes Authors.

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

package defaulting

import (
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

// DefaultingValidator interface is for objects that define both defaulting
// and validating webhooks.
type DefaultingValidator interface {
	runtime.Object
	Default()
	ValidateCreate() error
	ValidateUpdate(old runtime.Object) error
	ValidateDelete() error
}

// DefaultValidateTest returns a new testing function to be used in tests to
// make sure defaulting webhooks also pass validation tests on create,
// update and delete.
func DefaultValidateTest(object DefaultingValidator) func(*testing.T) {
	return func(t *testing.T) {
		createCopy := object.DeepCopyObject().(DefaultingValidator)
		updateCopy := object.DeepCopyObject().(DefaultingValidator)
		deleteCopy := object.DeepCopyObject().(DefaultingValidator)
		defaultingUpdateCopy := updateCopy.DeepCopyObject().(DefaultingValidator)

		t.Run("validate-on-create", func(t *testing.T) {
			g := gomega.NewWithT(t)
			createCopy.Default()
			g.Expect(createCopy.ValidateCreate()).To(gomega.Succeed())
		})
		t.Run("validate-on-update", func(t *testing.T) {
			g := gomega.NewWithT(t)
			defaultingUpdateCopy.Default()
			g.Expect(defaultingUpdateCopy.ValidateUpdate(updateCopy)).To(gomega.Succeed())
		})
		t.Run("validate-on-delete", func(t *testing.T) {
			g := gomega.NewWithT(t)
			deleteCopy.Default()
			g.Expect(deleteCopy.ValidateDelete()).To(gomega.Succeed())
		})
	}
}
