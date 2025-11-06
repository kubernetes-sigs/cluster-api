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
	"strings"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
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
// Note: The generic parameter enforces that all objects have the same type.
func WaitForCacheToBeUpToDate[T client.Object](ctx context.Context, c client.Client, action string, objs ...T) error {
	return waitFor(ctx, c, action, checkIfObjectUpToDate, objs...)
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
	if err := c.Get(ctx, desiredObj.Key, desiredObj.Object); err != nil {
		if apierrors.IsNotFound(err) {
			// Object is not yet in the cache (retryable).
			return true, err
		}
		// Unexpected error occurred (not retryable).
		return false, err
	}

	if desiredObj.MinimumResourceVersion == "" {
		// Done, if MinimumResourceVersion is empty, as it is enough if the object exists in the cache.
		// Note: This can happen when the ServerSidePatchHelper is used to create an object as the ServerSidePatchHelper
		//       does not update the object after Apply and accordingly resourceVersion remains empty.
		return false, nil
	}

	cmp, err := compareResourceVersion(desiredObj.Object.GetResourceVersion(), desiredObj.MinimumResourceVersion)
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

type invalidResourceVersion struct {
	rv string
}

func (i invalidResourceVersion) Error() string {
	return fmt.Sprintf("resource version is not well formed: %s", i.rv)
}

// compareResourceVersion runs a comparison between two ResourceVersions. This
// only has semantic meaning when the comparison is done on two objects of the
// same resource. The return values are:
//
//	-1: If RV a < RV b
//	 0: If RV a == RV b
//	+1: If RV a > RV b
//
// The function will return an error if the resource version is not a properly
// formatted positive integer, but has no restriction on length. A properly
// formatted integer will not contain leading zeros or non integer characters.
// Zero is also considered an invalid value as it is used as a special value in
// list/watch events and will never be a live resource version.
// TODO(controller-runtime-0.23): This code has been copied from
// https://github.com/kubernetes/kubernetes/blob/v1.35.0-alpha.2/staging/src/k8s.io/apimachinery/pkg/util/resourceversion/resourceversion.go
// and will be removed once we bump to CR v0.23 / k8s.io/apimachinery v1.35.0.
func compareResourceVersion(a, b string) (int, error) {
	if !isWellFormed(a) {
		return 0, invalidResourceVersion{rv: a}
	}
	if !isWellFormed(b) {
		return 0, invalidResourceVersion{rv: b}
	}
	// both are well-formed integer strings with no leading zeros
	aLen := len(a)
	bLen := len(b)
	switch {
	case aLen < bLen:
		// shorter is less
		return -1, nil
	case aLen > bLen:
		// longer is greater
		return 1, nil
	default:
		// equal-length compares lexically
		return strings.Compare(a, b), nil
	}
}

func isWellFormed(s string) bool {
	if len(s) == 0 { //nolint:gocritic // not going to modify code copied from upstream
		return false
	}
	if s[0] == '0' {
		return false
	}
	for i := range s {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
