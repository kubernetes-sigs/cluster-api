/*
Copyright 2022 The Kubernetes Authors.

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

package controllers

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=runtime.cluster.x-k8s.io,resources=extensionconfigs,verbs=get;list;watch;create;update;patch;delete

// Reconciler reconciles an Extension object.
type Reconciler struct {
	Client        client.Client
	APIReader     client.Reader
	RuntimeClient runtimeclient.Client
	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&runtimev1.ExtensionConfig{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var errs []error
	// TODO: check that this doesn't happen multiple times at startup due to concurrent reconciliation of existing Extensions.
	// Add sync.Mutex to warmupRegistry
	if !r.RuntimeClient.IsReady() {
		err := r.warmupRegistry(ctx)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "extensions controller not initialized")
		}
		// After the reconciler is warmed up requeue the request straight away.
		// This is important if e.g. the task is delete. Alternatively we could go directly into reconcile here.
		return ctrl.Result{Requeue: true}, nil
	}

	extension := &runtimev1.ExtensionConfig{}
	err := r.Client.Get(ctx, req.NamespacedName, extension)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Extension not found. Remove from registry.
			return r.reconcileDelete(ctx, extension)
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Handle deletion reconciliation loop.
	if !extension.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, extension)
	}

	// Make a copy of the extension to use as a base for patching.
	original := extension.DeepCopy()

	// discover will return a discovered Extension with the appropriate conditions.
	if extension, err = r.discoverExtension(ctx, extension); err != nil {
		errs = append(errs, err)
	}

	// Always patch the Extension as it may contain updates in conditions.
	if err = r.patchExtension(ctx, original, extension); err != nil {
		errs = append(errs, err)
	}

	if len(errs) != 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}

	// Register the extension if it was found and patched without error.
	if err = r.RuntimeClient.Extension(extension).Register(); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to register extension")
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) patchExtension(ctx context.Context, original *runtimev1.ExtensionConfig, ext *runtimev1.ExtensionConfig, options ...patch.Option) error {
	patchHelper, err := patch.NewHelper(original, r.Client)

	options = append(options, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
		runtimev1.RuntimeExtensionDiscovered,
	}})
	if err != nil {
		return err
	}
	return patchHelper.Patch(ctx, ext, options...)
}

func (r *Reconciler) reconcileDelete(_ context.Context, extension *runtimev1.ExtensionConfig) (ctrl.Result, error) {
	if err := r.RuntimeClient.Extension(extension).Unregister(); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// warmupRegistry attempts to discover all existing Extensions and patch their Status with discovered RuntimeExtensions.
// it warms up the registry by passing it the updated list of Extensions.
func (r *Reconciler) warmupRegistry(ctx context.Context) error {
	var errs []error

	extensionList := runtimev1.ExtensionConfigList{}
	if err := r.APIReader.List(ctx, &extensionList); err != nil {
		return err
	}

	for i := range extensionList.Items {
		extension := &extensionList.Items[i]
		original := extension.DeepCopy()

		extension, err := r.discoverExtension(ctx, extension)
		if err != nil {
			errs = append(errs, err)
		}

		// Patch the extension with the updated condition and RuntimeExtensions if discovered.
		if err = r.patchExtension(ctx, original, extension); err != nil {
			errs = append(errs, err)
		}
		extensionList.Items[i] = *extension
	}

	// If there was some error in discovery or patching return before committing to the Registry.
	if len(errs) != 0 {
		return kerrors.NewAggregate(errs)
	}

	if err := r.RuntimeClient.WarmUp(&extensionList); err != nil {
		return err
	}

	return nil
}

// discoverExtension attempts to discover the RuntimeExtensions for a passed Extension.
// If discovery succeeds it returns the Extension with RuntimeExtensions updated in Status and an updated Condition.
// If discovery fails it returns the extension with no update to RuntimeExtensions and a Failed Condition.
func (r Reconciler) discoverExtension(ctx context.Context, extension *runtimev1.ExtensionConfig) (*runtimev1.ExtensionConfig, error) {
	discoveredExtension, err := r.RuntimeClient.Extension(extension).Discover(ctx)
	if err != nil {
		conditions.MarkFalse(extension, runtimev1.RuntimeExtensionDiscovered, "DiscoveryFailed", clusterv1.ConditionSeverityError, "error in discovery: %v", err)
		return extension, errors.Wrap(err, "failed to discover extension")
	}

	if err = validateRuntimeExtensionDiscovery(extension); err != nil {
		conditions.MarkFalse(extension, runtimev1.RuntimeExtensionDiscovered, "DiscoveryFailed", clusterv1.ConditionSeverityError, "error in discovery: %v", err)
		return extension, errors.Wrap(err, "failed to validate RuntimeExtension")
	}

	extension = discoveredExtension
	conditions.MarkTrue(extension, runtimev1.RuntimeExtensionDiscovered)

	return extension, nil
}

// validateRuntimeExtensionDiscovery runs a set of validations on the data returned from an Extension's discovery call.
// if any of these checks fails the response is invalid and an error is returned. Extensions with previously valid
// RuntimeExtension registrations are not removed from the registry or the object's status.
func validateRuntimeExtensionDiscovery(ext *runtimev1.ExtensionConfig) error {
	for _, runtimeExtension := range ext.Status.Handlers {
		// TODO: Extend the validation to cover more cases.

		// simple dummy check to show how and where RuntimeExtension validation works.
		// this should aggregate the errors on validation.
		if len(runtimeExtension.Name) > 63 {
			return errors.New("RuntimeExtension name must be less than 64 characters")
		}
	}
	return nil
}
