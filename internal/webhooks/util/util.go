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

// Package util includes the utility functions for testing webhooks.
package util

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// CustomDefaulterValidator interface is for objects that define both custom defaulting
// and custom validating webhooks.
type CustomDefaulterValidator interface {
	webhook.CustomDefaulter
	webhook.CustomValidator
}

// CustomDefaultValidateTest returns a new testing function to be used in tests to
// make sure custom defaulting webhooks also pass validation tests on create,
// update and delete.
func CustomDefaultValidateTest(ctx context.Context, obj runtime.Object, webhook CustomDefaulterValidator) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()

		createCopy := obj.DeepCopyObject()
		updateCopy := obj.DeepCopyObject()
		deleteCopy := obj.DeepCopyObject()
		defaultingUpdateCopy := updateCopy.DeepCopyObject()

		t.Run("validate-on-create", func(t *testing.T) {
			g := gomega.NewWithT(t)
			g.Expect(webhook.Default(ctx, createCopy)).To(gomega.Succeed())
			_, err := webhook.ValidateCreate(ctx, createCopy)
			g.Expect(err).ToNot(gomega.HaveOccurred())
		})
		t.Run("validate-on-update", func(t *testing.T) {
			g := gomega.NewWithT(t)
			g.Expect(webhook.Default(ctx, defaultingUpdateCopy)).To(gomega.Succeed())
			g.Expect(webhook.Default(ctx, updateCopy)).To(gomega.Succeed())
			_, err := webhook.ValidateUpdate(ctx, createCopy, defaultingUpdateCopy)
			g.Expect(err).ToNot(gomega.HaveOccurred())
		})
		t.Run("validate-on-delete", func(t *testing.T) {
			g := gomega.NewWithT(t)
			g.Expect(webhook.Default(ctx, deleteCopy)).To(gomega.Succeed())
			_, err := webhook.ValidateDelete(ctx, deleteCopy)
			g.Expect(err).ToNot(gomega.HaveOccurred())
		})
	}
}
