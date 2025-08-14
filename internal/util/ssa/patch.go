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

package ssa

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"sigs.k8s.io/cluster-api/internal/contract"
)

// Option is the interface for configuration that modifies Options for a patch request.
type Option interface {
	// ApplyToOptions applies this configuration to the given Options.
	ApplyToOptions(*Options)
}

// WithCachingProxy enables caching for the patch request.
// The original and modified object will be used to generate an
// identifier for the request.
// The cache will be used to cache the result of the request.
type WithCachingProxy struct {
	Cache    Cache
	Original client.Object
}

// ApplyToOptions applies WithCachingProxy to the given Options.
func (w WithCachingProxy) ApplyToOptions(in *Options) {
	in.WithCachingProxy = true
	in.Cache = w.Cache
	in.Original = w.Original
}

// Options contains the options for the Patch func.
type Options struct {
	WithCachingProxy bool
	Cache            Cache
	Original         client.Object
}

// Patch executes an SSA patch.
// If WithCachingProxy is set and the request didn't change the object
// we will cache this result, so subsequent calls don't have to run SSA again.
func Patch(ctx context.Context, c client.Client, fieldManager string, modified client.Object, opts ...Option) error {
	// Calculate the options.
	options := &Options{}
	for _, opt := range opts {
		opt.ApplyToOptions(options)
	}

	// Convert the object to unstructured and filter out fields we don't
	// want to set (e.g. metadata creationTimestamp).
	// Note: This is necessary to avoid continuous reconciles.
	modifiedUnstructured, err := prepareModified(c.Scheme(), modified)
	if err != nil {
		return err
	}

	gvk, err := apiutil.GVKForObject(modifiedUnstructured, c.Scheme())
	if err != nil {
		return errors.Wrapf(err, "failed to apply object: failed to get GroupVersionKind of modified object %s", klog.KObj(modifiedUnstructured))
	}

	var requestIdentifier string
	if options.WithCachingProxy {
		// Check if the request is cached.
		requestIdentifier, err = ComputeRequestIdentifier(c.Scheme(), options.Original, modifiedUnstructured)
		if err != nil {
			return errors.Wrapf(err, "failed to apply object")
		}
		if options.Cache.Has(requestIdentifier, gvk.Kind) {
			// If the request is cached return the original object.
			if err := c.Scheme().Convert(options.Original, modified, ctx); err != nil {
				return errors.Wrapf(err, "failed to write original into modified object")
			}
			// Recover gvk e.g. for logging.
			modified.GetObjectKind().SetGroupVersionKind(gvk)
			return nil
		}
	}

	patchOptions := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(fieldManager),
	}
	if err := c.Patch(ctx, modifiedUnstructured, client.Apply, patchOptions...); err != nil {
		return errors.Wrapf(err, "failed to apply %s %s", gvk.Kind, klog.KObj(modifiedUnstructured))
	}

	// Write back the modified object so callers can access the patched object.
	if err := c.Scheme().Convert(modifiedUnstructured, modified, ctx); err != nil {
		return errors.Wrapf(err, "failed to write modified object")
	}

	// Recover gvk e.g. for logging.
	modified.GetObjectKind().SetGroupVersionKind(gvk)

	if options.WithCachingProxy {
		// If the SSA call did not update the object, add the request to the cache.
		if options.Original.GetResourceVersion() == modifiedUnstructured.GetResourceVersion() {
			options.Cache.Add(requestIdentifier)
		}
	}

	return nil
}

// prepareModified converts obj into an Unstructured and filters out undesired fields.
func prepareModified(scheme *runtime.Scheme, obj client.Object) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	switch obj.(type) {
	case *unstructured.Unstructured:
		u = obj.DeepCopyObject().(*unstructured.Unstructured)
	default:
		if err := scheme.Convert(obj, u, nil); err != nil {
			return nil, errors.Wrap(err, "failed to convert object to Unstructured")
		}
	}

	// Only keep the paths that we have opinions on.
	FilterObject(u, &FilterObjectInput{
		AllowedPaths: []contract.Path{
			// apiVersion, kind, name and namespace are required field for a server side apply intent.
			{"apiVersion"},
			{"kind"},
			{"metadata", "name"},
			{"metadata", "namespace"},
			// uid is optional for a server side apply intent but sets the expectation of an object getting created or a specific one updated.
			{"metadata", "uid"},
			// our controllers only have an opinion on labels, annotation, finalizers ownerReferences and spec.
			{"metadata", "labels"},
			{"metadata", "annotations"},
			{"metadata", "finalizers"},
			{"metadata", "ownerReferences"},
			{"spec"},
		},
	})
	return u, nil
}
