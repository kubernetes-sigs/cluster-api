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

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// DefaultingValidator interface is for objects that define both defaulting
// and validating webhooks.
type DefaultingValidator interface { //nolint:revive
	admission.Defaulter
	admission.Validator
}

// DefaultValidateTest returns a new testing function to be used in tests to
// make sure defaulting webhooks also pass validation tests on create,
// update and delete.
func DefaultValidateTest(object DefaultingValidator) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()

		createCopy := object.DeepCopyObject().(DefaultingValidator)
		updateCopy := object.DeepCopyObject().(DefaultingValidator)
		deleteCopy := object.DeepCopyObject().(DefaultingValidator)
		defaultingUpdateCopy := updateCopy.DeepCopyObject().(DefaultingValidator)

		t.Run("validate-on-create", func(t *testing.T) {
			g := NewWithT(t)
			createCopy.Default()
			warnings, err := createCopy.ValidateCreate()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
		})
		t.Run("validate-on-update", func(t *testing.T) {
			g := NewWithT(t)
			defaultingUpdateCopy.Default()
			updateCopy.Default()
			warnings, err := defaultingUpdateCopy.ValidateUpdate(updateCopy)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
		})
		t.Run("validate-on-delete", func(t *testing.T) {
			g := NewWithT(t)
			deleteCopy.Default()
			warnings, err := deleteCopy.ValidateDelete()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
		})
	}
}
