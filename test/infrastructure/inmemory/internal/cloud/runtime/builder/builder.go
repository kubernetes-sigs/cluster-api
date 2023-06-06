/*
Copyright 2023 The Kubernetes Authors.

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

package builder

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	ccontroller "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/controller"
	"sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/handler"
	cmanager "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/manager"
	cpredicate "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/predicate"
	creconciler "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/reconcile"
	csource "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/source"
)

// Builder builds a Controller.
type Builder struct {
	forInput         ForInput
	watchesInput     []WatchesInput
	mgr              cmanager.Manager
	globalPredicates []cpredicate.Predicate
	ctrl             ccontroller.Controller
	ctrlOptions      ccontroller.Options
	name             string
}

// ControllerManagedBy returns a new controller builder that will be started by the provided Manager.
func ControllerManagedBy(m cmanager.Manager) *Builder {
	return &Builder{mgr: m}
}

// ForInput represents the information set by For method.
type ForInput struct {
	object     client.Object
	predicates []cpredicate.Predicate
	err        error
}

// For defines the type of Object being *reconciled*, and configures the ControllerManagedBy to respond to create / delete /
// update events by *reconciling the object*.
func (blder *Builder) For(object client.Object, opts ...ForOption) *Builder {
	if blder.forInput.object != nil {
		blder.forInput.err = fmt.Errorf("method For(...) should only be called once, could not assign multiple objects for reconciliation")
		return blder
	}
	input := ForInput{object: object}
	for _, opt := range opts {
		opt.ApplyToFor(&input)
	}

	blder.forInput = input
	return blder
}

// WatchesInput represents the information set by Watches method.
type WatchesInput struct {
	src          csource.Source
	eventhandler handler.EventHandler
	predicates   []cpredicate.Predicate
}

// Watches defines the type of Object to watch, and configures the ControllerManagedBy to respond to create / delete /
// update events by *reconciling the object* with the given EventHandler.
func (blder *Builder) Watches(src csource.Source, eventhandler handler.EventHandler, opts ...WatchesOption) *Builder {
	input := WatchesInput{src: src, eventhandler: eventhandler}
	for _, opt := range opts {
		opt.ApplyToWatches(&input)
	}

	blder.watchesInput = append(blder.watchesInput, input)
	return blder
}

// WithEventFilter sets the event filters, to filter which create/update/delete/generic events eventually
// trigger reconciliations.  For example, filtering on whether the resource version has changed.
// Given predicate is added for all watched objects.
// Defaults to the empty list.
func (blder *Builder) WithEventFilter(p cpredicate.Predicate) *Builder {
	blder.globalPredicates = append(blder.globalPredicates, p)
	return blder
}

// WithOptions overrides the controller options use in doController. Defaults to empty.
func (blder *Builder) WithOptions(options ccontroller.Options) *Builder {
	blder.ctrlOptions = options
	return blder
}

// Named sets the name of the controller to the given name.  The name shows up
// in metrics, among other things, and thus should be a prometheus compatible name
// (underscores and alphanumeric characters only).
//
// By default, controllers are named using the lowercase version of their kind.
func (blder *Builder) Named(name string) *Builder {
	blder.name = name
	return blder
}

// Complete builds the Application Controller.
func (blder *Builder) Complete(r creconciler.Reconciler) error {
	_, err := blder.Build(r)
	return err
}

// Build builds the Application Controller and returns the Controller it created.
func (blder *Builder) Build(r creconciler.Reconciler) (ccontroller.Controller, error) {
	if r == nil {
		return nil, fmt.Errorf("must provide a non-nil Reconciler")
	}
	if blder.mgr == nil {
		return nil, fmt.Errorf("must provide a non-nil Manager")
	}
	if blder.forInput.err != nil {
		return nil, blder.forInput.err
	}

	// Set the ControllerManagedBy
	if err := blder.doController(r); err != nil {
		return nil, err
	}

	// Set the Watch
	if err := blder.doWatch(); err != nil {
		return nil, err
	}

	return blder.ctrl, nil
}

func (blder *Builder) doWatch() error {
	// Reconcile type
	if blder.forInput.object != nil {
		i, err := blder.mgr.GetCache().GetInformer(context.TODO(), blder.forInput.object)
		if err != nil {
			return err
		}

		src := &csource.Informer{Informer: i}
		hdler := &handler.EnqueueRequestForObject{}
		allPredicates := append(blder.globalPredicates, blder.forInput.predicates...)
		if err := blder.ctrl.Watch(src, hdler, allPredicates...); err != nil {
			return err
		}
	}

	// Do the watch requests
	if len(blder.watchesInput) == 0 && blder.forInput.object == nil {
		return errors.New("there are no watches configured, controller will never get triggered. Use For(), Owns() or Watches() to set them up")
	}
	for _, w := range blder.watchesInput {
		allPredicates := append([]cpredicate.Predicate(nil), blder.globalPredicates...)
		allPredicates = append(allPredicates, w.predicates...)

		if err := blder.ctrl.Watch(w.src, w.eventhandler, allPredicates...); err != nil {
			return err
		}
	}
	return nil
}

func (blder *Builder) getControllerName(gvk schema.GroupVersionKind, hasGVK bool) (string, error) {
	if blder.name != "" {
		return blder.name, nil
	}
	if !hasGVK {
		return "", errors.New("one of For() or Named() must be called")
	}
	return strings.ToLower(gvk.Kind), nil
}

func (blder *Builder) doController(r creconciler.Reconciler) error {
	ctrlOptions := blder.ctrlOptions
	if ctrlOptions.Reconciler == nil {
		ctrlOptions.Reconciler = r
	}

	var gvk schema.GroupVersionKind
	hasGVK := blder.forInput.object != nil
	if hasGVK {
		var err error
		gvk, err = apiutil.GVKForObject(blder.forInput.object, blder.mgr.GetScheme())
		if err != nil {
			return err
		}
	}

	// Setup concurrency.
	if ctrlOptions.Concurrency <= 0 {
		ctrlOptions.Concurrency = 1
	}

	controllerName, err := blder.getControllerName(gvk, hasGVK)
	if err != nil {
		return err
	}

	// Build the controller.
	blder.ctrl, err = ccontroller.New(controllerName, ctrlOptions)
	if err != nil {
		return err
	}

	// Add the controller to the Manager.
	if err := blder.mgr.AddController(blder.ctrl); err != nil {
		return err
	}
	return err
}
