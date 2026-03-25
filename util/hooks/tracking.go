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

// Package hooks has helper functions for Runtime Hooks.
package hooks

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

// MarkAsPending adds to the object's PendingHooksAnnotation the intent to execute a hook after an operation completes.
// Usually this function is called when an operation is starting in order to track the intent to call an After<operation> hook later in the process.
func MarkAsPending(ctx context.Context, c client.Client, obj client.Object, updateResourceVersionOnObject bool, hooks ...runtimecatalog.Hook) error {
	hookNames := []string{}
	for _, hook := range hooks {
		hookNames = append(hookNames, runtimecatalog.HookName(hook))
	}

	orig := obj.DeepCopyObject().(client.Object)

	if changed := MarkObjectAsPending(obj, hooks...); !changed {
		return nil
	}

	// In some cases it is preferred to not update resourceVersion in the input object,
	// because this could lead to conflict errors e.g. when patching at the end of a reconcile loop.
	if !updateResourceVersionOnObject {
		obj = obj.DeepCopyObject().(client.Object)
	}
	if err := c.Patch(ctx, obj, client.MergeFrom(orig)); err != nil {
		return errors.Wrapf(err, "failed to mark %q hook(s) as pending", strings.Join(hookNames, ","))
	}

	return nil
}

// MarkObjectAsPending adds to the object's PendingHooksAnnotation the intent to execute a hook after an operation completes.
// Usually this function is called when an operation is starting in order to track the intent to call an After<operation> hook later in the process.
func MarkObjectAsPending(obj client.Object, hooks ...runtimecatalog.Hook) (changed bool) {
	hookNames := []string{}
	for _, hook := range hooks {
		hookNames = append(hookNames, runtimecatalog.HookName(hook))
	}

	// Read the annotation of the objects and add the hook to the comma separated list
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	newAnnotationValue := addToCommaSeparatedList(annotations[runtimev1.PendingHooksAnnotation], hookNames...)

	if annotations[runtimev1.PendingHooksAnnotation] == newAnnotationValue {
		return false
	}

	annotations[runtimev1.PendingHooksAnnotation] = newAnnotationValue
	obj.SetAnnotations(annotations)
	return true
}

// IsPending returns true if there is an intent to call a hook being tracked in the object's PendingHooksAnnotation.
func IsPending(hook runtimecatalog.Hook, obj client.Object) bool {
	hookName := runtimecatalog.HookName(hook)
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	return isInCommaSeparatedList(annotations[runtimev1.PendingHooksAnnotation], hookName)
}

// MarkAsDone removes the intent to call a Hook from the object's PendingHooksAnnotation.
// Usually this func is called after all the registered extensions for the Hook returned an answer without requests
// to hold on to the object's lifecycle (retryAfterSeconds).
func MarkAsDone(ctx context.Context, c client.Client, obj client.Object, updateResourceVersionOnObject bool, hook runtimecatalog.Hook) error {
	if !IsPending(hook, obj) {
		return nil
	}

	hookName := runtimecatalog.HookName(hook)

	orig := obj.DeepCopyObject().(client.Object)

	// Read the annotation of the objects and add the hook to the comma separated list
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[runtimev1.PendingHooksAnnotation] = removeFromCommaSeparatedList(annotations[runtimev1.PendingHooksAnnotation], hookName)
	if annotations[runtimev1.PendingHooksAnnotation] == "" {
		delete(annotations, runtimev1.PendingHooksAnnotation)
	}
	obj.SetAnnotations(annotations)

	// In some cases it is preferred to not update resourceVersion in the input object,
	// because this could lead to conflict errors e.g. when patching at the end of a reconcile loop.
	if !updateResourceVersionOnObject {
		obj = obj.DeepCopyObject().(client.Object)
	}
	if err := c.Patch(ctx, obj, client.MergeFrom(orig)); err != nil {
		return errors.Wrapf(err, "failed to mark %q hook as done", hookName)
	}

	return nil
}

// IsOkToDelete returns true if object has the OkToDeleteAnnotation in the annotations of the object, false otherwise.
func IsOkToDelete(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	if _, ok := annotations[runtimev1.OkToDeleteAnnotation]; ok {
		return true
	}
	return false
}

// MarkAsOkToDelete adds the OkToDeleteAnnotation annotation to the object and patches it.
func MarkAsOkToDelete(ctx context.Context, c client.Client, obj client.Object, updateResourceVersionOnObject bool) error {
	if _, ok := obj.GetAnnotations()[runtimev1.OkToDeleteAnnotation]; ok {
		return nil
	}

	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return errors.Wrapf(err, "failed to mark %s as ok to delete: failed to get GVK for object", klog.KObj(obj))
	}

	orig := obj.DeepCopyObject().(client.Object)

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[runtimev1.OkToDeleteAnnotation] = ""
	obj.SetAnnotations(annotations)

	// In some cases it is preferred to not update resourceVersion in the input object,
	// because this could lead to conflict errors e.g. when patching at the end of a reconcile loop.
	if !updateResourceVersionOnObject {
		obj = obj.DeepCopyObject().(client.Object)
	}
	if err := c.Patch(ctx, obj, client.MergeFrom(orig)); err != nil {
		return errors.Wrapf(err, "failed to mark %s %s as ok to delete", gvk.Kind, klog.KObj(obj))
	}

	return nil
}

func addToCommaSeparatedList(list string, items ...string) string {
	set := sets.Set[string]{}.Insert(strings.Split(list, ",")...)

	// Remove empty strings (that might have been introduced by splitting an empty list)
	// from the hook list
	set.Delete("")

	set.Insert(items...)

	return strings.Join(sets.List(set), ",")
}

func isInCommaSeparatedList(list, item string) bool {
	set := sets.Set[string]{}.Insert(strings.Split(list, ",")...)
	return set.Has(item)
}

func removeFromCommaSeparatedList(list string, items ...string) string {
	set := sets.Set[string]{}.Insert(strings.Split(list, ",")...)

	// Remove empty strings (that might have been introduced by splitting an empty list)
	// from the hook list
	set.Delete("")

	set.Delete(items...)
	return strings.Join(sets.List(set), ",")
}
