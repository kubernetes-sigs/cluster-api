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

package external

import (
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/cluster-api/util/predicates"
)

// ObjectTracker is a helper struct to deal when watching external unstructured objects.
type ObjectTracker struct {
	m sync.Map

	Controller controller.Controller
	Cache      cache.Cache
}

// Watch uses the controller to issue a Watch only if the object hasn't been seen before.
func (o *ObjectTracker) Watch(log logr.Logger, obj client.Object, handler handler.EventHandler, p ...predicate.Predicate) error {
	// Consider this a no-op if the controller isn't present.
	if o.Controller == nil {
		return nil
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	key := gvk.GroupKind().String()
	if _, loaded := o.m.LoadOrStore(key, struct{}{}); loaded {
		return nil
	}

	log.Info(fmt.Sprintf("Adding watch on external object %q", gvk.String()))
	err := o.Controller.Watch(source.Kind(
		o.Cache,
		obj.DeepCopyObject().(client.Object),
		handler,
		append(p, predicates.ResourceNotPaused(log))...,
	))
	if err != nil {
		o.m.Delete(key)
		return errors.Wrapf(err, "failed to add watch on external object %q", gvk.String())
	}
	return nil
}
