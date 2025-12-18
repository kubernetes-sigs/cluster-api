/*
Copyright 2025 The Kubernetes Authors.

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

// Package client provides utils for usage with the controller-runtime client.
package client

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/resourceversion"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var (
	// waitBackoff is the timeout used when waiting for the cache to become up-to-date.
	// This adds up to ~ 10 seconds max wait duration.
	waitBackoff = wait.Backoff{
		Duration: 25 * time.Microsecond,
		Cap:      2 * time.Second,
		Factor:   1.2,
		Steps:    63,
	}
)

// WaitForCacheToBeUpToDate waits until the cache is up-to-date in the sense of that the cache contains
// all passed in objects with at least the passed in resourceVersion.
// This is done by retrieving objects from the cache via the client and then comparing resourceVersions.
// Note: This func will update the passed in objects while polling.
// Note: resourceVersion must be set on the passed in objects.
// Note: The generic parameter enforces that all objects have the same type.
func WaitForCacheToBeUpToDate[T client.Object](ctx context.Context, c client.Client, action string, objs ...T) error {
	return waitFor(ctx, c, action, checkIfObjectUpToDate, objs...)
}

// WaitForObjectsToBeAddedToTheCache waits until the cache is up-to-date in the sense of that the
// passed in objects exist in the cache.
// Note: This func will update the passed in objects while polling.
// Note: The generic parameter enforces that all objects have the same type.
func WaitForObjectsToBeAddedToTheCache[T client.Object](ctx context.Context, c client.Client, action string, objs ...T) error {
	return waitFor(ctx, c, action, checkIfObjectAdded, objs...)
}

// WaitForObjectsToBeDeletedFromTheCache waits until the cache is up-to-date in the sense of that the
// passed in objects have been either removed from the cache or they have a deletionTimestamp set.
// Note: This func will update the passed in objects while polling.
// Note: The generic parameter enforces that all objects have the same type.
func WaitForObjectsToBeDeletedFromTheCache[T client.Object](ctx context.Context, c client.Client, action string, objs ...T) error {
	return waitFor(ctx, c, action, checkIfObjectDeleted, objs...)
}

// checkIfObjectUpToDate checks if an object is up-to-date and returns an error if it is not.
func checkIfObjectUpToDate(ctx context.Context, c client.Client, desiredObj desiredObject) (isErrorRetryable bool, err error) {
	if desiredObj.MinimumResourceVersion == "" {
		// Unexpected error occurred: resourceVersion not set on passed in object (not retryable).
		return false, errors.Errorf("%s: cannot compare with invalid resourceVersion: resourceVersion not set",
			klog.KObj(desiredObj.Object))
	}

	if err := c.Get(ctx, desiredObj.Key, desiredObj.Object); err != nil {
		if apierrors.IsNotFound(err) {
			// Done, object was deleted in the meantime.
			return false, nil
		}
		// Unexpected error occurred (not retryable).
		return false, err
	}

	cmp, err := resourceversion.CompareResourceVersion(desiredObj.Object.GetResourceVersion(), desiredObj.MinimumResourceVersion)
	if err != nil {
		// Unexpected error occurred: invalid resourceVersion (not retryable).
		return false, errors.Wrapf(err, "%s: cannot compare with invalid resourceVersion: current: %s, expected to be >= %s",
			klog.KObj(desiredObj.Object), desiredObj.Object.GetResourceVersion(), desiredObj.MinimumResourceVersion)
	}
	if cmp < 0 {
		// resourceVersion < MinimumResourceVersion (retryable).
		return true, errors.Errorf("%s: resourceVersion not yet up-to-date: current: %s, expected to be >= %s",
			klog.KObj(desiredObj.Object), desiredObj.Object.GetResourceVersion(), desiredObj.MinimumResourceVersion)
	}

	// Done, resourceVersion is new enough.
	return false, nil
}

func checkIfObjectAdded(ctx context.Context, c client.Client, desiredObj desiredObject) (isErrorRetryable bool, err error) {
	if err := c.Get(ctx, desiredObj.Key, desiredObj.Object); err != nil {
		if apierrors.IsNotFound(err) {
			// Object is not yet in the cache (retryable).
			return true, err
		}
		// Unexpected error occurred (not retryable).
		return false, err
	}

	// Done, object exists in the cache.
	return false, nil
}

func checkIfObjectDeleted(ctx context.Context, c client.Client, desiredObj desiredObject) (isErrorRetryable bool, err error) {
	if err := c.Get(ctx, desiredObj.Key, desiredObj.Object); err != nil {
		if apierrors.IsNotFound(err) {
			// Done, object has been removed from the cache.
			return false, nil
		}
		// Unexpected error occurred (not retryable).
		return false, err
	}

	if !desiredObj.Object.GetDeletionTimestamp().IsZero() {
		// Done, object has deletionTimestamp set.
		return false, nil
	}

	// Object does not have deletionTimestamp set yet (retryable).
	return true, fmt.Errorf("%s still exists", klog.KObj(desiredObj.Object))
}

type desiredObject struct {
	Object                 client.Object
	Key                    client.ObjectKey
	MinimumResourceVersion string
}

type checkFunc func(ctx context.Context, c client.Client, desiredObj desiredObject) (retryableErr bool, err error)

func waitFor[T client.Object](ctx context.Context, c client.Client, action string, checkFunc checkFunc, objs ...T) error {
	// Done, if there are no objects.
	if len(objs) == 0 {
		return nil
	}

	var o any = objs[0]
	if _, ok := o.(*unstructured.Unstructured); ok {
		return errors.Errorf("failed to wait for up-to-date objects in the cache after %s: Unstructured is not supported", action)
	}

	// All objects have the same type, so we can just take the GVK of the first object.
	objGVK, err := apiutil.GVKForObject(objs[0], c.Scheme())
	if err != nil {
		return errors.Wrapf(err, "failed to wait for up-to-date objects in the cache after %s", action)
	}

	log := ctrl.LoggerFrom(ctx)

	desiredObjects := make([]desiredObject, len(objs))
	for i, obj := range objs {
		desiredObjects[i] = desiredObject{
			Object:                 obj,
			Key:                    client.ObjectKeyFromObject(obj),
			MinimumResourceVersion: obj.GetResourceVersion(),
		}
	}

	now := time.Now()

	var pollErrs []error
	err = wait.ExponentialBackoffWithContext(ctx, waitBackoff, func(ctx context.Context) (bool, error) {
		pollErrs = nil

		for _, desiredObj := range desiredObjects {
			if isErrorRetryable, err := checkFunc(ctx, c, desiredObj); err != nil {
				pollErrs = append(pollErrs, err)
				if !isErrorRetryable {
					// Stop polling, non-retryable error occurred.
					return true, nil
				}
			}
		}

		if len(pollErrs) > 0 {
			// Continue polling, only retryable errors occurred.
			return false, nil
		}

		// Stop polling, all objects are up-to-date.
		return true, nil
	})

	waitDuration := time.Since(now)

	if err != nil || len(pollErrs) > 0 {
		waitDurationMetric.WithLabelValues(objGVK.Kind, "error").Observe(waitDuration.Seconds())

		var errSuffix string
		if err != nil {
			if wait.Interrupted(err) {
				errSuffix = ": timed out"
			} else {
				errSuffix = fmt.Sprintf(": %s", err.Error())
			}
		}
		err := errors.Errorf("failed to wait for up-to-date %s objects in the cache after %s%s: %s", objGVK.Kind, action, errSuffix, kerrors.NewAggregate(pollErrs))
		log.Error(err, "Failed to wait for cache to be up-to-date", "kind", objGVK.Kind, "waitDuration", waitDuration)
		return err
	}

	waitDurationMetric.WithLabelValues(objGVK.Kind, "success").Observe(waitDuration.Seconds())

	// Log on a high log-level if it took a long time for the cache to be up-to-date.
	if waitDuration >= 1*time.Second {
		log.Info("Successfully waited for cache to be up-to-date (>=1s)", "kind", objGVK.Kind, "waitDuration", waitDuration)
	} else {
		log.V(10).Info("Successfully waited for cache to be up-to-date", "kind", objGVK.Kind, "waitDuration", waitDuration)
	}

	return nil
}
