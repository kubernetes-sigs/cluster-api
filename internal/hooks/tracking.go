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
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
	"sigs.k8s.io/cluster-api/util/patch"
)

// MarkAsPending sets the information on the object to signify that the hook is marked.
func MarkAsPending(ctx context.Context, c client.Client, obj client.Object, hooks ...runtimecatalog.Hook) (retErr error) {
	patchHelper, err := patch.NewHelper(obj, c)
	if err != nil {
		return errors.Wrap(err, "failed to create patch helper")
	}

	// read the annotation of the objects and add the hook to the comma separated list
	hookNames := []string{}
	for _, hook := range hooks {
		hookNames = append(hookNames, runtimecatalog.HookName(hook))
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[runtimehooksv1.PendingHooksAnnotation] = addToCommaSeparatedList(annotations[runtimehooksv1.PendingHooksAnnotation], hookNames...)
	obj.SetAnnotations(annotations)

	if err := patchHelper.Patch(ctx, obj); err != nil {
		return errors.Wrap(err, "failed to apply patch")
	}

	return nil
}

// IsPending returns true if the hook is marked on the object.
func IsPending(hook runtimecatalog.Hook, obj client.Object) bool {
	hookName := runtimecatalog.HookName(hook)
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	return isInCommaSeparatedList(annotations[runtimehooksv1.PendingHooksAnnotation], hookName)
}

// MarkAsDone remove the information on the object that represents tha hook is marked.
func MarkAsDone(ctx context.Context, c client.Client, obj client.Object, hooks ...runtimecatalog.Hook) (retErr error) {
	patchHelper, err := patch.NewHelper(obj, c)
	if err != nil {
		return errors.Wrap(err, "failed to create patch helper")
	}

	// read the annotation of the objects and add the hook to the comma separated list
	hookNames := []string{}
	for _, hook := range hooks {
		hookNames = append(hookNames, runtimecatalog.HookName(hook))
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[runtimehooksv1.PendingHooksAnnotation] = removeFromCommaSeparatedList(annotations[runtimehooksv1.PendingHooksAnnotation], hookNames...)
	if annotations[runtimehooksv1.PendingHooksAnnotation] == "" {
		delete(annotations, runtimehooksv1.PendingHooksAnnotation)
	}
	obj.SetAnnotations(annotations)

	if err := patchHelper.Patch(ctx, obj); err != nil {
		return errors.Wrap(err, "failed to apply patch")
	}

	return nil
}

func addToCommaSeparatedList(list string, items ...string) string {
	set := sets.NewString(strings.Split(list, ",")...)
	set.Insert(items...)
	return strings.Join(set.List(), ",")
}

func isInCommaSeparatedList(list, item string) bool {
	set := sets.NewString(strings.Split(list, ",")...)
	return set.Has(item)
}

func removeFromCommaSeparatedList(list string, items ...string) string {
	set := sets.NewString(strings.Split(list, ",")...)
	set.Delete(items...)
	return strings.Join(set.List(), ",")
}
